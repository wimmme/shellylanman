package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wimmme/shellylanman/internal/service"
)

func TestActor(t *testing.T) {
	var got service.Actor
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = service.ActorOf(r.Context()) })
	for _, c := range []struct {
		path, xff string
		ingress   bool
		source    func(*http.Request) string
		want      service.Actor
	}{
		{"/api/v1/reboot", "", false, webSource, service.Actor{Source: service.SourceWeb, Addr: "192.168.0.5"}},
		{"/api/v1/reboot", "10.0.0.9", false, webSource, service.Actor{Source: service.SourceWeb, Addr: "192.168.0.5"}}, // not trusted here
		{"/mcp", "", false, webSource, service.Actor{Source: service.SourceMCP, Addr: "192.168.0.5"}},
		{"/api/v1/reboot", "192.168.0.7, 172.30.32.1", true, webSource, service.Actor{Source: service.SourceIngress, Addr: "192.168.0.7"}},
		{"/api/v1/reboot", "", true, webSource, service.Actor{Source: service.SourceIngress, Addr: "192.168.0.5"}},
		{"/api/v1/backup", "", false, localSource, service.Actor{Source: service.SourceIntegration, Addr: "192.168.0.5"}},
		{"/mcp", "", false, localSource, service.Actor{Source: service.SourceMCP, Addr: "192.168.0.5"}},
	} {
		r := httptest.NewRequest(http.MethodPost, c.path, nil)
		r.RemoteAddr = "192.168.0.5:4711"
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if c.ingress {
			r = r.WithContext(context.WithValue(r.Context(), ingressKey{}, true))
		}
		withActor(next, c.source).ServeHTTP(httptest.NewRecorder(), r)
		if got != c.want {
			t.Errorf("%s ingress=%v xff=%q: %+v, want %+v", c.path, c.ingress, c.xff, got, c.want)
		}
	}
	if a := service.ActorOf(context.Background()); a.Source != service.SourceSystem {
		t.Errorf("no actor: %+v", a)
	}
}
