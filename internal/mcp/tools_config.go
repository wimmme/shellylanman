package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wimmme/shellylanman/internal/service"
)

// Phase 11a: configuration tools (access "configure", DECISIONS P11-2). Lists
// and reads go through shelly_rpc_read (KVS.GetMany, Schedule.List,
// Script.List, Webhook.List, Shelly.GetComponents).

// rpcResult unmarshals a device answer for the tool result.
func rpcResult(b json.RawMessage) any {
	var v any
	if len(b) == 0 || json.Unmarshal(b, &v) != nil {
		return nil
	}
	return v
}

// call resolves the device and runs one Gen2+ RPC method with params.
func call(ctx context.Context, s Service, ref, method string, params any) (any, error) {
	d, err := resolve(s, ref)
	if err != nil {
		return nil, err
	}
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	b, err := s.DeviceRPC(ctx, d.ID, method, p)
	if err != nil {
		return nil, err
	}
	return map[string]any{"device": d.ID, "method": method, "result": rpcResult(b)}, nil
}

func needConfirm(confirm bool, what string) error {
	if !confirm {
		return fmt.Errorf("%w: %s needs confirm=true after the user agreed", errUser, what)
	}
	return nil
}

// validTimespec: Shelly schedules take 6 cron fields (sec min hour dom month dow)
// or a sunrise/sunset form ("@sunrise+30m 0 * * *" style has fewer fields).
func validTimespec(ts string) error {
	f := strings.Fields(ts)
	if len(f) == 6 || (len(f) >= 1 && strings.HasPrefix(f[0], "@")) {
		return nil
	}
	return fmt.Errorf("%w: timespec needs 6 cron fields (sec min hour day month weekday), e.g. \"0 30 7 * * MON-FRI\", or @sunrise/@sunset", errUser)
}

var (
	idArg     = integer("Id", 0, 100000)
	callsArg  = map[string]any{"type": "array", "minItems": 1, "maxItems": 5, "description": "RPC calls to make: [{\"method\": \"Switch.Set\", \"params\": {\"id\": 0, \"on\": true}}]", "items": map[string]any{"type": "object"}}
	paramsArg = map[string]any{"type": "object", "description": "Method parameters"}
)

var configTools = []tool{
	// ---- KVS ----
	{
		name: "shelly_kvs_set", title: "Set a KVS value", level: levelConfigure,
		description: "Store a value (any JSON) under a key in a Gen2+ device's key-value store (KVS.Set).",
		schema: obj([]string{"device", "key", "value"}, map[string]any{
			"device": deviceArg, "key": str("Key"), "value": map[string]any{"description": "Value (string, number, boolean, object or array)"},
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Key string
				Value       json.RawMessage
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if a.Key == "" || len(a.Value) == 0 {
				return nil, fmt.Errorf("%w: key and value are required", errUser)
			}
			return call(ctx, s, a.Device, "KVS.Set", map[string]any{"key": a.Key, "value": a.Value})
		},
	},
	{
		name: "shelly_kvs_delete", title: "Delete a KVS value", level: levelConfigure, destructive: true,
		description: "Delete a key from a Gen2+ device's key-value store. Ask the user first and pass confirm=true.",
		schema:      obj([]string{"device", "key", "confirm"}, map[string]any{"device": deviceArg, "key": str("Key"), "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Key string
				Confirm     bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "deleting a KVS key"); err != nil {
				return nil, err
			}
			return call(ctx, s, a.Device, "KVS.Delete", map[string]any{"key": a.Key})
		},
	},

	// ---- schedules ----
	{
		name: "shelly_schedule_set", title: "Create or change a schedule", level: levelConfigure,
		description: "Create a schedule on a Gen2+ device (without id: timespec and calls required) or change one (with id: only the fields given). At most 20 per device.",
		schema: obj([]string{"device"}, map[string]any{
			"device": deviceArg, "id": idArg,
			"timespec": str("Cron with seconds: \"sec min hour day month weekday\", e.g. \"0 0 22 * * *\"; or @sunrise / @sunset with offset"),
			"calls":    callsArg,
			"enable":   boolean("Enabled (default true for a new schedule)"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device   string
				ID       *int
				Timespec *string
				Calls    []map[string]any
				Enable   *bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			p := map[string]any{}
			if a.Timespec != nil {
				if err := validTimespec(*a.Timespec); err != nil {
					return nil, err
				}
				p["timespec"] = *a.Timespec
			}
			for i, c := range a.Calls {
				m, _ := c["method"].(string)
				if m == "" {
					return nil, fmt.Errorf("%w: call %d has no method", errUser, i+1)
				}
				if cl := service.ClassifyRPC(m); cl == service.RPCDataLoss {
					return nil, fmt.Errorf("%w: %s is not allowed in a schedule", errUser, m)
				}
			}
			if a.Calls != nil {
				p["calls"] = a.Calls
			}
			if a.Enable != nil {
				p["enable"] = *a.Enable
			}
			if a.ID != nil {
				if len(p) == 0 {
					return nil, fmt.Errorf("%w: nothing to change", errUser)
				}
				p["id"] = *a.ID
				return call(ctx, s, a.Device, "Schedule.Update", p)
			}
			if a.Timespec == nil || len(a.Calls) == 0 {
				return nil, fmt.Errorf("%w: a new schedule needs timespec and calls", errUser)
			}
			if a.Enable == nil {
				p["enable"] = true
			}
			return call(ctx, s, a.Device, "Schedule.Create", p)
		},
	},
	{
		name: "shelly_schedule_delete", title: "Delete a schedule", level: levelConfigure, destructive: true,
		description: "Delete a schedule from a Gen2+ device. Ask the user first and pass confirm=true.",
		schema:      obj([]string{"device", "id", "confirm"}, map[string]any{"device": deviceArg, "id": idArg, "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device  string
				ID      int
				Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "deleting a schedule"); err != nil {
				return nil, err
			}
			return call(ctx, s, a.Device, "Schedule.Delete", map[string]any{"id": a.ID})
		},
	},

	// ---- scripts ----
	{
		name: "shelly_script_create", title: "Create a script", level: levelConfigure,
		description: "Create an empty script on a Gen2+ device; give it code with shelly_script_put_code.",
		schema:      obj([]string{"device", "name"}, map[string]any{"device": deviceArg, "name": str("Script name")}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Device, Name string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			info, err := s.ScriptCreate(ctx, d.ID, a.Name)
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "script": info}, nil
		},
	},
	{
		name: "shelly_script_put_code", title: "Upload script code", level: levelConfigure, destructive: true,
		description: "Replace (or with append=true, extend) the code of a script. The code runs on the device: show it to the user, ask first and pass confirm=true.",
		schema: obj([]string{"device", "id", "code", "confirm"}, map[string]any{
			"device": deviceArg, "id": integer("Script id", 1, 100), "code": str("JavaScript (mJS) code"),
			"append": boolean("Add to the existing code instead of replacing it"), "confirm": confirmArg,
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Code    string
				ID              int
				Append, Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "uploading script code"); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			code := a.Code
			if a.Append {
				old, err := s.ScriptCode(ctx, d.ID, a.ID)
				if err != nil {
					return nil, err
				}
				code = old + code
			}
			if err := s.ScriptPutCode(ctx, d.ID, a.ID, code); err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "id": a.ID, "length": len(code)}, nil
		},
	},
	{
		name: "shelly_script_run", title: "Start or stop a script", level: levelConfigure,
		description: "Start or stop a script on a Gen2+ device.",
		schema: obj([]string{"device", "id", "action"}, map[string]any{
			"device": deviceArg, "id": integer("Script id", 1, 100), "action": str("What to do", "start", "stop"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Action string
				ID             int
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if a.Action != "start" && a.Action != "stop" {
				return nil, fmt.Errorf("%w: action must be start or stop", errUser)
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			if err := s.ScriptRun(ctx, d.ID, a.ID, a.Action == "start", false); err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "id": a.ID, "done": a.Action}, nil
		},
	},
	{
		name: "shelly_script_eval", title: "Evaluate code in a script", level: levelConfigure, destructive: true,
		description: "Evaluate an expression inside a running script (Script.Eval) and return the result. Arbitrary code: ask first and pass confirm=true.",
		schema: obj([]string{"device", "id", "code", "confirm"}, map[string]any{
			"device": deviceArg, "id": integer("Script id (must be running)", 1, 100), "code": str("Expression"), "confirm": confirmArg,
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Code string
				ID           int
				Confirm      bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "evaluating code"); err != nil {
				return nil, err
			}
			return call(ctx, s, a.Device, "Script.Eval", map[string]any{"id": a.ID, "code": a.Code})
		},
	},
	{
		name: "shelly_script_delete", title: "Delete a script", level: levelConfigure, destructive: true,
		description: "Delete a script from a Gen2+ device. Ask the user first and pass confirm=true.",
		schema:      obj([]string{"device", "id", "confirm"}, map[string]any{"device": deviceArg, "id": integer("Script id", 1, 100), "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device  string
				ID      int
				Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "deleting a script"); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			if err := s.ScriptDelete(ctx, d.ID, a.ID); err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "deleted": a.ID}, nil
		},
	},

	// ---- webhooks ----
	{
		name: "shelly_webhook_set", title: "Create or change a webhook", level: levelConfigure,
		description: "Create a webhook on a Gen2+ device (without id: event, cid and urls required) or change one (with id: only the fields given). Supported events: shelly_rpc_read Webhook.ListSupported.",
		schema: obj([]string{"device"}, map[string]any{
			"device": deviceArg, "id": idArg,
			"event":         str("Event, e.g. \"switch.on\""),
			"cid":           integer("Component id the event belongs to", 0, 300),
			"urls":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "maxItems": 5, "description": "URLs to call"},
			"name":          str("Name"),
			"enable":        boolean("Enabled (default true for a new webhook)"),
			"condition":     str("Condition (JavaScript expression), optional"),
			"repeat_period": integer("Seconds before it may fire again (0 always, -1 once)", -1, 86400),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device       string
				ID           *int
				Event        *string
				Cid          *int
				URLs         []string
				Name         *string
				Enable       *bool
				Condition    *string
				RepeatPeriod *int `json:"repeat_period"`
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			p := map[string]any{}
			set := func(k string, v any, ok bool) {
				if ok {
					p[k] = v
				}
			}
			set("event", deref(a.Event), a.Event != nil)
			set("cid", derefInt(a.Cid), a.Cid != nil)
			set("urls", a.URLs, a.URLs != nil)
			set("name", deref(a.Name), a.Name != nil)
			set("enable", a.Enable != nil && *a.Enable, a.Enable != nil)
			set("condition", deref(a.Condition), a.Condition != nil)
			set("repeat_period", derefInt(a.RepeatPeriod), a.RepeatPeriod != nil)
			if a.ID != nil {
				if len(p) == 0 {
					return nil, fmt.Errorf("%w: nothing to change", errUser)
				}
				if a.Event != nil || a.Cid != nil {
					return nil, fmt.Errorf("%w: event and cid of a webhook cannot change; delete it and create a new one", errUser)
				}
				p["id"] = *a.ID
				return call(ctx, s, a.Device, "Webhook.Update", p)
			}
			if a.Event == nil || a.Cid == nil || len(a.URLs) == 0 {
				return nil, fmt.Errorf("%w: a new webhook needs event, cid and urls", errUser)
			}
			if a.Enable == nil {
				p["enable"] = true
			}
			return call(ctx, s, a.Device, "Webhook.Create", p)
		},
	},
	{
		name: "shelly_webhook_delete", title: "Delete a webhook", level: levelConfigure, destructive: true,
		description: "Delete a webhook from a Gen2+ device. Ask the user first and pass confirm=true.",
		schema:      obj([]string{"device", "id", "confirm"}, map[string]any{"device": deviceArg, "id": idArg, "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device  string
				ID      int
				Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "deleting a webhook"); err != nil {
				return nil, err
			}
			return call(ctx, s, a.Device, "Webhook.Delete", map[string]any{"id": a.ID})
		},
	},

	// ---- virtual components ----
	{
		name: "shelly_virtual_add", title: "Add a virtual component", level: levelConfigure,
		description: "Add a virtual component to a Gen2+ device (Virtual.Add): boolean, number, text, enum, button or group, optionally with its config and id (200–299). List them with shelly_list_components.",
		schema: obj([]string{"device", "type"}, map[string]any{
			"device": deviceArg, "type": str("Component type", "boolean", "number", "text", "enum", "button", "group"),
			"id": integer("Component id (optional)", 200, 299), "config": map[string]any{"type": "object", "description": "Component configuration, e.g. {\"name\": \"Away\"}"},
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Type string
				ID           *int
				Config       map[string]any
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if !slices.Contains([]string{"boolean", "number", "text", "enum", "button", "group"}, a.Type) {
				return nil, fmt.Errorf("%w: unknown type %q", errUser, a.Type)
			}
			p := map[string]any{"type": a.Type}
			if a.ID != nil {
				p["id"] = *a.ID
			}
			if a.Config != nil {
				p["config"] = a.Config
			}
			return call(ctx, s, a.Device, "Virtual.Add", p)
		},
	},
	{
		name: "shelly_virtual_delete", title: "Delete a virtual component", level: levelConfigure, destructive: true,
		description: "Delete a virtual component (key like \"boolean:200\"). Ask the user first and pass confirm=true.",
		schema:      obj([]string{"device", "key", "confirm"}, map[string]any{"device": deviceArg, "key": str("Component key, e.g. boolean:200"), "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Key string
				Confirm     bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "deleting a virtual component"); err != nil {
				return nil, err
			}
			return call(ctx, s, a.Device, "Virtual.Delete", map[string]any{"key": a.Key})
		},
	},

	// ---- RPC write, device login ----
	{
		name: "shelly_rpc_write", title: "Call an RPC method that changes something", level: levelConfigure, destructive: true,
		description: "Call any Gen2+ RPC method that changes the device (e.g. Switch.SetConfig, Sys.SetConfig, Input.SetConfig). Read methods go through shelly_rpc_read; the device password through shelly_device_login. Factory reset, Wi-Fi reset and delete-all also need allow_data_loss=true. Ask first and pass confirm=true.",
		schema: obj([]string{"device", "method", "confirm"}, map[string]any{
			"device": deviceArg, "method": str("RPC method"), "params": paramsArg, "confirm": confirmArg,
			"allow_data_loss": boolean("Must be true for factory reset, Wi-Fi reset and delete-all methods"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Method string
				Params         json.RawMessage
				Confirm        bool
				AllowDataLoss  bool `json:"allow_data_loss"`
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			switch {
			case service.ClassifyRPC(a.Method) == service.RPCRead:
				return nil, fmt.Errorf("%w: %s only reads: use shelly_rpc_read", errUser, a.Method)
			case strings.EqualFold(a.Method, "Shelly.SetAuth"):
				return nil, fmt.Errorf("%w: use shelly_device_login, so ShellyLanMan keeps the new password", errUser)
			case service.ClassifyRPC(a.Method) == service.RPCDataLoss && !a.AllowDataLoss:
				return nil, fmt.Errorf("%w: %s wipes data on the device: it also needs allow_data_loss=true after the user agreed", errUser, a.Method)
			}
			if err := needConfirm(a.Confirm, a.Method); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			b, err := s.DeviceRPC(ctx, d.ID, a.Method, a.Params)
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "method": a.Method, "result": rpcResult(b)}, nil
		},
	},
	{
		name: "shelly_device_login", title: "Set or remove the device password", level: levelConfigure, destructive: true,
		description: "Protect devices with a password (restricted login) or remove it. ShellyLanMan keeps the new password so it can still reach them. Gen2+ users are always \"admin\"; Gen1 needs user. Off-line devices get it when they come back. Ask first and pass confirm=true.",
		schema: obj([]string{"devices", "enabled", "confirm"}, map[string]any{
			"devices": devicesArg, "enabled": boolean("true: set the password; false: remove it"),
			"user": str("User name (Gen1; Gen2+ always admin)"), "password": str("New password (with enabled=true)"), "confirm": confirmArg,
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Devices          []string
				Enabled, Confirm bool
				User, Password   string
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "changing the device login"); err != nil {
				return nil, err
			}
			if a.Enabled && len(a.Password) < 8 {
				return nil, fmt.Errorf("%w: password needs at least 8 characters", errUser)
			}
			list, err := resolveAll(s, a.Devices)
			if err != nil {
				return nil, err
			}
			if a.User == "" {
				a.User = "admin"
			}
			lines, err := s.ConfigApply(ctx, ids(list), service.SectionLogin, service.LoginApply{Enabled: a.Enabled, User: a.User, Password: a.Password})
			if err != nil {
				return nil, err
			}
			return results(lines), nil
		},
	},

	// ---- scenes ----
	{
		name: "shelly_scene_set", title: "Create or replace a scene", level: levelConfigure,
		description: "Store a named scene in ShellyLanMan: an ordered list of actions, each either {\"tool\": one of shelly_switch, shelly_light, shelly_cover, shelly_thermostat, \"arguments\": the same arguments as that tool} or {\"device\", \"method\", \"params\"} for a Gen2+ RPC method (no reboot, update, delete, code or credentials). Prefer absolute states (on/off) over toggle, so a scene can safely run twice. Run it with shelly_scene_run.",
		schema: obj([]string{"name", "actions"}, map[string]any{
			"name": str("Scene name"), "description": str("What the scene is for"),
			"actions":   map[string]any{"type": "array", "minItems": 1, "maxItems": service.MaxSceneActions, "items": map[string]any{"type": "object"}, "description": "The actions in order"},
			"overwrite": boolean("Replace a scene with the same name"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Name, Description string
				Actions           []json.RawMessage
				Overwrite         bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			sc := service.Scene{Name: a.Name, Description: a.Description}
			var warnings []string
			for i, raw := range a.Actions {
				acts, warn, err := sceneActions(s, raw)
				if err != nil {
					return nil, fmt.Errorf("action %d: %w", i+1, err)
				}
				sc.Actions = append(sc.Actions, acts...)
				if warn != "" {
					warnings = append(warnings, fmt.Sprintf("action %d: %s", i+1, warn))
				}
			}
			saved, err := s.SceneSave(sc, a.Overwrite)
			if err != nil {
				return nil, err
			}
			out := map[string]any{"scene": saved.Name, "actions": len(saved.Actions)}
			if len(warnings) > 0 {
				out["warnings"] = warnings
			}
			return out, nil
		},
	},
	{
		name: "shelly_scene_delete", title: "Delete a scene", level: levelConfigure, destructive: true,
		description: "Delete a scene from ShellyLanMan. Ask the user first and pass confirm=true.",
		schema:      obj([]string{"name", "confirm"}, map[string]any{"name": str("Scene name"), "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Name    string
				Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if err := needConfirm(a.Confirm, "deleting a scene"); err != nil {
				return nil, err
			}
			if err := s.SceneDelete(a.Name); err != nil {
				return nil, err
			}
			return map[string]any{"deleted": a.Name}, nil
		},
	},
}

// sceneActions turns one scene action as the assistant gives it into the
// service's actions (a light action can become several commands).
func sceneActions(s Service, raw json.RawMessage) ([]service.SceneAction, string, error) {
	var head struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
		Device    string          `json:"device"`
		Method    string          `json:"method"`
		Params    json.RawMessage `json:"params"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&head); err != nil {
		return nil, "", fmt.Errorf("%w: %v", errUser, err)
	}
	if head.Tool != "" {
		planner, ok := planners[head.Tool]
		if !ok {
			return nil, "", fmt.Errorf("%w: tool must be one of %s", errUser, strings.Join(slices.Sorted(maps.Keys(planners)), ", "))
		}
		if len(head.Arguments) == 0 {
			head.Arguments = json.RawMessage("{}")
		}
		p, err := planner(head.Arguments)
		if err != nil {
			return nil, "", err
		}
		d, m, err := p.target(s)
		if err != nil {
			return nil, "", err
		}
		var out []service.SceneAction
		warn := ""
		for _, c := range p.cmds {
			c.Key = m.Key
			if c.Action == service.ActionToggle {
				warn = "toggle depends on the current state; on/off is safer to run twice"
			}
			cc := c
			out = append(out, service.SceneAction{Device: d.ID, Command: &cc})
		}
		return out, warn, nil
	}
	if head.Method == "" || head.Device == "" {
		return nil, "", fmt.Errorf("%w: give {tool, arguments} or {device, method, params}", errUser)
	}
	d, err := resolve(s, head.Device)
	if err != nil {
		return nil, "", err
	}
	warn := ""
	if strings.HasSuffix(strings.ToLower(head.Method), ".toggle") {
		warn = "toggle depends on the current state; Set is safer to run twice"
	}
	return []service.SceneAction{{Device: d.ID, Method: head.Method, Params: head.Params}}, warn, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
