package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMCPSettingsAndEndpoint(t *testing.T) {
	srv, _, _ := newDeviceServerStore(t)

	var info MCPInfo
	decode(t, do(t, "GET", srv.URL+"/api/v1/mcp", "", nil), &info)
	if info.Enabled || info.HasToken || info.Access != "read" {
		t.Fatalf("defaults: %+v", info)
	}
	// Off: /mcp does not exist.
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
