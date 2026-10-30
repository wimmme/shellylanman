package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/store"
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

// syncBuf is a log sink for the tests.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestOwnActionsLogged(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuf{}
	srv := httptest.NewServer(New(Config{Store: st, Hub: hub.New(nil, nil, nil), Static: fstest.MapFS{"index.html": {Data: []byte("x")}},
		Log: slog.New(slog.NewTextHandler(logs, nil))}))
	t.Cleanup(srv.Close)
	api := srv.URL + "/api/v1"

	if r := do(t, "PUT", api+"/settings", `{"backupKeep":7,"language":"nl"}`, jsonHdr); r.StatusCode != 200 {
		t.Fatalf("settings %d", r.StatusCode)
	}
	if r := do(t, "PUT", api+"/settings", `{"backupKeep":7}`, jsonHdr); r.StatusCode != 200 { // no change: no line
		t.Fatalf("settings %d", r.StatusCode)
	}
	if r := do(t, "PUT", api+"/mcp", `{"access":"control"}`, jsonHdr); r.StatusCode != 200 {
		t.Fatalf("mcp %d", r.StatusCode)
	}
	if r := do(t, "PUT", api+"/auth/password", `{"password":"Secret pass 1"}`, jsonHdr); r.StatusCode != 200 {
		t.Fatalf("password %d", r.StatusCode)
	}
	if r := do(t, "POST", api+"/auth/login", `{"password":"wrong one"}`, jsonHdr); r.StatusCode != 401 {
		t.Fatalf("wrong login %d", r.StatusCode)
	}
	out := logs.String()
	for _, want := range []string{
		`msg="action settings" changed=backupKeep,language backupKeep=7 language="\"nl\"" result=ok source=web addr=127.0.0.1`,
		`msg="action mcp.settings" enabled=`,
		`msg="action password.set" result=ok source=web`,
		`level=WARN msg="action login" tries=1 result=fail`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if n := strings.Count(out, `msg="action settings"`); n != 1 {
		t.Errorf("%d settings lines, want 1 (an unchanged patch logs nothing)", n)
	}
	if strings.Contains(out, "Secret pass") || strings.Contains(out, "wrong one") {
		t.Errorf("a password in the log:\n%s", out)
	}
}
