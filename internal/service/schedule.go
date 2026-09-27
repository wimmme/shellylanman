// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/scheduler/gen2plus/MethodHints, G2JobPanel (test button),
// blu/BluTRV.getTRVJSON/postTRVCommand, RestoreAction.readBackupFile.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/sbk"
)

// DeviceRPC runs one RPC method on a Gen2+ device (the scheduler's calls and
// the "test method" button). For a BLU TRV the call goes to its gateway as
// BluTrv.Call {id, method, params} (BluTRV.getTRVJSON).
func (m *Devices) DeviceRPC(ctx context.Context, id, method string, params json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(method) == "" {
		return nil, invalid("method")
	}
	if len(params) == 0 || string(params) == "null" {
		params = json.RawMessage("{}")
	}
	e, err := m.entryFor(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	d, conn, blu := e.dev, e.conn, e.blu
	m.mu.Unlock()
	switch {
	case blu != nil && blu.trv:
		idx, _ := strconv.Atoi(blu.index)
		return blu.gw.Call(ctx, "BluTrv.Call", map[string]any{"id": idx, "method": method, "params": params})
	case d.Gen == "1" || d.Gen == model.GenBLU || d.Gen == model.GenBTHome || !d.Managed:
		return nil, fmt.Errorf("%w: Gen2+ devices only", ErrBadCommand)
	case conn == nil || d.Status == model.StatusGhost:
		return nil, ErrNoConnection
	}
	return conn.Call(ctx, method, params)
}

// MethodHint is one entry of the scheduler's "hint method" menu.
type MethodHint struct {
	Name   string `json:"name"`
	Method string `json:"method,omitempty"`
	Params string `json:"params,omitempty"` // the parameters without the outer braces, as the text field shows them
}

// ScheduleHints: MethodHints.generate — actions for the device's switches,
// covers, lights, RGB(W), CCT, RGBCCT, cameras and XT1 thermostat components.
func (m *Devices) ScheduleHints(ctx context.Context, id string) ([]MethodHint, error) {
	c, d, err := m.g2conn(id)
	if err != nil {
		return nil, err
	}
	comps, err := getComponents(ctx, c, `?include=["config"]`)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	variant := m.devs[id].info.Svc0Type
	m.mu.Unlock()
	enableID, targetID := "", ""
	if d.TypeID == "XT1" {
		switch variant {
		case "linkedgo-st1820-floor-thermostat":
			enableID, targetID = "202", "202"
		case "linkedgo-st-802-hvac":
			enableID, targetID = "201", "203"
		}
	}
	var out []MethodHint
	add := func(base, name, method, params string) {
		out = append(out, MethodHint{Name: base + name, Method: method, Params: params})
	}
	for _, comp := range comps {
		cfg, _ := ojson.Parse(comp.Config)
		idx := cfg.Get("id").Int()
		sid := `"id":` + strconv.Itoa(idx)
		base := cfg.Get("name").Text()
		if base == "" {
			base = comp.Key
		}
		base += " - "
		typ, _, _ := strings.Cut(comp.Key, ":")
		switch typ {
		case "switch":
			add(base, "On", "switch.set", sid+`,"on":true`)
			add(base, "Off", "switch.set", sid+`,"on":false`)
			add(base, "Toggle", "switch.toggle", sid)
		case "cover":
			add(base, "Open", "Cover.Open", sid)
			add(base, "Close", "Cover.Close", sid)
			add(base, "Go 50%", "Cover.GoToPosition", sid+`,"pos":50`)
			add(base, "Stop", "Cover.Stop", sid)
			if cfg.Get("slat").NonNull() {
				add(base, "Go 50%, Slat 50%", "Cover.GoToPosition", sid+`,"pos":50,"slat_pos":50`)
			}
		case "light":
			add(base, "On", "Light.Set", sid+`,"on":true`)
			add(base, "Off", "Light.Set", sid+`,"on":false`)
			add(base, "Toggle", "Light.Toggle", sid)
			add(base, "On 50%", "Light.Set", sid+`,"on":true,"brightness":50`)
		case "rgb":
			add(base, "On", "RGB.Set", sid+`,"on":true`)
			add(base, "Off", "RGB.Set", sid+`,"on":false`)
			add(base, "Toggle", "RGB.Toggle", sid)
			add(base, "On 50%", "RGB.Set", sid+`,"on":true,"brightness":50`)
			add(base, "On, red", "RGB.Set", sid+`,"on":true,"rgb":[255,0,0]`)
			add(base, "On, green", "RGB.Set", sid+`,"on":true,"rgb":[0,255,0]`)
			add(base, "On, blu", "RGB.Set", sid+`,"on":true,"rgb":[0,0,255]`)
		case "rgbw":
			add(base, "On", "RGBW.Set", sid+`,"on":true`)
			add(base, "Off", "RGBW.Set", sid+`,"on":false`)
			add(base, "Toggle", "RGBW.Toggle", sid)
			add(base, "On 50%", "RGBW.Set", sid+`,"on":true,"brightness":50`)
			add(base, "On, red", "RGBW.Set", sid+`,"on":true,"white":0,"rgb":[255,0,0]`)
			add(base, "On, green", "RGBW.Set", sid+`,"on":true,"white":0,"rgb":[0,255,0]`)
			add(base, "On, blu", "RGBW.Set", sid+`,"on":true,"white":0,"rgb":[0,0,255]`)
			add(base, "On, white", "RGBW.Set", sid+`,"on":true,"rgb":[0,0,0],"white":255`)
		case "cct":
			add(base, "On", "CCT.Set", sid+`,"on":true`)
			add(base, "Off", "CCT.Set", sid+`,"on":false`)
			add(base, "Toggle", "CCT.Toggle", sid)
			add(base, "On 50%", "CCT.Set", sid+`,"on":true,"brightness":50`)
			add(base, "On, 4000K", "CCT.Set", sid+`,"on":true,"ct":4000`)
		case "rgbcct":
			add(base, "On", "RGBCCT.Set", sid+`,"on":true`)
			add(base, "Off", "RGBCCT.Set", sid+`,"on":false`)
			add(base, "Toggle", "RGBCCT.Toggle", sid)
			add(base, "On 50%", "RGBCCT.Set", sid+`,"on":true,"brightness":50`)
			add(base, "To CCT", "RGBCCT.Set", sid+`,"mode":"cct"`)
			add(base, "To RGB", "RGBCCT.Set", sid+`,"mode":"rgb"`)
			add(base, "On, red", "RGBCCT.Set", sid+`,"mode":"rgb","on":true,"rgb":[255,0,0]`)
			add(base, "On, 4000K", "RGBCCT.Set", sid+`,"mode":"cct","on":true,"ct":4000`)
		case "camera":
			add(base, "Capture image", "Camera.CaptureImage", sid)
			add(base, "Start recording", "Camera.StartRecording", sid)
			add(base, "Stop recording", "Camera.StopRecording", sid)
			add(base, "Start recording 30s", "Camera.StartRecording", sid+`,"duration":30`)
		default:
			if targetID != "" && comp.Key == "number:"+targetID {
				add(base, "20", "number.Set", `"id":`+targetID+`,"value":20`)
			} else if enableID != "" && comp.Key == "boolean:"+enableID {
				add(base, "yes", "boolean.Set", `"id":`+enableID+`,"value":true`)
				add(base, "no", "boolean.Set", `"id":`+enableID+`,"value":false`)
			}
		}
	}
	if len(out) == 0 {
		out = []MethodHint{{Name: "No hints"}}
	}
	return out, nil
}

// BackupJSON returns the JSON entries of a .sbk (scripts excluded), for the
// scheduler's "Load ..." (readBackupFile).
func BackupJSON(data []byte) (map[string]*ojson.Value, error) {
	files, err := sbk.Read(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	out := map[string]*ojson.Value{}
	for name, v := range files {
		if !strings.HasSuffix(name, ".mjs.json") {
			out[name] = v
		}
	}
	return out, nil
}
