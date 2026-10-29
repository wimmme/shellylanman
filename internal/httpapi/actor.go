package httpapi

import (
	"net/http"
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
