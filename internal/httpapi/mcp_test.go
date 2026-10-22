package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/store"
)

func TestMCPSettingsAndEndpoint(t *testing.T) {
	srv, _, _ := newDeviceServerStore(t)

	var info MCPInfo
	decode(t, do(t, "GET", srv.URL+"/api/v1/mcp", "", nil), &info)
	if !info.Enabled || info.HasToken || info.Access != "read" {
		t.Fatalf("defaults (on, read only, no token yet): %+v", info)
	}
	// On without a token: nobody gets in over the LAN.
	if r := do(t, "POST", srv.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, jsonHdr); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mcp without a token made: %d", r.StatusCode)
	}
	// Off: /mcp does not exist.
	decode(t, do(t, "PUT", srv.URL+"/api/v1/mcp", `{"enabled":false}`, jsonHdr), &info)
	if r := do(t, "POST", srv.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, jsonHdr); r.StatusCode != http.StatusNotFound {
		t.Fatalf("mcp while off: %d", r.StatusCode)
	}
	// Switching on makes a token, shown this once.
	decode(t, do(t, "PUT", srv.URL+"/api/v1/mcp", `{"enabled":true}`, jsonHdr), &info)
	if !info.Enabled || !info.HasToken || !strings.HasPrefix(info.Token, "slm_") {
		t.Fatalf("enable: %+v", info)
	}
	token := info.Token
	var again MCPInfo
	decode(t, do(t, "GET", srv.URL+"/api/v1/mcp", "", nil), &again)
	if again.Token != "" || !again.HasToken {
		t.Fatalf("token shown again: %+v", again)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/mcp", `{"access":"everything"}`, jsonHdr); r.StatusCode != 400 {
		t.Fatalf("bad access: %d", r.StatusCode)
	}
	var cfgd MCPInfo
	if decode(t, do(t, "PUT", srv.URL+"/api/v1/mcp", `{"access":"configure"}`, jsonHdr), &cfgd); cfgd.Access != "configure" {
		t.Fatalf("configure access: %+v", cfgd)
	}

	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	auth := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + token}
	if r := do(t, "POST", srv.URL+"/mcp", ping, auth); r.StatusCode != 200 {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("ping with token: %d %s", r.StatusCode, b)
	}
	if r := do(t, "POST", srv.URL+"/mcp", ping, jsonHdr); r.StatusCode != 401 {
		t.Fatalf("ping without token: %d", r.StatusCode)
	}
	// A browser page elsewhere cannot use a leaked token through the user's browser.
	cross := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + token, "Origin": "http://evil.example"}
	if r := do(t, "POST", srv.URL+"/mcp", ping, cross); r.StatusCode != 403 {
		t.Fatalf("cross-origin: %d", r.StatusCode)
	}

	// A new token replaces the old one; the endpoint wants JSON (CSRF).
	if r := do(t, "POST", srv.URL+"/api/v1/mcp/token", "", nil); r.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("token without JSON: %d", r.StatusCode)
	}
	decode(t, do(t, "POST", srv.URL+"/api/v1/mcp/token", `{}`, jsonHdr), &info)
	if info.Token == "" || info.Token == token {
		t.Fatalf("new token: %+v", info)
	}
	if r := do(t, "POST", srv.URL+"/mcp", ping, auth); r.StatusCode != 401 {
		t.Fatalf("old token still works: %d", r.StatusCode)
	}
}

func TestMCPLocalWithoutToken(t *testing.T) {
	_, devs, st := newDeviceServerStore(t)
	srv := httptest.NewServer(MCPLocal(Config{Store: st, Devices: devs}))
	t.Cleanup(srv.Close)
	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	st.Update(func(s *store.Settings) { s.MCP.Enabled = false })
	if r := do(t, "POST", srv.URL+"/mcp", ping, jsonHdr); r.StatusCode != http.StatusNotFound {
		t.Fatalf("MCP off: %d", r.StatusCode)
	}
	st.Update(func(s *store.Settings) { s.MCP.Enabled = true })
	if r := do(t, "POST", srv.URL+"/mcp", ping, jsonHdr); r.StatusCode != http.StatusOK {
		t.Fatalf("no token on the loopback listener: %d", r.StatusCode)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/status", "", nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("other paths served: %d", r.StatusCode)
	}
}

func TestMCPSwitchedOnHook(t *testing.T) {
	_, devs, st := newDeviceServerStore(t)
	calls := make(chan struct{}, 4)
	srv := httptest.NewServer(New(Config{Store: st, Hub: hub.New(nil, nil, nil), Devices: devs, MCPSwitchedOn: func() { calls <- struct{}{} }}))
	t.Cleanup(srv.Close)
	// Already on (the default): saving it again, or the access level, is no switch.
	do(t, "PUT", srv.URL+"/api/v1/mcp", `{"enabled":true}`, jsonHdr)
	do(t, "PUT", srv.URL+"/api/v1/mcp", `{"access":"control"}`, jsonHdr)
	do(t, "PUT", srv.URL+"/api/v1/mcp", `{"enabled":false}`, jsonHdr)
	if len(calls) != 0 {
		t.Fatalf("called %d times without switching on", len(calls))
	}
	do(t, "PUT", srv.URL+"/api/v1/mcp", `{"enabled":true}`, jsonHdr)
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("not called when switched on")
	}
}
