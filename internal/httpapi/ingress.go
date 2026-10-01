package httpapi

import (
	"context"
	"net"
	"net/http"
)

// Home Assistant ingress (Phase 11b, docs/phase-11-ha-mcp.md §5): as a Home
// Assistant app, ShellyLanMan gets a second listener that only the Supervisor
// can reach. Requests there were authenticated by Home Assistant; the
// Supervisor strips the /api/hassio_ingress/<token> prefix and forwards the
// browser's Host and Origin, so the same-origin check works unchanged.

type ingressKey struct{}

// Ingress wraps the handler for the ingress listener: requests from any
// address other than from (the Supervisor) are refused, the others are
// marked so the UI knows it runs inside Home Assistant.
func Ingress(next http.Handler, from string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || host != from {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ingressKey{}, true)))
	})
}

// viaIngress reports whether r came through the Home Assistant ingress.
func viaIngress(r *http.Request) bool {
	v, _ := r.Context().Value(ingressKey{}).(bool)
	return v
}
