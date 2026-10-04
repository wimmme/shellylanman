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
	srv, devs, _ := newDeviceServerStore(t)
	return srv, devs
}

func newDeviceServerStore(t *testing.T) (*httptest.Server, *service.Devices, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Update(func(s *store.Settings) { s.Scan.Mode = store.ScanOffline })
	st.SaveArchive([]store.ArchivedDevice{{TypeID: "SHPLG-S", TypeName: "PlugS", Host: "shellyplug-s-AABBCC000001", MAC: "AABBCC000001", IP: "192.0.2.1", Port: 80, Gen: "1"}})
	h := hub.New(nil, nil, nil)
	devs := service.NewDevices(st, shelly.NewClient(), func(typ string, data any) { h.Broadcast(hub.Event{Type: typ, Data: data}) }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); devs.Wait() })
	devs.Start(ctx)
	srv := httptest.NewServer(New(Config{Store: st, Hub: h, Devices: devs, Static: fstest.MapFS{"index.html": {Data: []byte("x")}}}))
	t.Cleanup(srv.Close)
	return srv, devs, st
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

func TestConfigEndpoints(t *testing.T) {
	srv, _ := newDeviceServer(t)
	if r := do(t, "GET", srv.URL+"/api/v1/config/wifi1", "", nil); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("no ids: %d", r.StatusCode)
	}
	// The only device is archived: Wi-Fi cannot be read, everything is excluded.
	if r := do(t, "GET", srv.URL+"/api/v1/config/wifi1?ids=AABBCC000001", "", nil); r.StatusCode != http.StatusConflict {
		t.Fatalf("all excluded: %d", r.StatusCode)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/config/bogus?ids=AABBCC000001", "", nil); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown section: %d", r.StatusCode)
	}
	// NTP for an archived device is queued as a deferred task.
	r := do(t, "POST", srv.URL+"/api/v1/config/others", `{"ids":["AABBCC000001"],"part":"ntp","ntp":"pool.ntp.org"}`, jsonHdr)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `"result":"queued"`) {
		t.Fatalf("queue: %d %s", r.StatusCode, b)
	}
	var list []map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/deferred", "", nil), &list)
	if len(list) != 1 || list[0]["status"] != "WAITING" || list[0]["sealed"] != "" && list[0]["sealed"] != nil {
		t.Fatalf("deferred %v", list)
	}
	id, _ := list[0]["id"].(string)
	if r := do(t, "DELETE", srv.URL+"/api/v1/deferred/"+id, "", nil); r.StatusCode != 204 {
		t.Fatalf("cancel: %d", r.StatusCode)
	}
	if r := do(t, "DELETE", srv.URL+"/api/v1/deferred/"+id, "", nil); r.StatusCode != http.StatusConflict {
		t.Fatalf("cancel twice: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/config/wifi1", `{"ids":["AABBCC000001"],"enabled":true,"ssid":"x","password":"y","mode":"dhcp"}`, jsonHdr); r.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("wifi without confirm: %d", r.StatusCode)
	}
	var rows []map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/checklist", "", nil), &rows)
	if len(rows) != 1 || rows[0]["id"] != "AABBCC000001" {
		t.Fatalf("checklist %v", rows)
	}
}

func TestDeviceCredentialsOnlyOnTrustedPaths(t *testing.T) {
	srv, devs, st := newDeviceServerStore(t)
	const url = "/api/v1/devices/AABBCC000001/credentials"
	if r := do(t, "GET", srv.URL+url, "", nil); r.StatusCode != 403 {
		t.Fatalf("no token, nothing stored: %d", r.StatusCode)
	}
	if err := devs.SetGlobalCredentials(shelly.Credentials{User: "admin", Password: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	// The open LAN port: never without the MCP token at access level configure.
	if r := do(t, "GET", srv.URL+url, "", nil); r.StatusCode != 403 {
		t.Fatalf("no token: %d", r.StatusCode)
	}
	st.SetSecret(store.MCPTokenSecret, "tok")
	bearer := map[string]string{"Authorization": "Bearer tok"}
	st.Update(func(s *store.Settings) { s.MCP.Enabled, s.MCP.Access = true, "control" })
	if r := do(t, "GET", srv.URL+url, "", bearer); r.StatusCode != 403 {
		t.Fatalf("token, access control: %d", r.StatusCode)
	}
	st.Update(func(s *store.Settings) { s.MCP.Access = "configure" })
	if r := do(t, "GET", srv.URL+url, "", map[string]string{"Authorization": "Bearer wrong"}); r.StatusCode != 403 {
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	resp := do(t, "GET", srv.URL+url, "", bearer)
	var c shelly.Credentials
	decode(t, resp, &c)
	if c.User != "admin" || c.Password != "s3cret" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("with token: %+v %q", c, resp.Header.Get("Cache-Control"))
	}
	if r := do(t, "GET", srv.URL+"/api/v1/devices/NOPE/credentials", "", bearer); r.StatusCode != 404 {
		t.Fatalf("unknown device: %d", r.StatusCode)
	}

	// The Home Assistant app's loopback listener: without token (MCP may be off).
	st.Update(func(s *store.Settings) { s.MCP.Enabled = false })
	local := httptest.NewServer(MCPLocal(Config{Store: st, Devices: devs}))
	t.Cleanup(local.Close)
	c = shelly.Credentials{}
	decode(t, do(t, "GET", local.URL+url, "", nil), &c)
	if c.Password != "s3cret" {
		t.Fatalf("local listener: %+v", c)
	}
	if r := do(t, "GET", local.URL+"/api/v1/devices", "", nil); r.StatusCode != 404 {
		t.Fatalf("local listener serves only /mcp and the credentials: %d", r.StatusCode)
	}
}
