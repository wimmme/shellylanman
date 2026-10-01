package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
)

// Phase 11a: the read and control tools that bring the MCP to the Shelly-MCP's
// functions (DECISIONS P11-5). Configuration tools are in tools_config.go.

func init() {
	tools = append(tools, moreTools...)
	tools = append(tools, configTools...)
}

var componentArg = str("Only this part, e.g. \"switch:0\" (Gen2+) or \"relays\" (Gen1)")

// raw reads a device's full status or configuration as the device answers it.
func raw(ctx context.Context, s Service, d model.Device, config bool) (map[string]any, error) {
	var b json.RawMessage
	var err error
	switch d.Gen {
	case "1":
		path := "/status"
		if config {
			path = "/settings"
		}
		b, err = s.DeviceGet(ctx, d.ID, path)
	case model.GenBLU, model.GenBTHome:
		return nil, fmt.Errorf("%w: %s is a BLU device: use shelly_get_device", errUser, label(d))
	default:
		method := "Shelly.GetStatus"
		if config {
			method = "Shelly.GetConfig"
		}
		b, err = s.DeviceRPC(ctx, d.ID, method, nil)
	}
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// pick narrows a status/config to one top-level component.
func pick(v map[string]any, component string) (any, error) {
	if component == "" {
		return v, nil
	}
	if x, ok := v[component]; ok {
		return x, nil
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return nil, fmt.Errorf("%w: no component %q; there are: %s", errUser, component, strings.Join(keys, ", "))
}

// secretKeys are masked in configurations the MCP returns (Gen1 /settings
// holds some in clear text); "key" only under Wi-Fi/AP objects, where it is a
// password (elsewhere it names a component or a KVS key).
var secretKeys = map[string]bool{"pass": true, "password": true, "passwd": true, "psk": true, "secret": true, "token": true, "ha1": true}

func mask(v any, parent string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			lk := strings.ToLower(k)
			wifiKey := lk == "key" && (strings.HasPrefix(parent, "wifi") || parent == "ap" || strings.HasPrefix(parent, "sta"))
			if s, ok := val.(string); ok && s != "" && (secretKeys[lk] || wifiKey) {
				t[k] = "***"
				continue
			}
			t[k] = mask(val, lk)
		}
	case []any:
		for i := range t {
			t[i] = mask(t[i], parent)
		}
	}
	return v
}

var moreTools = []tool{
	{
		name: "shelly_get_status", title: "Get raw status",
		description: "A device's full live status as the device reports it (Gen1 /status, Gen2+ Shelly.GetStatus), or one component of it.",
		schema:      obj([]string{"device"}, map[string]any{"device": deviceArg, "component": componentArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Device, Component string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			v, err := raw(ctx, s, d, false)
			if err != nil {
				return nil, err
			}
			part, err := pick(v, a.Component)
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "status": part}, nil
		},
	},
	{
		name: "shelly_get_config", title: "Get raw configuration",
		description: "A device's configuration as the device reports it (Gen1 /settings, Gen2+ Shelly.GetConfig), or one component of it. Passwords are masked as ***.",
		schema:      obj([]string{"device"}, map[string]any{"device": deviceArg, "component": componentArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Device, Component string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			v, err := raw(ctx, s, d, true)
			if err != nil {
				return nil, err
			}
			part, err := pick(mask(v, "").(map[string]any), a.Component)
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "config": part}, nil
		},
	},
	{
		name: "shelly_list_components", title: "List components",
		description: "The components of a device (e.g. switch:0, input:0, script:1, boolean:200). Gen2+: Shelly.GetComponents, including virtual components; Gen1: the modules ShellyLanMan knows. For the RPC methods of a Gen2+ device use shelly_rpc_read with Shelly.ListMethods.",
		schema:      obj([]string{"device"}, map[string]any{"device": deviceArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Device string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			if d.Gen == "1" || d.Gen == model.GenBLU || d.Gen == model.GenBTHome {
				keys := []string{}
				for _, m := range d.Modules {
					if m.Key != "" {
						keys = append(keys, m.Key)
					}
				}
				return map[string]any{"device": d.ID, "components": keys}, nil
			}
			var keys []string
			for offset := 0; ; {
				b, err := s.DeviceRPC(ctx, d.ID, "Shelly.GetComponents", json.RawMessage(fmt.Sprintf(`{"offset":%d,"include":["config"]}`, offset)))
				if err != nil {
					return nil, err
				}
				var page struct {
					Components []struct {
						Key string `json:"key"`
					} `json:"components"`
					Offset int `json:"offset"`
					Total  int `json:"total"`
				}
				if err := json.Unmarshal(b, &page); err != nil {
					return nil, err
				}
				for _, c := range page.Components {
					keys = append(keys, c.Key)
				}
				offset = page.Offset + len(page.Components)
				if len(page.Components) == 0 || offset >= page.Total {
					break
				}
			}
			return map[string]any{"device": d.ID, "components": keys}, nil
		},
	},
	{
		name: "shelly_script_code", title: "Read script code",
		description: "The full source of a script on a Gen2+ device (shelly_rpc_read with Script.List lists the scripts).",
		schema:      obj([]string{"device", "id"}, map[string]any{"device": deviceArg, "id": integer("Script id", 1, 100)}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device string
				ID     int
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			code, err := s.ScriptCode(ctx, d.ID, a.ID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "id": a.ID, "code": code}, nil
		},
	},
	{
		name: "shelly_energy_history", title: "Energy history",
		description: "Energy per period from the device's own energy log (Pro 3EM, Pro EM and other EM/EM1 meters: EMData / EM1Data), in Wh per record. For other devices use shelly_get_readings.",
		schema:      obj([]string{"device"}, map[string]any{"device": deviceArg, "hours": integer("How far back, default 24", 1, 24*31)}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device string
				Hours  int
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if a.Hours == 0 {
				a.Hours = 24
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			end := time.Now().Unix()
			series, err := s.EMEnergy(ctx, d.ID, end-int64(a.Hours)*3600, end)
			if err != nil {
				return nil, err
			}
			if len(series) == 0 {
				return nil, fmt.Errorf("%w: %s keeps no energy log; use shelly_get_readings", errUser, label(d))
			}
			return map[string]any{"device": d.ID, "hours": a.Hours, "series": series,
				"note": "data: [unix seconds, Wh] per record; a 3-phase meter has its lines a, b, c one after the other"}, nil
		},
	},
	{
		name: "shelly_scenes", title: "List scenes",
		description: "The scenes stored in ShellyLanMan (named lists of device actions), or one scene with its actions when name is given.",
		schema:      obj(nil, map[string]any{"name": str("Scene name (optional)")}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Name string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if a.Name != "" {
				sc, err := s.Scene(a.Name)
				if err != nil {
					return nil, err
				}
				return map[string]any{"scene": sc}, nil
			}
			list, err := s.Scenes()
			if err != nil {
				return nil, err
			}
			type row struct {
				Name        string `json:"name"`
				Description string `json:"description,omitempty"`
				Actions     int    `json:"actions"`
			}
			rows := []row{}
			for _, sc := range list {
				rows = append(rows, row{sc.Name, sc.Description, len(sc.Actions)})
			}
			return map[string]any{"scenes": rows}, nil
		},
	},

	// ---- control ----
	{
		name: "shelly_scene_run", title: "Run a scene", level: levelControl,
		description: "Run a stored scene: every action in order; a failing action does not stop the others (status ok, partial or failed, per step).",
		schema:      obj([]string{"name"}, map[string]any{"name": str("Scene name")}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Name string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			return s.SceneRunNow(ctx, a.Name)
		},
	},
	{
		name: "shelly_rescan", title: "Scan the network again", level: levelControl,
		description: "Forget the current device list and discover the devices on the LAN again (as ShellyLanMan's Rescan). Takes a few seconds to minutes; then use shelly_list_devices.",
		schema:      obj(nil, map[string]any{}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			if err := decode(args, &struct{}{}); err != nil {
				return nil, err
			}
			s.Rescan()
			return map[string]any{"rescan": "started"}, nil
		},
	},
}
