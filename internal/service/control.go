// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// the commands of the "Command" column (view/DevicesCommandCellEditor,
// view/lightsEditor/*) as implemented by the modules g1/modules Relay,
// Roller, LightWhite, LightRGBW, LightBulbRGB, ThermostatG1, Actions;
// g2/modules Relay, Roller, LightWhite, LightCCT, LightRGB, LightRGBW,
// ThermostatG2, CBreakerPro, Webhooks; g3/modules LightRGBCCT, Camera,
// XT1Thermostat; blu/BluTRV; and the reboot action (MainView.rebootAction,
// Devices.reboot).

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Command is one action on one module of a device.
type Command struct {
	Key    string   `json:"key"`    // parse.Module.Key
	Action string   `json:"action"` // see the Action* constants
	Value  *float64 `json:"value,omitempty"`
	RGB    []int    `json:"rgb,omitempty"`
	White  *int     `json:"white,omitempty"` // with ActionColor on RGBW lights
	Event  int      `json:"event,omitempty"` // ActionEvent: index in Module.Events
	// Confirm is required for the circuit breaker (ShellyScanner asks
	// "Confirm you want to toggle the circuit status?").
	Confirm bool `json:"confirm,omitempty"`
}

// Command actions.
const (
	ActionOn         = "on"
	ActionOff        = "off"
	ActionToggle     = "toggle"
	ActionBrightness = "brightness" // Value 0–100
	ActionGain       = "gain"       // Value 0–100 (RGB)
	ActionWhite      = "white"      // Value 0–255 (RGBW)
	ActionColor      = "color"      // RGB (+ White for RGBW)
	ActionTemp       = "temp"       // Value K
	ActionMode       = "mode"       // Value 1 colour, 0 white (RGBCCT)
	ActionOpen       = "open"
	ActionClose      = "close"
	ActionStop       = "stop"
	ActionPosition   = "position" // Value 0–100
	ActionTarget     = "target"   // Value °C
	ActionEnable     = "enable"   // Value 1/0 (thermostat)
	ActionPrivacy    = "privacy"  // Value 1/0 (camera)
	ActionEvent      = "event"    // run the URLs of Module.Events[Event]
)

var (
	// ErrBadCommand: the module does not exist or does not support the action.
	ErrBadCommand = errors.New("unsupported command")
	// ErrConfirm: the command needs confirm=true.
	ErrConfirm = errors.New("confirmation required")
)

// Command runs cmd on device id and refreshes the device's state.
func (m *Devices) Command(ctx context.Context, id string, cmd Command) error {
	m.mu.Lock()
	e, ok := m.devs[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	if e.conn == nil || e.dev.Status == model.StatusGhost {
		m.mu.Unlock()
		return ErrNoConnection
	}
	var mod *parse.Module
	for i := range e.dev.Modules {
		if e.dev.Modules[i].Key == cmd.Key && cmd.Key != "" {
			mc := e.dev.Modules[i]
			mod = &mc
			break
		}
	}
	gen1, blu := e.info.Gen == 0, e.blu
	addr := e.dev.Address()
	if blu != nil {
		if gw, ok := m.devs[blu.gwID]; ok {
			addr = gw.dev.Address()
		}
	}
	m.mu.Unlock()
	if mod == nil {
		return ErrBadCommand
	}

	var err error
	switch {
	case cmd.Action == ActionEvent:
		err = m.runEvent(ctx, mod, cmd.Event, addr)
	case blu != nil && blu.trv:
		err = m.trvCommand(ctx, e, mod, cmd)
	case gen1:
		err = gen1Command(ctx, e.conn, mod, cmd)
	default:
		err = gen2Command(ctx, e.conn, mod, cmd)
	}
	if err != nil {
		return err
	}
	if cmd.Action != ActionEvent {
		m.refreshStatus(ctx, e) // the original updates the module from the answer; one status read does the same
	}
	return nil
}

func val(cmd Command) (float64, error) {
	if cmd.Value == nil || math.IsNaN(*cmd.Value) || math.IsInf(*cmd.Value, 0) {
		return 0, fmt.Errorf("%w: %s needs a value", ErrBadCommand, cmd.Action)
	}
	return *cmd.Value, nil
}

func intIn(cmd Command, lo, hi int) (int, error) {
	v, err := val(cmd)
	if err != nil {
		return 0, err
	}
	i := int(math.Round(v))
	if i < lo || i > hi {
		return 0, fmt.Errorf("%w: %s %d out of range %d–%d", ErrBadCommand, cmd.Action, i, lo, hi)
	}
	return i, nil
}

func rgbOf(cmd Command) (r, g, b int, err error) {
	if len(cmd.RGB) != 3 {
		return 0, 0, 0, fmt.Errorf("%w: color needs rgb [r,g,b]", ErrBadCommand)
	}
	for _, c := range cmd.RGB {
		if c < 0 || c > 255 {
			return 0, 0, 0, fmt.Errorf("%w: color component out of range", ErrBadCommand)
		}
	}
	return cmd.RGB[0], cmd.RGB[1], cmd.RGB[2], nil
}

func isOn(m *parse.Module) bool { return m.On != nil && *m.On }

// targetIn checks a thermostat target against the module range.
func targetIn(m *parse.Module, cmd Command) (float64, error) {
	t, err := val(cmd)
	if err != nil {
		return 0, err
	}
	if m.Min != nil && m.Max != nil && (t < *m.Min-1e-9 || t > *m.Max+1e-9) {
		return 0, fmt.Errorf("%w: target %.1f out of range %.1f–%.1f", ErrBadCommand, t, *m.Min, *m.Max)
	}
	return t, nil
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func turn(action string) (string, bool) {
	switch action {
	case ActionOn:
		return "on", true
	case ActionOff:
		return "off", true
	case ActionToggle:
		return "toggle", true
	}
	return "", false
}

// gen1Command: REST GETs of the g1 modules.
func gen1Command(ctx context.Context, c *shelly.Conn, m *parse.Module, cmd Command) error {
	get := func(path string) error { _, err := c.Get(ctx, path); return err }
	prefix := "/" + m.Key // relay/0, roller/0, light/0, white/2, color/0, thermostats/0, input/1
	switch m.Kind {
	case parse.KindRelay:
		if t, ok := turn(cmd.Action); ok {
			return get(prefix + "?turn=" + t)
		}
	case parse.KindCover:
		switch cmd.Action {
		case ActionOpen, ActionClose, ActionStop:
			return get(prefix + "?go=" + cmd.Action)
		case ActionPosition:
			p, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return get(prefix + "?go=to_pos&roller_pos=" + strconv.Itoa(p))
		}
	case parse.KindLight:
		if t, ok := turn(cmd.Action); ok {
			return get(prefix + "?turn=" + t)
		}
		if cmd.Action == ActionBrightness {
			b, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return get(prefix + "?brightness=" + strconv.Itoa(b))
		}
	case parse.KindRGBW: // LightRGBW (RGBW2 colour): /color/0, colours via /light/0
		if t, ok := turn(cmd.Action); ok {
			return get(prefix + "?turn=" + t)
		}
		switch cmd.Action {
		case ActionGain:
			b, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return get(prefix + "?gain=" + strconv.Itoa(b))
		case ActionWhite:
			w, err := intIn(cmd, 0, 255)
			if err != nil {
				return err
			}
			return get(prefix + "?white=" + strconv.Itoa(w))
		case ActionColor:
			r, g, b, err := rgbOf(cmd)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/light/%d?red=%d&green=%d&blue=%d", m.Index, r, g, b)
			if cmd.White != nil {
				path += "&white=" + strconv.Itoa(*cmd.White)
			}
			return get(path)
		}
	case parse.KindRGBCCT: // LightBulbRGB: /light/0
		if t, ok := turn(cmd.Action); ok {
			// LightBulbRGB.change sends "/light/?turn=…" without the index;
			// we always send the index (FEATURE_PARITY O14).
			return get(prefix + "?turn=" + t)
		}
		switch cmd.Action {
		case ActionBrightness, ActionGain:
			b, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return get(prefix + "?" + cmd.Action + "=" + strconv.Itoa(b))
		case ActionTemp:
			k, err := intIn(cmd, m.TMin, m.TMax)
			if err != nil {
				return err
			}
			return get(prefix + "?temp=" + strconv.Itoa(k))
		case ActionColor:
			r, g, b, err := rgbOf(cmd)
			if err != nil {
				return err
			}
			return get(fmt.Sprintf("%s?red=%d&green=%d&blue=%d", prefix, r, g, b))
		case ActionMode:
			v, err := intIn(cmd, 0, 1)
			if err != nil {
				return err
			}
			return get("/settings?mode=" + map[int]string{1: "color", 0: "white"}[v])
		}
	case parse.KindThermostat: // ThermostatG1 (TRV): target only
		if cmd.Action == ActionTarget {
			t, err := targetIn(m, cmd)
			if err != nil {
				return err
			}
			return get("/settings/thermostats/0?target_t=" + num(t))
		}
	}
	return fmt.Errorf("%w: %s on %s", ErrBadCommand, cmd.Action, m.Kind)
}

// gen2Command: the RPC calls of the g2/g3 modules — GET /rpc/<Method>?… where
// ShellyScanner uses getJSON, POST /rpc where it uses postCommand.
func gen2Command(ctx context.Context, c *shelly.Conn, m *parse.Module, cmd Command) error {
	get := func(path string) error { _, err := c.Get(ctx, path); return err }
	post := func(method string, params any) error { _, err := c.Call(ctx, method, params); return err }
	comp, idxStr, _ := strings.Cut(m.Key, ":")
	idx, _ := strconv.Atoi(idxStr)
	set := func(method, extra string) error {
		return get(fmt.Sprintf("/rpc/%s.Set?id=%d%s", method, idx, extra))
	}
	onOff := func(method string) (bool, error) { // change(on) / toggle = change(!isOn)
		switch cmd.Action {
		case ActionOn:
			return true, set(method, "&on=true")
		case ActionOff:
			return true, set(method, "&on=false")
		case ActionToggle:
			return true, set(method, "&on="+strconv.FormatBool(!isOn(m)))
		}
		return false, nil
	}
	switch m.Kind {
	case parse.KindRelay:
		if comp == "boolean" { // XT1 ST802 "dry": humidity on/off = thermostat enable
			on := !isOn(m)
			switch cmd.Action {
			case ActionOn:
				on = true
			case ActionOff:
				on = false
			case ActionToggle:
			default:
				return fmt.Errorf("%w: %s on %s", ErrBadCommand, cmd.Action, m.Kind)
			}
			return post("Boolean.Set", map[string]any{"id": idx, "value": on})
		}
		switch cmd.Action {
		case ActionToggle:
			return get(fmt.Sprintf("/rpc/Switch.Toggle?id=%d", idx))
		case ActionOn, ActionOff:
			return set("Switch", "&on="+strconv.FormatBool(cmd.Action == ActionOn))
		}
	case parse.KindCover: // g2 Roller uses the Gen1-compatible /roller/N endpoint
		switch cmd.Action {
		case ActionOpen, ActionClose, ActionStop:
			return get(fmt.Sprintf("/roller/%d?go=%s", idx, cmd.Action))
		case ActionPosition:
			p, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return get(fmt.Sprintf("/roller/%d?go=to_pos&roller_pos=%d", idx, p))
		}
	case parse.KindLight, parse.KindCCT:
		method := "Light"
		if m.Kind == parse.KindCCT {
			method = "CCT"
		}
		if done, err := onOff(method); done {
			return err
		}
		switch cmd.Action {
		case ActionBrightness:
			b, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return set(method, "&brightness="+strconv.Itoa(b))
		case ActionTemp:
			if m.Kind == parse.KindCCT {
				k, err := intIn(cmd, m.TMin, m.TMax)
				if err != nil {
					return err
				}
				return set(method, "&ct="+strconv.Itoa(k))
			}
		}
	case parse.KindRGB, parse.KindRGBW, parse.KindRGBCCT:
		method := map[string]string{parse.KindRGB: "RGB", parse.KindRGBW: "RGBW", parse.KindRGBCCT: "RGBCCT"}[m.Kind]
		if done, err := onOff(method); done {
			return err
		}
		switch cmd.Action {
		case ActionGain, ActionBrightness:
			b, err := intIn(cmd, 0, 100)
			if err != nil {
				return err
			}
			return set(method, "&brightness="+strconv.Itoa(b))
		case ActionWhite:
			if m.Kind == parse.KindRGBW {
				w, err := intIn(cmd, 0, 255)
				if err != nil {
					return err
				}
				return set(method, "&white="+strconv.Itoa(w))
			}
		case ActionColor:
			r, g, b, err := rgbOf(cmd)
			if err != nil {
				return err
			}
			extra := ""
			if m.Kind == parse.KindRGBW && cmd.White != nil {
				extra = "&white=" + strconv.Itoa(*cmd.White)
			}
			return set(method, fmt.Sprintf("%s&rgb=[%d,%d,%d]", extra, r, g, b))
		case ActionTemp:
			if m.Kind == parse.KindRGBCCT {
				k, err := intIn(cmd, m.TMin, m.TMax)
				if err != nil {
					return err
				}
				return set(method, "&ct="+strconv.Itoa(k))
			}
		case ActionMode:
			if m.Kind == parse.KindRGBCCT {
				v, err := intIn(cmd, 0, 1)
				if err != nil {
					return err
				}
				return post("RGBCCT.Set", map[string]any{"id": idx, "mode": map[int]string{1: "rgb", 0: "cct"}[v]})
			}
		}
	case parse.KindThermostat:
		if comp == "xt1" { // xt1:<enableId>:<targetId>:<C|F>
			parts := strings.Split(m.Key, ":")
			if len(parts) != 4 {
				break
			}
			enID, _ := strconv.Atoi(parts[1])
			tgID, _ := strconv.Atoi(parts[2])
			switch cmd.Action {
			case ActionTarget:
				t, err := targetIn(m, cmd)
				if err != nil {
					return err
				}
				if parts[3] == "F" {
					t = math.Round(t*18+320) / 10
				}
				return post("Number.Set", map[string]any{"id": tgID, "value": t})
			case ActionEnable:
				v, err := intIn(cmd, 0, 1)
				if err != nil {
					return err
				}
				return post("Boolean.Set", map[string]any{"id": enID, "value": v == 1})
			}
			break
		}
		switch cmd.Action { // ThermostatG2 (Wall Display)
		case ActionTarget:
			t, err := targetIn(m, cmd)
			if err != nil {
				return err
			}
			return post("Thermostat.SetConfig", map[string]any{"id": idx, "config": map[string]any{"target_C": t}})
		case ActionEnable:
			v, err := intIn(cmd, 0, 1)
			if err != nil {
				return err
			}
			return post("Thermostat.SetConfig", map[string]any{"id": idx, "config": map[string]any{"enable": v == 1}})
		}
	case parse.KindBreaker:
		if cmd.Action == ActionToggle {
			if m.Locked != nil && *m.Locked {
				return fmt.Errorf("%w: the breaker is locked", ErrBadCommand)
			}
			if !cmd.Confirm {
				return ErrConfirm
			}
			return post("CB.Set", map[string]any{"id": idx, "output": !isOn(m)})
		}
	case parse.KindCamera:
		if cmd.Action == ActionPrivacy {
			v, err := intIn(cmd, 0, 1)
			if err != nil {
				return err
			}
			return post("Camera.Set", map[string]any{"id": idx, "privacy": v == 1})
		}
	}
	return fmt.Errorf("%w: %s on %s", ErrBadCommand, cmd.Action, m.Kind)
}

// trvCommand: BluTRV.setTargetTemp / setEnabled through BluTrv.Call on the
// gateway. After a target change the next remote status read keeps the new
// target (BluTRV.tempChanged): the TRV reports it only after its next sync.
func (m *Devices) trvCommand(ctx context.Context, e *entry, mod *parse.Module, cmd Command) error {
	idx, _ := strconv.Atoi(e.blu.index)
	switch cmd.Action {
	case ActionTarget:
		t, err := targetIn(mod, cmd)
		if err != nil {
			return err
		}
		if _, err := e.blu.gw.Call(ctx, "BluTrv.Call", map[string]any{"id": idx, "method": "TRV.SetTarget",
			"params": map[string]any{"id": 0, "target_C": t}}); err != nil {
			return err
		}
		m.mu.Lock()
		e.blu.trvTarget = &t
		m.mu.Unlock()
		return nil
	case ActionEnable:
		v, err := intIn(cmd, 0, 1)
		if err != nil {
			return err
		}
		_, err = e.blu.gw.Call(ctx, "BluTrv.Call", map[string]any{"id": idx, "method": "TRV.SetConfig",
			"params": map[string]any{"id": 0, "config": map[string]any{"enable": v == 1}}})
		if err == nil {
			m.mu.Lock()
			e.rawConfig, _ = json.Marshal(map[string]any{"config": map[string]any{"trv:0": map[string]any{"enable": v == 1}}})
			m.mu.Unlock()
		}
		return err
	}
	return fmt.Errorf("%w: %s on %s", ErrBadCommand, cmd.Action, mod.Kind)
}

// EventClient performs the URLs of input actions (tests replace it).
var EventClient = &http.Client{Timeout: 10 * time.Second}

var loopback = regexp.MustCompile(`^http://127\.0\.0\.1|^http://localhost`)

// runEvent performs the URLs of an input action, as ShellyScanner does when
// its event button is used: Gen1 Actions.execute GETs the URLs as they are;
// Gen2+ Webhook.execute first rewrites http://127.0.0.1 and http://localhost
// to the device (for BLU: the gateway) address.
func (m *Devices) runEvent(ctx context.Context, mod *parse.Module, i int, addr string) error {
	if mod.Kind != parse.KindInput || i < 0 || i >= len(mod.Events) {
		return ErrBadCommand
	}
	ev := mod.Events[i]
	if !ev.Enabled {
		return fmt.Errorf("%w: the action is not enabled", ErrBadCommand)
	}
	gen1 := strings.HasSuffix(ev.Event, "_url")
	for _, u := range ev.URLs {
		if !gen1 {
			u = loopback.ReplaceAllString(u, "http://"+addr)
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return fmt.Errorf("action URL %q: unsupported scheme", u)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return fmt.Errorf("action URL: %w", err)
		}
		resp, err := EventClient.Do(req)
		if err != nil {
			return fmt.Errorf("action URL: %w", err)
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
	}
	return nil
}

// RebootPause is how long refresh stays paused after a reboot command
// (Devices.reboot: 3 s).
var RebootPause = 3 * time.Second

// Rebootable reports whether ShellyScanner offers reboot for d: not a stored
// device and not a BLU device other than the TRV.
func Rebootable(d model.Device) bool {
	return d.Status != model.StatusGhost && d.Gen != model.GenBTHome
}

// Reboot restarts the given devices (MainView.rebootAction + Devices.reboot):
// per device, refresh is paused, the status shows "reading", the reboot is
// sent and after 3 s refresh resumes. It returns at once; the devices'
// states arrive as events.
func (m *Devices) Reboot(ids []string) error {
	m.mu.Lock()
	var targets []*entry
	for _, id := range ids {
		e, ok := m.devs[id]
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		if !Rebootable(e.dev) || e.conn == nil {
			m.mu.Unlock()
			return fmt.Errorf("%w: %s cannot be rebooted", ErrBadCommand, id)
		}
		targets = append(targets, e)
	}
	run := m.run
	m.mu.Unlock()
	for _, e := range targets {
		m.mu.Lock()
		e.rebooting = true
		m.mu.Unlock()
		m.setStatus(e, model.StatusReading)
		go func(e *entry) {
			defer func() {
				m.mu.Lock()
				e.rebooting = false
				m.mu.Unlock()
				select { // read the device again now instead of waiting for the next tick
				case e.now <- struct{}{}:
				default:
				}
			}()
			if err := m.reboot(run, e); err != nil {
				m.log.Debug("reboot", "device", e.dev.ID, "err", err)
				return
			}
			m.setStatus(e, model.StatusReading)
			select {
			case <-time.After(RebootPause):
			case <-run.Done():
			}
		}(e)
	}
	return nil
}

func (m *Devices) reboot(ctx context.Context, e *entry) error {
	switch {
	case e.blu != nil && e.blu.trv:
		_, err := e.blu.gw.Get(ctx, "/rpc/BluTrv.call?id="+e.blu.index+"&method=Shelly.Reboot")
		return err
	case e.info.Gen == 0:
		_, err := e.conn.Get(ctx, "/reboot")
		if err == nil {
			m.apply(e, func(d *model.Device) { d.RebootRequired = false })
		}
		return err
	default:
		_, err := e.conn.Get(ctx, "/rpc/Shelly.Reboot")
		return err
	}
}
