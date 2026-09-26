package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/store"
)

func newDeviceServer(t *testing.T) (*httptest.Server, *service.Devices) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Update(func(s *store.Settings) { s.Scan.Mode = store.ScanOffline })
	st.SaveArchive([]store.ArchivedDevice{{TypeID: "SHPLG-S", TypeName: "PlugS", MAC: "AABBCC000001", IP: "192.0.2.1", Port: 80, Gen: "1"}})
	h := hub.New(nil, nil, nil)
	devs := service.NewDevices(st, shelly.NewClient(), func(typ string, data any) { h.Broadcast(hub.Event{Type: typ, Data: data}) }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); devs.Wait() })
	devs.Start(ctx)
	srv := httptest.NewServer(New(Config{Store: st, Hub: h, Devices: devs, Static: fstest.MapFS{"index.html": {Data: []byte("x")}}}))
	t.Cleanup(srv.Close)
	return srv, devs
}

func TestDeviceEndpoints(t *testing.T) {
	srv, _ := newDeviceServer(t)

	var list []map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/devices", "", nil), &list)
	if len(list) != 1 || list[0]["status"] != "ghost" || list[0]["typeName"] != "PlugS" {
		t.Fatalf("devices = %v", list)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/devices/AABBCC000001", "", nil); r.StatusCode != 200 {
		t.Fatalf("get device: %d", r.StatusCode)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/devices/NOPE", "", nil); r.StatusCode != 404 {
		t.Fatalf("unknown device: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/NOPE/reload", "", nil); r.StatusCode != 404 {
		t.Fatalf("reload unknown: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/refresh", "", nil); r.StatusCode != 202 {
		t.Fatalf("refresh all: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/refresh", `{"ids":["AABBCC000001"]}`, jsonHdr); r.StatusCode != 202 {
		t.Fatalf("refresh one: %d", r.StatusCode)
	}
	var scan map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/scan", "", nil), &scan)
	if scan["mode"] != "offline" {
		t.Fatalf("scan = %v", scan)
	}
	// A ghost can be removed; afterwards it is gone.
	if r := do(t, "DELETE", srv.URL+"/api/v1/devices/AABBCC000001", "", nil); r.StatusCode != 204 {
		t.Fatalf("remove ghost: %d", r.StatusCode)
	}
	if r := do(t, "DELETE", srv.URL+"/api/v1/devices/AABBCC000001", "", nil); r.StatusCode != 404 {
		t.Fatalf("remove twice: %d", r.StatusCode)
	}
}

func TestCredentialsAreWriteOnly(t *testing.T) {
	srv, _ := newDeviceServer(t)
	r := do(t, "PUT", srv.URL+"/api/v1/credentials", `{"user":"admin","password":"top-secret-pw"}`, jsonHdr)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || strings.Contains(string(b), "top-secret-pw") {
		t.Fatalf("PUT credentials: %d %s", r.StatusCode, b)
	}
	r = do(t, "GET", srv.URL+"/api/v1/credentials", "", nil)
	b, _ = io.ReadAll(r.Body)
	if !strings.Contains(string(b), `"globalSet":true`) || strings.Contains(string(b), "top-secret-pw") {
		t.Fatalf("GET credentials leaks or misses: %s", b)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/devices/NOPE/credentials", `{"password":"x"}`, jsonHdr); r.StatusCode != 404 {
		t.Fatalf("credentials for unknown device: %d", r.StatusCode)
	}
}

func TestScanSettingsTriggerRescan(t *testing.T) {
	srv, devs := newDeviceServer(t)
	before := devs.ScanState().StartedAt
	time.Sleep(5 * time.Millisecond)
	body := `{"scan":{"mode":"ip","ranges":[{"base":"192.0.2","first":1,"last":1}],"refreshSeconds":2,"configTics":5}}`
	if r := do(t, "PUT", srv.URL+"/api/v1/settings", body, jsonHdr); r.StatusCode != 200 {
		t.Fatalf("PUT scan settings: %d", r.StatusCode)
	}
	deadline := time.Now().Add(5 * time.Second)
	for devs.ScanState().StartedAt == before || devs.ScanState().Mode != "ip" {
		if time.Now().After(deadline) {
			t.Fatalf("no rescan after scan settings change: %+v", devs.ScanState())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/settings", `{"scan":{"mode":"ip","refreshSeconds":2,"configTics":5}}`, jsonHdr); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("IP scan without ranges accepted: %d", r.StatusCode)
	}
}

func TestCommandAndRebootEndpoints(t *testing.T) {
	srv, _ := newDeviceServer(t)
	// The archived device is a ghost: commands have no connection, reboot is refused.
	if r := do(t, "POST", srv.URL+"/api/v1/devices/AABBCC000001/command", `{"key":"relay/0","action":"toggle"}`, jsonHdr); r.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("command on stored device: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/NOPE/command", `{"key":"relay/0","action":"toggle"}`, jsonHdr); r.StatusCode != 404 {
		t.Fatalf("command on unknown device: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/reboot", `{"ids":["AABBCC000001"]}`, jsonHdr); r.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("reboot without confirm: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/reboot", `{"ids":["AABBCC000001"],"confirm":true}`, jsonHdr); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("reboot of a stored device: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/reboot", `{"ids":[],"confirm":true}`, jsonHdr); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("reboot without devices: %d", r.StatusCode)
	}
}
