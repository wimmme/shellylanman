// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// the info requests of each device kind (getInfoRequests) and the logs
// dialogs (view/DialogDeviceLogsG1, view/DialogDeviceLogsG2).

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/wimmme/shellylanman/internal/shelly"
)

// InfoRequest is one tab of the device info panel.
type InfoRequest struct {
	Name string `json:"name"` // tab title
	Path string `json:"path"` // request sent to the device (or the gateway)
}

// ErrNoConnection: the device has no connection (archived or unmanaged).
var ErrNoConnection = errors.New("device not connected")

// InfoRequests lists the info tabs of a device (ShellyScanner getInfoRequests).
func (m *Devices) InfoRequests(id string) ([]InfoRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.devs[id]
	if !ok {
		return nil, ErrNotFound
	}
	var paths []string
	switch {
	case e.conn == nil:
		paths = []string{"/shelly"}
	case e.blu != nil && e.blu.trv:
		i := e.blu.index
		paths = []string{"/rpc/BluTrv.GetRemoteDeviceInfo?id=" + i, "/rpc/BluTrv.GetConfig?id=" + i, "/rpc/BluTrv.GetRemoteConfig?id=" + i,
			"/rpc/BluTrv.GetStatus?id=" + i, "/rpc/BluTrv.GetRemoteStatus?id=" + i, "/rpc/BluTrv.CheckForUpdates?id=" + i,
			"/rpc/BluTrv.Call?id=" + i + "&method=%22TRV.ListScheduleRules%22&params=%7B%22id%22:0%7D"}
	case e.blu != nil:
		i := e.blu.index
		paths = []string{"/rpc/BTHomeDevice.GetConfig?id=" + i, "/rpc/BTHomeDevice.GetStatus?id=" + i, "/rpc/BTHomeDevice.GetKnownObjects?id=" + i}
		var objs struct {
			Objects []struct {
				ObjID     int    `json:"obj_id"`
				Component string `json:"component"`
			} `json:"objects"`
		}
		_ = json.Unmarshal(e.blu.known, &objs)
		for _, o := range objs.Objects {
			if strings.HasPrefix(o.Component, keyBTHomeSensor) {
				sid := strings.TrimPrefix(o.Component, keyBTHomeSensor)
				paths = append(paths, "/rpc/BTHomeSensor.GetConfig?id="+sid, "/rpc/BTHomeSensor.GetStatus?id="+sid)
			}
		}
	case e.info.Gen == 0:
		paths = []string{"/shelly", "/settings", "/settings/actions", "/status"}
	case e.dev.Battery:
		paths = []string{"/rpc/Shelly.GetDeviceInfo?ident=true", "/rpc/Shelly.GetConfig", "/rpc/Shelly.GetStatus", "/rpc/Shelly.CheckForUpdate",
			"/rpc/Webhook.List", "/rpc/KVS.GetMany", "/rpc/Shelly.GetComponents"}
	default:
		paths = []string{"/rpc/Shelly.GetDeviceInfo?ident=true", "/rpc/Shelly.GetConfig", "/rpc/Shelly.GetStatus", "/rpc/Shelly.CheckForUpdate",
			"/rpc/Schedule.List", "/rpc/Webhook.List", "/rpc/Script.List", "/rpc/WiFi.ListAPClients", "/rpc/KVS.GetMany",
			"/rpc/Shelly.GetComponents", "/rpc/BLE.CloudRelay.ListInfos"}
		if len(e.rawPeriph) > 0 {
			paths = append(paths, "/rpc/SensorAddon.GetPeripherals")
		}
	}
	out := make([]InfoRequest, 0, len(paths))
	for _, p := range paths {
		out = append(out, InfoRequest{Name: infoName(p), Path: p})
	}
	return out, nil
}

// infoName: "/rpc/Shelly.GetConfig" → "Shelly.GetConfig", "/settings/actions" → "settings/actions".
func infoName(p string) string {
	p = strings.TrimPrefix(p, "/rpc/")
	p = strings.TrimPrefix(p, "/")
	if strings.Contains(p, "method=%22") { // BluTrv.Call
		if i := strings.Index(p, "method=%22"); i >= 0 {
			rest := p[i+len("method=%22"):]
			if j := strings.Index(rest, "%22"); j >= 0 {
				return rest[:j]
			}
		}
	}
	name, _, _ := strings.Cut(p, "?")
	return name
}

// InfoResult is the answer to one info request.
type InfoResult struct {
	Data   json.RawMessage `json:"data"`
	Stored bool            `json:"stored"` // true: last answer kept from when the device was awake
}

// Info performs one info request. When a battery device sleeps, the last
// stored answer is returned instead (BatteryDeviceInterface.getStoredJSON).
func (m *Devices) Info(ctx context.Context, id string, index int) (InfoResult, error) {
	reqs, err := m.InfoRequests(id)
	if err != nil {
		return InfoResult{}, err
	}
	if index < 0 || index >= len(reqs) {
		return InfoResult{}, fmt.Errorf("no info request %d", index)
	}
	m.mu.Lock()
	e := m.devs[id]
	conn := e.conn
	if e.blu != nil {
		conn = e.blu.gw
	}
	stored := m.storedFor(e, reqs[index].Path)
	m.mu.Unlock()
	if conn == nil {
		if stored != nil {
			return InfoResult{Data: stored, Stored: true}, nil
		}
		return InfoResult{}, ErrNoConnection
	}
	b, err := conn.Get(ctx, reqs[index].Path)
	if err != nil {
		if stored != nil && shelly.IsOffline(err) {
			return InfoResult{Data: stored, Stored: true}, nil
		}
		return InfoResult{}, err
	}
	if !json.Valid(b) {
		b, _ = json.Marshal(string(b))
	}
	return InfoResult{Data: b}, nil
}

// storedFor returns the kept answer for the main requests. Callers hold m.mu.
func (m *Devices) storedFor(e *entry, path string) json.RawMessage {
	switch {
	case path == "/settings", path == "/rpc/Shelly.GetConfig":
		return e.rawConfig
	case path == "/status", path == "/rpc/Shelly.GetStatus":
		return e.rawStatus
	case path == "/shelly" || strings.HasPrefix(path, "/rpc/Shelly.GetDeviceInfo"):
		if e.info.MAC != "" {
			b, _ := json.Marshal(e.info)
			return b
		}
	}
	return nil
}

// ---- logs -----------------------------------------------------------------

// LogSnapshot reads a Gen1 debug log: file 0 is /debug/log, 1 is /debug/log1.
func (m *Devices) LogSnapshot(ctx context.Context, id string, file int) (string, error) {
	m.mu.Lock()
	e, ok := m.devs[id]
	m.mu.Unlock()
	if !ok {
		return "", ErrNotFound
	}
	if e.conn == nil || e.info.Gen != 0 {
		return "", errors.New("log snapshots exist for Gen1 devices only")
	}
	path := "/debug/log"
	if file == 1 {
		path = "/debug/log1"
	}
	b, err := e.conn.Get(ctx, path)
	return string(b), err
}

// LogStream connects to a Gen2+ device's /debug/log WebSocket (for a BLU
// device: its gateway's) and calls line for every message until ctx ends.
// Protected devices get the auth.* query parameters ShellyScanner computes
// (LoginManagerG2.getAuthString).
func (m *Devices) LogStream(ctx context.Context, id string, line func(json.RawMessage)) error {
	m.mu.Lock()
	e, ok := m.devs[id]
	if ok && e.blu != nil {
		e = m.devs[e.blu.gwID]
		ok = e != nil
	}
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	if e.conn == nil || e.info.Gen == 0 {
		return errors.New("live logs exist for Gen2+ devices only")
	}
	u := "ws://" + e.conn.Addr() + "/debug/log"
	if cred := m.credentialsFor(e.dev.ID); cred != nil && e.info.AuthG2 {
		q, err := wsLogAuth(ctx, m.client, e.conn.Addr(), cred.Password)
		if err != nil {
			return err
		}
		u += "?" + q
	}
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	c, _, err := websocket.Dial(dctx, u, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("connect to device log: %w", err)
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		line(b)
	}
}

// wsLogAuth asks the device for a challenge on /debug/log and builds
// "auth.username=admin&auth.realm=…&auth.response=…" with the JSON-RPC
// response formula (ha2 = sha256("dummy_method:dummy_uri")).
func wsLogAuth(ctx context.Context, c *shelly.Client, addr, password string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/debug/log", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", &shelly.OfflineError{Err: err}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ch := shelly.ParseChallengeParams(resp.Header.Get("WWW-Authenticate"))
	if ch["nonce"] == "" {
		return "", shelly.ErrUnauthorized
	}
	nc := ch["nc"]
	if nc == "" {
		nc = "1"
	}
	cnonce := fmt.Sprintf("ShLM%d", time.Now().UnixNano()%1_000_000_000)
	alg := ch["algorithm"]
	if alg == "" {
		alg = "SHA-256"
	}
	v := url.Values{}
	v.Set("auth.username", shelly.DigestUser)
	v.Set("auth.realm", ch["realm"])
	v.Set("auth.nonce", ch["nonce"])
	v.Set("auth.cnonce", cnonce)
	v.Set("auth.algorithm", alg)
	v.Set("auth.response", shelly.RPCAuthResponse(ch["realm"], password, ch["nonce"], nc, cnonce))
	v.Set("auth.nc", nc)
	return v.Encode(), nil
}

// SetPaused pauses or resumes the refresh of one device ("Pause refresh" in
// the logs dialog, so ShellyScanner's own polling does not fill the log).
func (m *Devices) SetPaused(id string, paused bool) error {
	m.mu.Lock()
	e, ok := m.devs[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	e.paused = paused
	e.dev.Paused = paused
	d := e.dev
	m.mu.Unlock()
	m.emit(EventDeviceUpsert, d)
	return nil
}
