package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/discovery"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/sim"
	"github.com/wimmme/shellylanman/internal/store"
)

// ---- a fake service --------------------------------------------------------------

type fake struct {
	devs     []model.Device
	commands []string
	rebooted []string
	rpc      []string
	samples  []service.Sample
}

func (f *fake) List() []model.Device { return f.devs }
func (f *fake) Command(_ context.Context, id string, c service.Command) error {
	f.commands = append(f.commands, id+" "+c.Key+" "+c.Action)
	return nil
}
func (f *fake) Reboot(ids []string) error { f.rebooted = append(f.rebooted, ids...); return nil }
func (f *fake) Firmware(context.Context, []string) ([]service.FirmwareRow, error) {
	return []service.FirmwareRow{{ID: "A1", Current: "1.0"}}, nil
}
func (f *fake) FirmwareUpdate(_ context.Context, r []service.FirmwareRequest) ([]service.ResultLine, error) {
	var out []service.ResultLine
	for _, x := range r {
		out = append(out, service.ResultLine{DeviceRef: service.DeviceRef{ID: x.ID}, Result: "ok"})
	}
	return out, nil
}
func (f *fake) Backup(_ context.Context, ids []string) ([]service.ResultLine, error) {
	return []service.ResultLine{{DeviceRef: service.DeviceRef{ID: ids[0]}, Result: "ok"}}, nil
}
func (f *fake) Backups(string) []service.BackupFile { return nil }
func (f *fake) Samples([]string, int64) map[string][]service.Sample {
	return map[string][]service.Sample{"A1": f.samples}
}
func (f *fake) DeviceRPC(_ context.Context, id, method string, _ json.RawMessage) (json.RawMessage, error) {
	f.rpc = append(f.rpc, id+" "+method)
	return json.RawMessage(`{"ok":true}`), nil
}
func (f *fake) Checklist(context.Context, []string) []service.ChecklistRow { return nil }

func tru() *bool { b := true; return &b }

func newFake() *fake {
	return &fake{devs: []model.Device{
		{ID: "A1", Name: "Kitchen", Hostname: "shellyplus1-a1", IP: "10.0.0.1", Gen: "2", Status: model.StatusOnline,
			Modules: []parse.Module{{Kind: parse.KindRelay, Index: 0, Key: "switch:0", On: tru()}}},
		{ID: "B2", Name: "Kitchen LED", Hostname: "shellyrgbw2-b2", IP: "10.0.0.2", Gen: "1", Status: model.StatusOnline,
			Modules: []parse.Module{{Kind: parse.KindRGBW, Index: 0, Key: "color/0"}}},
		{ID: "C3", Name: "Garage", IP: "10.0.0.3", Gen: "1", Status: model.StatusOffline},
	}}
}

type client struct {
	t     *testing.T
	url   string
	token string
	n     int
}

func (c *client) post(body string) (*http.Response, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// call sends a request and returns result (or fails on a JSON-RPC error).
func (c *client) call(method string, params any) map[string]any {
	c.t.Helper()
	c.n++
	p, _ := json.Marshal(params)
	_, b := c.post(`{"jsonrpc":"2.0","id":` + strconv.Itoa(c.n) + `,"method":"` + method + `","params":` + string(p) + `}`)
	var r struct {
		Result map[string]any `json:"result"`
		Error  *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		c.t.Fatalf("%s: %v: %s", method, err, b)
	}
	if r.Error != nil {
		c.t.Fatalf("%s: rpc error %+v", method, r.Error)
	}
	return r.Result
}

// tool calls a tool; it returns the text and whether it was an error.
func (c *client) tool(name string, args any) (string, bool) {
	c.t.Helper()
	res := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	content := res["content"].([]any)[0].(map[string]any)
	isErr, _ := res["isError"].(bool)
	return content["text"].(string), isErr
}

func serve(t *testing.T, svc Service, cfg *Config) *client {
	t.Helper()
	srv := httptest.NewServer(&Server{Service: svc, Config: func() Config { return *cfg }, Version: "test", Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(srv.Close)
	return &client{t: t, url: srv.URL, token: cfg.Token}
}

// ---- protocol ----------------------------------------------------------------------

func TestHandshakeAndAuth(t *testing.T) {
	cfg := &Config{Enabled: true, Access: AccessRead, Token: "secret"}
	c := serve(t, newFake(), cfg)

	hello := c.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t"}})
	if hello["protocolVersion"] != "2025-03-26" || hello["serverInfo"].(map[string]any)["name"] != "shellylanman" {
		t.Fatalf("initialize: %v", hello)
	}
	if v := c.call("initialize", map[string]any{"protocolVersion": "1999-01-01"})["protocolVersion"]; v != versions[0] {
		t.Fatalf("unknown version answered with %v", v)
	}
	if r, _ := c.post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); r.StatusCode != http.StatusAccepted {
		t.Fatalf("notification: %d", r.StatusCode)
	}
	c.call("ping", map[string]any{})
	if _, b := c.post(`{"jsonrpc":"2.0","id":9,"method":"nope"}`); !strings.Contains(string(b), "-32601") {
		t.Fatalf("unknown method: %s", b)
	}
	if _, b := c.post(`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`); !strings.Contains(string(b), "-32600") {
		t.Fatalf("batch: %s", b)
	}

	bad := *c
	bad.token = "wrong"
	if r, _ := bad.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); r.StatusCode != http.StatusUnauthorized || r.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	bad.token = ""
	if r, _ := bad.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d", r.StatusCode)
	}
	cfg.Token = "" // no token configured: nobody gets in, not even with an empty header
	if r, _ := bad.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token configured: %d", r.StatusCode)
	}
	cfg.Token, cfg.Enabled = "secret", false
	if r, _ := c.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); r.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled: %d", r.StatusCode)
	}
	cfg.Enabled = true
	req, _ := http.NewRequest("GET", c.url, nil)
	req.Header.Set("Authorization", "Bearer secret")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", r.StatusCode)
	}
}

func toolNames(res map[string]any) []string {
	var names []string
	for _, x := range res["tools"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	return names
}

func TestAccessLevels(t *testing.T) {
	cfg := &Config{Enabled: true, Access: AccessRead, Token: "t"}
	f := newFake()
	c := serve(t, f, cfg)

	names := toolNames(c.call("tools/list", map[string]any{}))
	if !slices.Contains(names, "shelly_list_devices") || slices.Contains(names, "shelly_switch") || slices.Contains(names, "shelly_reboot") {
		t.Fatalf("read-only tools: %v", names)
	}
	if _, b := c.post(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shelly_switch","arguments":{"device":"A1","action":"on"}}}`); !strings.Contains(string(b), "unknown tool") {
		t.Fatalf("control tool callable read-only: %s", b)
	}
	if len(f.commands) > 0 {
		t.Fatal("command sent in read-only mode")
	}

	cfg.Access = AccessControl
	res := c.call("tools/list", map[string]any{})
	names = toolNames(res)
	if !slices.Contains(names, "shelly_switch") || !slices.Contains(names, "shelly_firmware_update") {
		t.Fatalf("control tools: %v", names)
	}
	for _, x := range res["tools"].([]any) {
		tl := x.(map[string]any)
		ann := tl["annotations"].(map[string]any)
		if tl["name"] == "shelly_reboot" && ann["destructiveHint"] != true {
			t.Fatal("reboot not marked destructive")
		}
		if tl["name"] == "shelly_list_devices" && ann["readOnlyHint"] != true {
			t.Fatal("list not marked read-only")
		}
	}
}

// ---- tools -------------------------------------------------------------------------

func TestReadTools(t *testing.T) {
	f := newFake()
	c := serve(t, f, &Config{Enabled: true, Access: AccessRead, Token: "t"})

	txt, isErr := c.tool("shelly_list_devices", map[string]any{"status": "online"})
	if isErr || !strings.Contains(txt, `"count": 2`) || strings.Contains(txt, "Garage") {
		t.Fatalf("list online: %s", txt)
	}
	if txt, _ = c.tool("shelly_list_devices", map[string]any{"query": "garage"}); !strings.Contains(txt, `"count": 1`) {
		t.Fatalf("query: %s", txt)
	}
	// By IP, MAC in any form, host name, exact name, and unique partial name.
	for _, ref := range []string{"10.0.0.1", "a1", "shellyplus1-a1", "kitchen"} {
		if txt, isErr = c.tool("shelly_get_device", map[string]any{"device": ref}); isErr || !strings.Contains(txt, `"id": "A1"`) {
			t.Fatalf("resolve %q: %s", ref, txt)
		}
	}
	if txt, isErr = c.tool("shelly_get_device", map[string]any{"device": "kitch"}); !isErr || !strings.Contains(txt, "several") {
		t.Fatalf("ambiguous: %s", txt)
	}
	if txt, isErr = c.tool("shelly_get_device", map[string]any{"device": "attic"}); !isErr || !strings.Contains(txt, "no device") {
		t.Fatalf("unknown: %s", txt)
	}
	if txt, isErr = c.tool("shelly_get_device", map[string]any{"device": "A1", "extra": 1}); !isErr {
		t.Fatalf("unknown argument accepted: %s", txt)
	}

	if _, isErr = c.tool("shelly_rpc_read", map[string]any{"device": "A1", "method": "Switch.GetStatus", "params": map[string]any{"id": 0}}); isErr || len(f.rpc) != 1 {
		t.Fatalf("rpc read: %v", f.rpc)
	}
	for _, m := range []string{"Switch.Set", "Shelly.Reboot", "Shelly.FactoryReset", "Script.Eval", "KVS.Set", "Sys.SetConfig"} {
		if txt, isErr = c.tool("shelly_rpc_read", map[string]any{"device": "A1", "method": m}); !isErr {
			t.Fatalf("%s allowed: %s", m, txt)
		}
	}
	if len(f.rpc) != 1 {
		t.Fatalf("write method reached the device: %v", f.rpc)
	}

	// Readings: at most ~120 points, ending with the newest.
	now := time.Now().UnixMilli()
	for i := range 500 {
		f.samples = append(f.samples, service.Sample{T: now - int64(500-i)*1000, RSSI: -i})
	}
	txt, _ = c.tool("shelly_get_readings", map[string]any{"device": "A1", "minutes": 30})
	var r struct {
		Points []struct {
			RSSI int `json:"rssi_dbm"`
		}
	}
	json.Unmarshal([]byte(txt), &r)
	if len(r.Points) > 121 || len(r.Points) < 100 || r.Points[len(r.Points)-1].RSSI != -499 {
		t.Fatalf("readings: %d points, last %+v", len(r.Points), r.Points[len(r.Points)-1])
	}
}

func TestControlTools(t *testing.T) {
	f := newFake()
	c := serve(t, f, &Config{Enabled: true, Access: AccessControl, Token: "t"})

	if _, isErr := c.tool("shelly_switch", map[string]any{"device": "Kitchen", "action": "off"}); isErr || f.commands[0] != "A1 switch:0 off" {
		t.Fatalf("switch: %v", f.commands)
	}
	if txt, isErr := c.tool("shelly_switch", map[string]any{"device": "Kitchen", "action": "on", "channel": 3}); !isErr || !strings.Contains(txt, "no channel 3") {
		t.Fatalf("missing channel: %s", txt)
	}
	if txt, isErr := c.tool("shelly_switch", map[string]any{"device": "Kitchen LED", "action": "on"}); !isErr || !strings.Contains(txt, "no relay") {
		t.Fatalf("switch on a light: %s", txt)
	}
	f.commands = nil
	if _, isErr := c.tool("shelly_light", map[string]any{"device": "B2", "rgb": []int{255, 0, 0}, "brightness": 40, "on": true}); isErr {
		t.Fatal("light")
	}
	if !slices.Equal(f.commands, []string{"B2 color/0 brightness", "B2 color/0 color", "B2 color/0 on"}) {
		t.Fatalf("light commands: %v", f.commands)
	}

	// Destructive: nothing happens without confirm.
	if txt, isErr := c.tool("shelly_reboot", map[string]any{"devices": []string{"A1"}, "confirm": false}); !isErr || len(f.rebooted) > 0 {
		t.Fatalf("reboot without confirm: %s %v", txt, f.rebooted)
	}
	if _, isErr := c.tool("shelly_reboot", map[string]any{"devices": []string{"A1", "10.0.0.1"}, "confirm": true}); isErr || !slices.Equal(f.rebooted, []string{"A1"}) {
		t.Fatalf("reboot: %v", f.rebooted)
	}
	if txt, isErr := c.tool("shelly_firmware_update", map[string]any{"devices": []string{"A1"}, "stage": "any", "confirm": true}); !isErr {
		t.Fatalf("stage any accepted: %s", txt)
	}
	if txt, isErr := c.tool("shelly_firmware_update", map[string]any{"devices": []string{"A1"}, "stage": "stable", "confirm": true}); isErr || !strings.Contains(txt, `"result": "ok"`) {
		t.Fatalf("firmware: %s", txt)
	}
}

// ---- end to end: the real device service and a simulated Plug S -------------------------

func TestEndToEndWithSimulator(t *testing.T) {
	d, err := sim.New("../../testdata/gen1/SHPLG-S")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	simSrv := &http.Server{Handler: d}
	go simSrv.Serve(ln)
	t.Cleanup(func() { simSrv.Close() })

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *store.Settings) {
		s.Scan.Mode = store.ScanIP
		s.Scan.Ranges = []discovery.Range{{Base: "127.0.0", First: 1, Last: 1}}
	}); err != nil {
		t.Fatal(err)
	}
	devs := service.NewDevices(st, shelly.NewClient(), func(string, any) {}, slog.New(slog.DiscardHandler))
	devs.IPScanPort = ln.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); devs.Wait() })
	devs.Start(ctx)

	deadline := time.Now().Add(10 * time.Second)
	for {
		if l := devs.List(); len(l) == 1 && l[0].Status == model.StatusOnline {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("simulated device not found: %+v", devs.List())
		}
		time.Sleep(50 * time.Millisecond)
	}

	c := serve(t, devs, &Config{Enabled: true, Access: AccessControl, Token: "t"})
	c.call("initialize", map[string]any{"protocolVersion": versions[0]})
	txt, isErr := c.tool("shelly_list_devices", map[string]any{})
	if isErr || !strings.Contains(txt, "PlugS") {
		t.Fatalf("list: %s", txt)
	}
	if txt, isErr = c.tool("shelly_switch", map[string]any{"device": "127.0.0.1", "action": "off"}); isErr {
		t.Fatalf("switch off: %s", txt)
	}
	if !slices.Contains(d.Calls(), "GET /relay/0?turn=off") || !strings.Contains(txt, `"on": false`) {
		t.Fatalf("relay not switched: %v %s", d.Calls(), txt)
	}
	if _, isErr = c.tool("shelly_rpc_read", map[string]any{"device": "127.0.0.1", "method": "Shelly.GetStatus"}); !isErr {
		t.Fatal("RPC on a Gen1 device should be refused")
	}
}
