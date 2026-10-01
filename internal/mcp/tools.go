package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/service"
)

// Service is what the tools need from the device service (service.Devices).
type Service interface {
	List() []model.Device
	Command(ctx context.Context, id string, cmd service.Command) error
	Reboot(ids []string) error
	Firmware(ctx context.Context, ids []string) ([]service.FirmwareRow, error)
	FirmwareUpdate(ctx context.Context, reqs []service.FirmwareRequest) ([]service.ResultLine, error)
	Backup(ctx context.Context, ids []string) ([]service.ResultLine, error)
	Backups(id string) []service.BackupFile
	Samples(ids []string, since int64) map[string][]service.Sample
	DeviceRPC(ctx context.Context, id, method string, params json.RawMessage) (json.RawMessage, error)
	Checklist(ctx context.Context, ids []string) []service.ChecklistRow
}

type tool struct {
	name, title, description string
	schema                   map[string]any
	control                  bool // needs access "control"
	destructive              bool // also needs confirm=true
	run                      func(ctx context.Context, s Service, args json.RawMessage) (any, error)
}

func (t *tool) describe() map[string]any {
	return map[string]any{
		"name":        t.name,
		"title":       t.title,
		"description": t.description,
		"inputSchema": t.schema,
		"annotations": map[string]any{
			"title":           t.title,
			"readOnlyHint":    !t.control,
			"destructiveHint": t.destructive,
			"idempotentHint":  !t.control,
			"openWorldHint":   false, // only devices on this LAN
		},
	}
}

func findTool(name string) *tool {
	for i := range tools {
		if tools[i].name == name {
			return &tools[i]
		}
	}
	return nil
}

// ---- JSON schema helpers ------------------------------------------------------

func obj(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string, enum ...string) map[string]any {
	s := map[string]any{"type": "string", "description": desc}
	if len(enum) > 0 {
		s["enum"] = enum
	}
	return s
}

func num(desc string, min, max float64) map[string]any {
	return map[string]any{"type": "number", "description": desc, "minimum": min, "maximum": max}
}

func integer(desc string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": desc, "minimum": min, "maximum": max}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

var (
	deviceArg  = str("Device: id (MAC), name, host name or IP address")
	devicesArg = map[string]any{"type": "array", "items": deviceArg, "minItems": 1, "maxItems": 100, "description": "Devices: id (MAC), name, host name or IP address each"}
	channelArg = integer("Channel (component id) on the device; default the first one", 0, 16)
	confirmArg = boolean("Must be true: the user has confirmed this action")
)

func decode(args json.RawMessage, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(args)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", errUser, err)
	}
	return nil
}

// ---- device lookup --------------------------------------------------------------

// resolve finds one device by id/MAC, IP, host name or name (case-insensitive).
func resolve(s Service, ref string) (model.Device, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.Device{}, fmt.Errorf("%w: device is required", errUser)
	}
	list := s.List()
	mac := model.NormalizeMAC(ref)
	for _, d := range list {
		if d.ID == mac || d.IP == ref || strings.EqualFold(d.Hostname, ref) {
			return d, nil
		}
	}
	var hits []model.Device
	for _, d := range list {
		if strings.EqualFold(d.Name, ref) {
			hits = append(hits, d)
		}
	}
	if len(hits) == 0 { // a unique partial name match is still clear
		for _, d := range list {
			if d.Name != "" && strings.Contains(strings.ToLower(d.Name), strings.ToLower(ref)) {
				hits = append(hits, d)
			}
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return model.Device{}, fmt.Errorf("%w: no device %q; use shelly_list_devices", errUser, ref)
	}
	names := make([]string, 0, len(hits))
	for _, d := range hits {
		names = append(names, fmt.Sprintf("%s (%s, %s)", d.Name, d.ID, d.IP))
	}
	return model.Device{}, fmt.Errorf("%w: %q matches several devices: %s", errUser, ref, strings.Join(names, "; "))
}

func resolveAll(s Service, refs []string) ([]model.Device, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("%w: devices is required", errUser)
	}
	var out []model.Device
	for _, r := range refs {
		d, err := resolve(s, r)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(out, func(x model.Device) bool { return x.ID == d.ID }) {
			out = append(out, d)
		}
	}
	return out, nil
}

func ids(list []model.Device) []string {
	out := make([]string, len(list))
	for i, d := range list {
		out[i] = d.ID
	}
	return out
}

// module picks the module of one of kinds with component id channel (or the first).
func module(d model.Device, channel *int, kinds ...string) (parse.Module, error) {
	var found []parse.Module
	for _, m := range d.Modules {
		if slices.Contains(kinds, m.Kind) && m.Key != "" {
			found = append(found, m)
		}
	}
	if len(found) == 0 {
		return parse.Module{}, fmt.Errorf("%w: %s has no %s", errUser, label(d), strings.Join(kinds, "/"))
	}
	if channel == nil {
		return found[0], nil
	}
	for _, m := range found {
		if m.Index == *channel {
			return m, nil
		}
	}
	return parse.Module{}, fmt.Errorf("%w: %s has no channel %d", errUser, label(d), *channel)
}

func label(d model.Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Hostname
}

// ---- output shapes ------------------------------------------------------------------

type deviceSummary struct {
	ID             string           `json:"id"`
	Name           string           `json:"name,omitempty"`
	Hostname       string           `json:"hostname,omitempty"`
	Type           string           `json:"type"`
	Generation     string           `json:"generation"`
	IP             string           `json:"ip,omitempty"`
	Status         model.Status     `json:"status"`
	RSSI           int              `json:"rssi_dbm,omitempty"`
	UptimeS        int              `json:"uptime_s,omitempty"`
	TemperatureC   *float64         `json:"temperature_c,omitempty"`
	Cloud          string           `json:"cloud,omitempty"`
	MQTT           string           `json:"mqtt,omitempty"`
	RebootRequired bool             `json:"reboot_required,omitempty"`
	Channels       []parse.Module   `json:"channels,omitempty"`
	Meters         []parse.MeterSet `json:"meters,omitempty"`
	Keyword        string           `json:"keyword,omitempty"`
	Note           string           `json:"note,omitempty"`
	Gateway        string           `json:"gateway,omitempty"`
	LastSeen       string           `json:"last_seen,omitempty"`
	Error          string           `json:"error,omitempty"`
}

func onOff(enabled, connected bool) string {
	switch {
	case connected:
		return "connected"
	case enabled:
		return "enabled, not connected"
	}
	return "off"
}

func summarize(d model.Device, full bool) deviceSummary {
	s := deviceSummary{
		ID: d.ID, Name: d.Name, Hostname: d.Hostname, Type: d.TypeName, Generation: d.Gen, IP: d.IP,
		Status: d.Status, RSSI: d.RSSI, TemperatureC: d.InternalTemp, RebootRequired: d.RebootRequired,
		Keyword: d.Keyword, Gateway: d.Parent, Error: d.Error,
	}
	if d.Uptime > 0 {
		s.UptimeS = d.Uptime
	}
	if d.Gen != model.GenBLU && d.Gen != model.GenBTHome {
		s.Cloud = onOff(d.CloudEnabled, d.CloudConnected)
		s.MQTT = onOff(d.MQTTEnabled, d.MQTTConnected)
	}
	if d.LastSeen > 0 {
		s.LastSeen = time.UnixMilli(d.LastSeen).UTC().Format(time.RFC3339)
	}
	if full {
		s.Channels, s.Meters, s.Note = d.Modules, d.Meters, d.Note
	} else if len(d.Meters) > 0 {
		s.Meters = d.Meters
	}
	return s
}

func results(lines []service.ResultLine) map[string]any {
	return map[string]any{"results": lines}
}

// ---- the tools ---------------------------------------------------------------------

// readRPC: the Gen2+ methods shelly_rpc_read may call — they only read.
var readRPC = regexp.MustCompile(`^[A-Za-z0-9]+\.(Get[A-Za-z]*|List[A-Za-z]*|CheckForUpdate)$`)

var tools = []tool{
	{
		name: "shelly_list_devices", title: "List devices",
		description: "All Shelly devices ShellyLanMan knows on this LAN, with status, type, IP, signal, uptime, temperature and meter readings. Optional filters.",
		schema: obj(nil, map[string]any{
			"status": str("Only devices with this status", "online", "offline", "login", "reading", "error", "ghost"),
			"query":  str("Only devices whose name, host name, type, keyword or IP contains this text"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Status, Query string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			q := strings.ToLower(a.Query)
			out := []deviceSummary{}
			for _, d := range s.List() {
				if a.Status != "" && string(d.Status) != a.Status {
					continue
				}
				if q != "" && !strings.Contains(strings.ToLower(strings.Join([]string{d.Name, d.Hostname, d.TypeName, d.Keyword, d.IP}, " ")), q) {
					continue
				}
				out = append(out, summarize(d, false))
			}
			return map[string]any{"count": len(out), "devices": out}, nil
		},
	},
	{
		name: "shelly_get_device", title: "Get device",
		description: "Everything ShellyLanMan knows about one device: status, channels (relays, lights, covers, inputs, thermostats with their state), meters, notes.",
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
			return map[string]any{"device": summarize(d, true)}, nil
		},
	},
	{
		name: "shelly_get_readings", title: "Get readings",
		description: "Readings kept by ShellyLanMan for one device over the last minutes (up to 24 h): power, energy, voltage, current, temperature, humidity, RSSI … At most 120 points, evenly spread.",
		schema: obj([]string{"device"}, map[string]any{
			"device":  deviceArg,
			"minutes": integer("How far back, default 60", 1, 1440),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device  string
				Minutes int
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if a.Minutes <= 0 {
				a.Minutes = 60
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			since := time.Now().Add(-time.Duration(a.Minutes) * time.Minute).UnixMilli()
			all := s.Samples([]string{d.ID}, since)[d.ID]
			step := max(1, (len(all)+119)/120)
			type point struct {
				Time   string           `json:"time"`
				RSSI   int              `json:"rssi_dbm,omitempty"`
				Temp   *float64         `json:"temperature_c,omitempty"`
				Meters []parse.MeterSet `json:"meters,omitempty"`
			}
			pts := []point{}
			for i := 0; i < len(all); i += step {
				p := all[i]
				pts = append(pts, point{time.UnixMilli(p.T).UTC().Format(time.RFC3339), p.RSSI, p.Temp, p.Meters})
			}
			if n := len(all); n > 0 && (n-1)%step != 0 { // always end with the latest reading
				p := all[n-1]
				pts = append(pts, point{time.UnixMilli(p.T).UTC().Format(time.RFC3339), p.RSSI, p.Temp, p.Meters})
			}
			return map[string]any{"device": d.ID, "name": label(d), "points": pts}, nil
		},
	},
	{
		name: "shelly_firmware_check", title: "Check firmware",
		description: "Current firmware and available stable / beta updates, as reported by the devices themselves.",
		schema:      obj(nil, map[string]any{"devices": devicesArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Devices []string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			var sel []string
			if len(a.Devices) > 0 {
				list, err := resolveAll(s, a.Devices)
				if err != nil {
					return nil, err
				}
				sel = ids(list)
			}
			rows, err := s.Firmware(ctx, sel)
			if err != nil {
				return nil, err
			}
			return map[string]any{"firmware": rows}, nil
		},
	},
	{
		name: "shelly_checklist", title: "Configuration checklist",
		description: "Settings worth checking per device (ShellyScanner's checklist): eco mode, LED, logs, Bluetooth, access point, roaming, Wi-Fi static/DHCP, range extender, scripts, automatic firmware update. " +
			"Cells are as in the original table: true/false, \"✓\"/\"✗\", \"-\" = not applicable to this generation, null = the device does not have the setting (or it could not be read). " +
			"eco: eco mode on. led: the device LED is OFF (Gen1 only; true = LED disabled). logs: false, or the active log targets (\"socket\", \"mqtt\", \"udp\"). " +
			"ble: Gen2+ list of BLU devices this device relays as gateway (empty list = Bluetooth on, nothing relayed), \"✗\" = Bluetooth off; for a BLU device, the gateways that see it. " +
			"ap: own access point on. roaming: RSSI threshold in dBm when on, \"✗\" when off. wifi1/wifi2: \"✓\" static IP, \"✗\" DHCP, \"-\" not configured (e.g. on Ethernet). " +
			"extender: number of clients when the range extender is on, \"✗\" off. scripts: \"total / enabled\". autoFW: update stage (\"stable\"/\"beta\") of the daily update schedule, \"✗\" none.",
		schema: obj(nil, map[string]any{"devices": devicesArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Devices []string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			list := s.List()
			if len(a.Devices) > 0 {
				var err error
				if list, err = resolveAll(s, a.Devices); err != nil {
					return nil, err
				}
			}
			return map[string]any{"checklist": s.Checklist(ctx, ids(list))}, nil
		},
	},
	{
		name: "shelly_list_backups", title: "List backups",
		description: "The configuration backups ShellyLanMan keeps for a device (newest first).",
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
			b := s.Backups(d.ID)
			if b == nil {
				b = []service.BackupFile{}
			}
			return map[string]any{"backups": b}, nil
		},
	},
	{
		name: "shelly_rpc_read", title: "Read with an RPC call",
		description: "Call a read-only RPC method on a Gen2+ device (Gen2, Gen3, Gen4, Pro), e.g. Shelly.GetStatus, Shelly.GetConfig, Switch.GetStatus, Schedule.List, Script.List, KVS.GetMany, Shelly.ListMethods. Only Get*, List* and CheckForUpdate methods are allowed.",
		schema: obj([]string{"device", "method"}, map[string]any{
			"device": deviceArg,
			"method": str("RPC method, e.g. Shelly.GetStatus"),
			"params": map[string]any{"type": "object", "description": "Method parameters, e.g. {\"id\": 0}"},
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Method string
				Params         json.RawMessage
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if !readRPC.MatchString(a.Method) {
				return nil, fmt.Errorf("%w: %q is not a read-only method (Get*, List*, CheckForUpdate)", errUser, a.Method)
			}
			d, err := resolve(s, a.Device)
			if err != nil {
				return nil, err
			}
			raw, err := s.DeviceRPC(ctx, d.ID, a.Method, a.Params)
			if err != nil {
				return nil, err
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			return map[string]any{"device": d.ID, "method": a.Method, "result": v}, nil
		},
	},

	// ---- control ----
	{
		name: "shelly_switch", title: "Switch a relay", control: true,
		description: "Turn a relay (switch, plug) on, off or toggle it.",
		schema: obj([]string{"device", "action"}, map[string]any{
			"device": deviceArg, "channel": channelArg,
			"action": str("What to do", "on", "off", "toggle"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Action string
				Channel        *int
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if !slices.Contains([]string{service.ActionOn, service.ActionOff, service.ActionToggle}, a.Action) {
				return nil, fmt.Errorf("%w: action must be on, off or toggle", errUser)
			}
			return command(ctx, s, a.Device, a.Channel, []string{parse.KindRelay}, service.Command{Action: a.Action})
		},
	},
	{
		name: "shelly_light", title: "Set a light", control: true,
		description: "Switch a light and/or set its brightness, colour, white level or colour temperature. Give only what should change.",
		schema: obj([]string{"device"}, map[string]any{
			"device": deviceArg, "channel": channelArg,
			"on":            boolean("Switch on (true) or off (false)"),
			"brightness":    num("Brightness %", 0, 100),
			"gain":          num("Colour gain % (RGB lights)", 0, 100),
			"rgb":           map[string]any{"type": "array", "items": integer("0–255", 0, 255), "minItems": 3, "maxItems": 3, "description": "Colour [red, green, blue], each 0–255"},
			"white":         integer("White channel 0–255 (RGBW lights, with rgb)", 0, 255),
			"temperature_k": integer("Colour temperature in kelvin (CCT lights)", 1000, 10000),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device       string
				Channel      *int
				On           *bool
				Brightness   *float64
				Gain         *float64
				RGB          []int
				White        *int
				TemperatureK *float64 `json:"temperature_k"`
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			var cmds []service.Command
			if a.Brightness != nil {
				cmds = append(cmds, service.Command{Action: service.ActionBrightness, Value: a.Brightness})
			}
			if a.Gain != nil {
				cmds = append(cmds, service.Command{Action: service.ActionGain, Value: a.Gain})
			}
			if a.RGB != nil {
				if len(a.RGB) != 3 {
					return nil, fmt.Errorf("%w: rgb needs three values", errUser)
				}
				cmds = append(cmds, service.Command{Action: service.ActionColor, RGB: a.RGB, White: a.White})
			} else if a.White != nil {
				v := float64(*a.White)
				cmds = append(cmds, service.Command{Action: service.ActionWhite, Value: &v})
			}
			if a.TemperatureK != nil {
				cmds = append(cmds, service.Command{Action: service.ActionTemp, Value: a.TemperatureK})
			}
			if a.On != nil {
				act := service.ActionOff
				if *a.On {
					act = service.ActionOn
				}
				cmds = append(cmds, service.Command{Action: act})
			}
			if len(cmds) == 0 {
				return nil, fmt.Errorf("%w: nothing to change", errUser)
			}
			var out any
			for _, c := range cmds {
				var err error
				if out, err = command(ctx, s, a.Device, a.Channel, lightKinds, c); err != nil {
					return nil, err
				}
			}
			return out, nil
		},
	},
	{
		name: "shelly_cover", title: "Move a cover", control: true,
		description: "Open, close or stop a roller shutter / cover, or move it to a position (when calibrated).",
		schema: obj([]string{"device", "action"}, map[string]any{
			"device": deviceArg, "channel": channelArg,
			"action":   str("What to do", "open", "close", "stop", "position"),
			"position": num("Position % (0 closed, 100 open) with action position", 0, 100),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device, Action string
				Channel        *int
				Position       *float64
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if !slices.Contains([]string{service.ActionOpen, service.ActionClose, service.ActionStop, service.ActionPosition}, a.Action) {
				return nil, fmt.Errorf("%w: action must be open, close, stop or position", errUser)
			}
			if a.Action == service.ActionPosition && a.Position == nil {
				return nil, fmt.Errorf("%w: position is required", errUser)
			}
			return command(ctx, s, a.Device, a.Channel, []string{parse.KindCover}, service.Command{Action: a.Action, Value: a.Position})
		},
	},
	{
		name: "shelly_thermostat", title: "Set a thermostat", control: true,
		description: "Set the target temperature of a thermostat or TRV, and/or enable or disable it.",
		schema: obj([]string{"device"}, map[string]any{
			"device": deviceArg, "channel": channelArg,
			"target_c": num("Target temperature °C", 4, 35),
			"enabled":  boolean("Enable (true) or disable (false) the thermostat"),
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Device  string
				Channel *int
				TargetC *float64 `json:"target_c"`
				Enabled *bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if a.TargetC == nil && a.Enabled == nil {
				return nil, fmt.Errorf("%w: nothing to change", errUser)
			}
			var out any
			if a.Enabled != nil {
				v := 0.0
				if *a.Enabled {
					v = 1
				}
				var err error
				if out, err = command(ctx, s, a.Device, a.Channel, []string{parse.KindThermostat}, service.Command{Action: service.ActionEnable, Value: &v}); err != nil {
					return nil, err
				}
			}
			if a.TargetC != nil {
				return command(ctx, s, a.Device, a.Channel, []string{parse.KindThermostat}, service.Command{Action: service.ActionTarget, Value: a.TargetC})
			}
			return out, nil
		},
	},
	{
		name: "shelly_backup", title: "Back up configuration", control: true,
		description: "Back up the configuration of devices to ShellyLanMan's backup folder (.sbk, compatible with ShellyScanner). Nothing on the devices changes.",
		schema:      obj([]string{"devices"}, map[string]any{"devices": devicesArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct{ Devices []string }
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			list, err := resolveAll(s, a.Devices)
			if err != nil {
				return nil, err
			}
			lines, err := s.Backup(ctx, ids(list))
			if err != nil {
				return nil, err
			}
			return results(lines), nil
		},
	},
	{
		name: "shelly_reboot", title: "Reboot devices", control: true, destructive: true,
		description: "Reboot devices. Their outputs may switch while they restart. Ask the user first and pass confirm=true.",
		schema:      obj([]string{"devices", "confirm"}, map[string]any{"devices": devicesArg, "confirm": confirmArg}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Devices []string
				Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if !a.Confirm {
				return nil, fmt.Errorf("%w: reboot needs confirm=true after the user agreed", errUser)
			}
			list, err := resolveAll(s, a.Devices)
			if err != nil {
				return nil, err
			}
			if err := s.Reboot(ids(list)); err != nil {
				return nil, err
			}
			return map[string]any{"rebooting": ids(list)}, nil
		},
	},
	{
		name: "shelly_firmware_update", title: "Update firmware", control: true, destructive: true,
		description: "Update the firmware of devices to the stable or beta version they report (see shelly_firmware_check). Devices restart. Off-line devices get a deferred update. Ask the user first and pass confirm=true.",
		schema: obj([]string{"devices", "stage", "confirm"}, map[string]any{
			"devices": devicesArg,
			"stage":   str("Which firmware", "stable", "beta"),
			"confirm": confirmArg,
		}),
		run: func(ctx context.Context, s Service, args json.RawMessage) (any, error) {
			var a struct {
				Devices []string
				Stage   string
				Confirm bool
			}
			if err := decode(args, &a); err != nil {
				return nil, err
			}
			if !a.Confirm {
				return nil, fmt.Errorf("%w: firmware update needs confirm=true after the user agreed", errUser)
			}
			if a.Stage != service.StageStable && a.Stage != service.StageBeta {
				return nil, fmt.Errorf("%w: stage must be stable or beta", errUser)
			}
			list, err := resolveAll(s, a.Devices)
			if err != nil {
				return nil, err
			}
			var reqs []service.FirmwareRequest
			for _, d := range list {
				reqs = append(reqs, service.FirmwareRequest{ID: d.ID, Stage: a.Stage})
			}
			lines, err := s.FirmwareUpdate(ctx, reqs)
			if err != nil {
				return nil, err
			}
			return results(lines), nil
		},
	},
}

var lightKinds = []string{parse.KindLight, parse.KindRGB, parse.KindRGBW, parse.KindCCT, parse.KindRGBCCT}

// command runs one Command on the chosen module and returns the device's
// channels as they are afterwards.
func command(ctx context.Context, s Service, ref string, channel *int, kinds []string, cmd service.Command) (any, error) {
	d, err := resolve(s, ref)
	if err != nil {
		return nil, err
	}
	m, err := module(d, channel, kinds...)
	if err != nil {
		return nil, err
	}
	cmd.Key = m.Key
	if err := s.Command(ctx, d.ID, cmd); err != nil {
		return nil, err
	}
	after := d
	if again, err := resolve(s, d.ID); err == nil {
		after = again
	}
	for _, x := range after.Modules {
		if x.Key == m.Key {
			return map[string]any{"device": d.ID, "name": label(d), "channel": x}, nil
		}
	}
	return map[string]any{"device": d.ID, "name": label(d), "done": cmd.Action}, nil
}
