package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/sim"
)

func startSimDev(t *testing.T, dir string, configure func(*sim.Device)) (*sim.Device, string) {
	t.Helper()
	d, err := sim.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if configure != nil {
		configure(d)
	}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	return d, strings.TrimPrefix(srv.URL, "http://")
}

func hasCall(d *sim.Device, want string) bool {
	for _, c := range d.Calls() {
		if c == want {
			return true
		}
	}
	return false
}

func relayOn(d model.Device) bool {
	return len(d.Modules) > 0 && d.Modules[0].On != nil && *d.Modules[0].On
}

func fv(f float64) *float64 { return &f }

func TestGen1RelayCommands(t *testing.T) {
	m, _, ctx := newService(t, nil)
	simd, addr := startSimDev(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	m.handle(ctx, addr, "shellyplug-s-aabbcc000001", true)
	d := waitDevice(t, m, "AABBCC000001", online)
	was := relayOn(d)

	if err := m.Command(ctx, d.ID, Command{Key: "relay/0", Action: ActionToggle}); err != nil {
		t.Fatal(err)
	}
	if !hasCall(simd, "GET /relay/0?turn=toggle") {
		t.Fatalf("calls %v", simd.Calls())
	}
	// The status is re-read at once, so the new state is there without waiting for the poll.
	if got, _ := m.Get(d.ID); relayOn(got) == was {
		t.Fatalf("relay state not refreshed after toggle: %+v", got.Modules)
	}
	if err := m.Command(ctx, d.ID, Command{Key: "relay/0", Action: ActionOff}); err != nil || !hasCall(simd, "GET /relay/0?turn=off") {
		t.Fatalf("off: %v %v", err, simd.Calls())
	}
	if got, _ := m.Get(d.ID); relayOn(got) {
		t.Fatal("relay still on after off")
	}
	if err := m.Command(ctx, d.ID, Command{Key: "relay/9", Action: ActionOn}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("unknown module: %v", err)
	}
	if err := m.Command(ctx, d.ID, Command{Key: "relay/0", Action: ActionBrightness, Value: fv(10)}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("unsupported action: %v", err)
	}
}

func TestGen2CommandsWithAuthentication(t *testing.T) {
	m, _, ctx := newService(t, nil)
	simd, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json": `{"id":"shellyplus1-aabbcc000001","mac":"AABBCC000001","model":"SNSW-001X16EU","gen":2,"app":"Plus1","auth_en":true}`,
	}), func(d *sim.Device) { d.Password = "secret" })
	if err := m.SetGlobalCredentials(shelly.Credentials{User: "admin", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	m.handle(ctx, addr, "shellyplus1-aabbcc000001", true)
	d := waitDevice(t, m, "AABBCC000001", func(d model.Device) bool { return online(d) && len(d.Modules) == 1 })
	was := relayOn(d)
	if err := m.Command(ctx, d.ID, Command{Key: "switch:0", Action: ActionToggle}); err != nil {
		t.Fatal(err)
	}
	if !hasCall(simd, "GET /rpc/Switch.Toggle?id=0") {
		t.Fatalf("calls %v", simd.Calls())
	}
	if got, _ := m.Get(d.ID); relayOn(got) == was {
		t.Fatal("state not refreshed")
	}
	if err := m.Command(ctx, d.ID, Command{Key: "switch:0", Action: ActionOn}); err != nil || !hasCall(simd, "GET /rpc/Switch.Set?id=0&on=true") {
		t.Fatalf("on: %v %v", err, simd.Calls())
	}
}

func TestGen2PostCommands(t *testing.T) {
	m, _, ctx := newService(t, nil)
	simd, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json":               `{"id":"shellywalldisplay-aabbcc000002","mac":"AABBCC000002","model":"SAWD-0A1XX10EU1","gen":2,"app":"WallDisplay","auth_en":true}`,
		"rpc_Shelly.GetConfig.json": `{"sys":{"device":{"name":"Hall"}},"thermostat:0":{"name":"Living","type":"heating"}}`,
		"rpc_Shelly.GetStatus.json": `{"sys":{"uptime":10},"thermostat:0":{"enable":true,"target_C":21,"output":false},"temperature:0":{"tC":20}}`,
	}), func(d *sim.Device) { d.Password = "pw" })
	m.SetGlobalCredentials(shelly.Credentials{User: "admin", Password: "pw"})
	m.handle(ctx, addr, "shellywalldisplay-aabbcc000002", true)
	d := waitDevice(t, m, "AABBCC000002", func(d model.Device) bool { return online(d) && len(d.Modules) == 1 })
	if d.Layout != "thermostat" {
		t.Fatalf("layout %q", d.Layout)
	}
	// POST /rpc with the JSON-RPC auth object (the device is protected).
	if err := m.Command(ctx, d.ID, Command{Key: "thermostat:0", Action: ActionTarget, Value: fv(22.5)}); err != nil {
		t.Fatal(err)
	}
	if !hasCall(simd, `RPC Thermostat.SetConfig {"config":{"target_C":22.5},"id":0}`) {
		t.Fatalf("calls %v", simd.Calls())
	}
	if err := m.Command(ctx, d.ID, Command{Key: "thermostat:0", Action: ActionEnable, Value: fv(0)}); err != nil ||
		!hasCall(simd, `RPC Thermostat.SetConfig {"config":{"enable":false},"id":0}`) {
		t.Fatalf("enable: %v %v", err, simd.Calls())
	}
	if err := m.Command(ctx, d.ID, Command{Key: "thermostat:0", Action: ActionTarget, Value: fv(40)}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("target out of range: %v", err)
	}
}

// Firmware 2.0+ answers an unauthenticated POST /rpc with an empty 401 and the
// challenge only in WWW-Authenticate: the call is repeated with a Digest header.
func TestGen2PostCommandsFirmware2Auth(t *testing.T) {
	m, _, ctx := newService(t, nil)
	simd, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json":               `{"id":"shellywalldisplay-aabbcc000002","mac":"AABBCC000002","model":"SAWD-0A1XX10EU1","gen":2,"app":"WallDisplay","auth_en":true}`,
		"rpc_Shelly.GetConfig.json": `{"sys":{"device":{"name":"Hall"}},"thermostat:0":{"name":"Living","type":"heating"}}`,
		"rpc_Shelly.GetStatus.json": `{"sys":{"uptime":10},"thermostat:0":{"enable":true,"target_C":21,"output":false},"temperature:0":{"tC":20}}`,
	}), func(d *sim.Device) { d.Password = "pw"; d.FW2Auth = true })
	m.SetGlobalCredentials(shelly.Credentials{User: "admin", Password: "pw"})
	m.handle(ctx, addr, "shellywalldisplay-aabbcc000002", true)
	d := waitDevice(t, m, "AABBCC000002", func(d model.Device) bool { return online(d) && len(d.Modules) == 1 })
	for _, v := range []float64{22.5, 23} { // the second call reuses the nonce
		if err := m.Command(ctx, d.ID, Command{Key: "thermostat:0", Action: ActionTarget, Value: fv(v)}); err != nil {
			t.Fatalf("target %v: %v (calls %v)", v, err, simd.Calls())
		}
	}
	if !hasCall(simd, `RPC Thermostat.SetConfig {"config":{"target_C":23},"id":0}`) {
		t.Fatalf("calls %v", simd.Calls())
	}
	if _, err := m.DeviceRPC(ctx, d.ID, "Shelly.GetConfig", nil); err != nil {
		t.Fatalf("rpc: %v", err)
	}
}

func TestBreakerNeedsConfirmation(t *testing.T) {
	m, _, ctx := newService(t, nil)
	simd, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json":               `{"id":"shellypro2cb-aabbcc000003","mac":"AABBCC000003","model":"SPCB-02","gen":2,"app":"ProCB","auth_en":false}`,
		"rpc_Shelly.GetConfig.json": `{"cb:0":{"name":"Main"}}`,
		"rpc_Shelly.GetStatus.json": `{"cb:0":{"output":true,"safety":false}}`,
	}), nil)
	m.handle(ctx, addr, "shellypro2cb-aabbcc000003", true)
	d := waitDevice(t, m, "AABBCC000003", func(d model.Device) bool { return online(d) && len(d.Modules) == 1 })
	if err := m.Command(ctx, d.ID, Command{Key: "cb:0", Action: ActionToggle}); !errors.Is(err, ErrConfirm) {
		t.Fatalf("without confirm: %v", err)
	}
	if err := m.Command(ctx, d.ID, Command{Key: "cb:0", Action: ActionToggle, Confirm: true}); err != nil ||
		!hasCall(simd, `RPC CB.Set {"id":0,"output":false}`) {
		t.Fatalf("with confirm: %v %v", err, simd.Calls())
	}
}

func TestInputEventsRunTheActionURLs(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.RequestURI())
		mu.Unlock()
	}))
	defer target.Close()

	m, _, ctx := newService(t, nil)
	_, addr := startSimDev(t, fixtureDir(t, "gen1/SHIX3-1", map[string]string{
		"settings_actions.json": `{"actions":{"shortpush_url":[{"index":0,"enabled":true,"urls":["` + target.URL + `/a","` + target.URL + `/b"]}],"longpush_url":[{"index":0,"enabled":false,"urls":["` + target.URL + `/c"]}]}}`,
	}), nil)
	m.handle(ctx, addr, "shellyix3-aabbcc000004", true)
	var id string
	for id == "" {
		for _, d := range m.List() {
			if d.TypeID == "SHIX3-1" && online(d) && len(d.Modules) == 3 && len(d.Modules[0].Events) == 2 {
				id = d.ID
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := m.Command(ctx, id, Command{Key: "input/0", Action: ActionEvent, Event: 0}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := strings.Join(hits, ",")
	mu.Unlock()
	if got != "/a,/b" {
		t.Fatalf("action URLs hit: %q", got)
	}
	if err := m.Command(ctx, id, Command{Key: "input/0", Action: ActionEvent, Event: 1}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("disabled action: %v", err)
	}

	// Gen2+: webhook URLs on 127.0.0.1 go to the device itself.
	simd, addr2 := startSimDev(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json":               `{"id":"shellyplusi4-aabbcc000005","mac":"AABBCC000005","model":"SNSN-0024X","gen":2,"app":"PlusI4","auth_en":false}`,
		"rpc_Shelly.GetConfig.json": `{"input:0":{"name":"Hall","enable":true},"input:1":{},"input:2":{},"input:3":{}}`,
		"rpc_Shelly.GetStatus.json": `{"input:0":{"state":false}}`,
		"rpc_Webhook.List.json":     `{"hooks":[{"id":1,"cid":0,"event":"input.button_push","enable":true,"urls":["http://127.0.0.1/rpc/Switch.Toggle?id=0"]}]}`,
	}), nil)
	m.handle(ctx, addr2, "shellyplusi4-aabbcc000005", true)
	waitDevice(t, m, "AABBCC000005", func(d model.Device) bool { return online(d) && len(d.Modules) == 4 && len(d.Modules[0].Events) == 1 })
	if err := m.Command(ctx, "AABBCC000005", Command{Key: "input:0", Action: ActionEvent, Event: 0}); err != nil {
		t.Fatal(err)
	}
	if !hasCall(simd, "GET /rpc/Switch.Toggle?id=0") {
		t.Fatalf("webhook not sent to the device: %v", simd.Calls())
	}
}

func TestReboot(t *testing.T) {
	RebootPause = 50 * time.Millisecond
	m, _, ctx := newService(t, nil)
	simd, addr := startSimDev(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	m.handle(ctx, addr, "shellyplug-s-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", online)
	if err := m.Reboot([]string{"AABBCC000001"}); err != nil {
		t.Fatal(err)
	}
	if d, _ := m.Get("AABBCC000001"); d.Status != model.StatusReading {
		t.Fatalf("status during reboot %s", d.Status)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !hasCall(simd, "GET /reboot") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !hasCall(simd, "GET /reboot") {
		t.Fatalf("calls %v", simd.Calls())
	}
	waitDevice(t, m, "AABBCC000001", online) // refresh resumes after the pause
	if err := m.Reboot([]string{"nope"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestCommandOnStoredDevice(t *testing.T) {
	m, _, _ := newService(t, nil)
	m.mu.Lock()
	m.devs["X"] = &entry{dev: model.Device{ID: "X", Status: model.StatusGhost}}
	m.mu.Unlock()
	if err := m.Command(context.Background(), "X", Command{Key: "relay/0", Action: ActionOn}); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("ghost: %v", err)
	}
	if err := m.Reboot([]string{"X"}); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("reboot ghost: %v", err)
	}
}
