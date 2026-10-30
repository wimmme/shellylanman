package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/wimmme/shellylanman/internal/service"
)

// withActor tells the service who asks, for its action log (DECISIONS P26-2):
// the web UI or API, the UI in Home Assistant's sidebar, or an MCP client, with
// the client's address. Behind the ingress the client is always the Supervisor;
// Home Assistant and the Supervisor each append the address they saw to
// X-Forwarded-For, so its first address is the browser's, or the one a proxy in
// front of Home Assistant passed on. Good for a log line, never for a decision.
func withActor(next http.Handler, source func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := service.Actor{Source: source(r), Addr: clientAddr(r)}
		if a.Source == service.SourceIngress {
			if first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ","); strings.TrimSpace(first) != "" {
				a.Addr = strings.TrimSpace(first)
			}
		}
		next.ServeHTTP(w, r.WithContext(service.WithActor(r.Context(), a)))
	})
}

// webSource: the main listener (and the ingress, which wraps it).
func webSource(r *http.Request) string {
	switch {
	case r.URL.Path == "/mcp":
		return service.SourceMCP
	case viaIngress(r):
		return service.SourceIngress
	}
	return service.SourceWeb
}

// localSource: the Home Assistant app's loopback listener, used by the
// integration and by local MCP clients.
func localSource(r *http.Request) string {
	if r.URL.Path == "/mcp" {
		return service.SourceMCP
	}
	return service.SourceIntegration
}

// action logs one of the HTTP layer's own actions (settings, UI password, MCP)
// like the service's (DECISIONS P26-5); err nil is ok.
func (s *server) action(r *http.Request, name string, err error, attrs ...any) {
	result, msg := service.ResultOK, ""
	if err != nil {
		result, msg = service.ResultFail, err.Error()
	}
	service.LogAction(r.Context(), s.Log, name, service.DeviceRef{}, result, msg, attrs...)
}

// changedSettings names the top-level settings that differ between before and
// after, each with its new value as compact JSON (the settings hold no secrets:
// those are in the store's secrets).
func changedSettings(before, after any) []any {
	var b, a map[string]json.RawMessage
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(after)
	if json.Unmarshal(bj, &b) != nil || json.Unmarshal(aj, &a) != nil {
		return nil
	}
	keys := make([]string, 0, len(a))
	for k, v := range a {
		if string(b[k]) != string(v) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	out := []any{"changed", strings.Join(keys, ",")}
	for _, k := range keys {
		out = append(out, k, string(a[k]))
	}
	return out
}
