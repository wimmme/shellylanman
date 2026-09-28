package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/store"
)

func newTestServer(t *testing.T, static fstest.MapFS) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if static == nil {
		static = fstest.MapFS{
			"index.html": {Data: []byte("<!doctype html><title>ShellyLanMan</title>")},
			"app.js":     {Data: []byte("console.log(1)")},
		}
	}
	h := New(Config{Store: st, Hub: hub.New(nil, nil, nil), Static: static, Origins: []string{"proxy.example.net"}})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

func do(t *testing.T, method, url, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

var jsonHdr = map[string]string{"Content-Type": "application/json"}

func TestHealthz(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	resp := do(t, "GET", srv.URL+"/healthz", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(b), "version") {
		t.Fatal("healthz discloses the version")
	}
}

func TestAboutCreditsShellyScanner(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	var a About
	decode(t, do(t, "GET", srv.URL+"/api/v1/about", "", nil), &a)
	if len(a.BasedOn) != 2 || a.BasedOn[0].Name != "ShellyScanner" || !strings.Contains(a.BasedOn[0].URL, "usnasoft/shellyscanner") || a.BasedOn[1].Name != "MikroDash" {
		t.Fatalf("about does not credit ShellyScanner and MikroDash: %+v", a.BasedOn)
	}
	if a.Started == 0 || a.Runtime.Go == "" || len(a.Deps) == 0 {
		t.Fatalf("runtime information missing: %+v", a)
	}
	for _, p := range []string{"changelog", "license", "notices"} {
		resp := do(t, "GET", srv.URL+"/api/v1/about/"+p, "", nil)
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || len(b) < 100 {
			t.Fatalf("%s: %d, %d bytes", p, resp.StatusCode, len(b))
		}
	}
	if a.License != "GPL-3.0-or-later" || !strings.Contains(a.Notice, "independent") {
		t.Fatalf("licence/notice missing: %+v", a)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	srv, st := newTestServer(t, nil)
	resp := do(t, "PUT", srv.URL+"/api/v1/settings", `{"language":"nl","firstRunDone":true}`, jsonHdr)
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status %d: %s", resp.StatusCode, b)
	}
	if got := st.Settings(); got.Language != "nl" || !got.FirstRunDone {
		t.Fatalf("stored settings = %+v", got)
	}
	var s store.Settings
	decode(t, do(t, "GET", srv.URL+"/api/v1/settings", "", nil), &s)
	if s.Language != "nl" {
		t.Fatalf("GET settings = %+v", s)
	}
	// Partial update leaves other fields alone.
	do(t, "PUT", srv.URL+"/api/v1/settings", `{"language":"en"}`, jsonHdr)
	if got := st.Settings(); got.Language != "en" || !got.FirstRunDone {
		t.Fatalf("partial update changed too much: %+v", got)
	}
}

func TestSettingsRejectsBadInput(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	cases := []struct {
		name, body string
		hdr        map[string]string
		want       int
	}{
		{"wrong content type", `{"language":"nl"}`, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		{"unknown field", `{"langauge":"nl"}`, jsonHdr, http.StatusBadRequest},
		{"invalid language", `{"language":"xx"}`, jsonHdr, http.StatusBadRequest},
		{"cross origin", `{"language":"nl"}`, map[string]string{"Content-Type": "application/json", "Origin": "http://evil.example"}, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if resp := do(t, "PUT", srv.URL+"/api/v1/settings", c.body, c.hdr); resp.StatusCode != c.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
}

func TestConfiguredOriginAllowed(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	resp := do(t, "PUT", srv.URL+"/api/v1/settings", `{"language":"nl"}`,
		map[string]string{"Content-Type": "application/json", "Origin": "https://proxy.example.net"})
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestStatusReportsFirstRun(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	var s Status
	decode(t, do(t, "GET", srv.URL+"/api/v1/status", "", nil), &s)
	if s.FirstRunDone || s.AuthEnabled {
		t.Fatalf("fresh status = %+v", s)
	}
}

func TestSPAFallbackAndSecurityHeaders(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	resp := do(t, "GET", srv.URL+"/devices/some/route", "", nil)
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(b), "<title>ShellyLanMan") {
		t.Fatalf("SPA fallback: %d %q", resp.StatusCode, b)
	}
	if resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
	if ct := do(t, "GET", srv.URL+"/app.js", "", nil).Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("app.js content type %q", ct)
	}
}

func TestUnknownAPIIs404JSON(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	resp := do(t, "GET", srv.URL+"/api/v1/nope", "", nil)
	if resp.StatusCode != 404 || !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestFrontendNotBuilt(t *testing.T) {
	srv, _ := newTestServer(t, fstest.MapFS{".keep": {}})
	if resp := do(t, "GET", srv.URL+"/", "", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
