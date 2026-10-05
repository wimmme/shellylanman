package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/auth"
	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/store"
)

func cookieOf(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie (status %d)", cookieName, resp.StatusCode)
	return nil
}

func withCookie(c *http.Cookie, extra map[string]string) map[string]string {
	h := map[string]string{"Cookie": c.Name + "=" + c.Value}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func statusOf(t *testing.T, url string, hdr map[string]string) Status {
	t.Helper()
	var st Status
	decode(t, do(t, "GET", url+"/api/v1/status", "", hdr), &st)
	return st
}

func TestNoPasswordEverythingOpen(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	if st := statusOf(t, srv.URL, nil); st.AuthEnabled || !st.LoggedIn {
		t.Fatalf("status %+v", st)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/settings", "", nil); r.StatusCode != 200 {
		t.Fatalf("settings %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/auth/login", `{"password":"x"}`, jsonHdr); r.StatusCode != http.StatusConflict {
		t.Fatalf("login without password %d", r.StatusCode)
	}
}

func TestPasswordLifecycle(t *testing.T) {
	srv, st := newTestServer(t, nil)
	api := srv.URL + "/api/v1"

	// The rules of P15-7.
	for pw, code := range map[string]string{"Short1": "tooShort", "longenough": "noCapital"} {
		r := do(t, "PUT", api+"/auth/password", `{"password":"`+pw+`"}`, jsonHdr)
		var e authError
		decode(t, r, &e)
		if r.StatusCode != 400 || e.Code != code {
			t.Fatalf("%q: %d %+v", pw, r.StatusCode, e)
		}
	}
	// Set: the browser that set it stays logged in.
	r := do(t, "PUT", api+"/auth/password", `{"password":"Secret pass"}`, jsonHdr)
	if r.StatusCode != 200 {
		t.Fatalf("set %d", r.StatusCode)
	}
	setter := cookieOf(t, r)
	if !setter.HttpOnly || setter.SameSite != http.SameSiteStrictMode || setter.MaxAge != 0 {
		t.Fatalf("cookie %+v", setter)
	}
	if h, _, _ := st.Secret(auth.PasswordSecret); !strings.HasPrefix(h, "pbkdf2-sha256$") {
		t.Fatalf("stored %q", h)
	}

	// Without a session: the page loads, the API and the WebSocket do not.
	if s := statusOf(t, srv.URL, nil); !s.AuthEnabled || s.LoggedIn || s.FirstRunDone {
		t.Fatalf("status %+v", s)
	}
	for _, p := range []string{"/", "/app.js", "/healthz"} {
		if r := do(t, "GET", srv.URL+p, "", nil); r.StatusCode != 200 {
			t.Fatalf("%s: %d", p, r.StatusCode)
		}
	}
	for _, p := range []string{"/api/v1/settings", "/api/v1/devices", "/ws", "/api/v1/about"} {
		if r := do(t, "GET", srv.URL+p, "", nil); r.StatusCode != 401 {
			t.Fatalf("%s without session: %d", p, r.StatusCode)
		}
	}
	if r := do(t, "GET", api+"/settings", "", withCookie(setter, nil)); r.StatusCode != 200 {
		t.Fatalf("settings with session %d", r.StatusCode)
	}

	// Log in: wrong, then right with "Stay logged in".
	r = do(t, "POST", api+"/auth/login", `{"password":"secret pass"}`, jsonHdr)
	var e authError
	decode(t, r, &e)
	if r.StatusCode != 401 || e.Code != "wrong" {
		t.Fatalf("wrong password %d %+v", r.StatusCode, e)
	}
	r = do(t, "POST", api+"/auth/login", `{"password":"Secret pass","remember":true}`, jsonHdr)
	if r.StatusCode != 204 {
		t.Fatalf("login %d", r.StatusCode)
	}
	tablet := cookieOf(t, r)
	if tablet.MaxAge != int(auth.IdleLimit.Seconds()) {
		t.Fatalf("remember cookie %+v", tablet)
	}
	if s := statusOf(t, srv.URL, withCookie(tablet, nil)); !s.LoggedIn {
		t.Fatalf("status with session %+v", s)
	}

	// Log out ends that session only.
	if r := do(t, "POST", api+"/auth/logout", "", withCookie(tablet, nil)); r.StatusCode != 204 || cookieOf(t, r).MaxAge >= 0 {
		t.Fatalf("logout %d", r.StatusCode)
	}
	if r := do(t, "GET", api+"/settings", "", withCookie(tablet, nil)); r.StatusCode != 401 {
		t.Fatalf("after logout %d", r.StatusCode)
	}

	// Change: the current password is needed; every other session ends.
	r = do(t, "POST", api+"/auth/login", `{"password":"Secret pass"}`, jsonHdr)
	other := cookieOf(t, r)
	if r := do(t, "PUT", api+"/auth/password", `{"current":"nope","password":"New Secret 2"}`, withCookie(setter, jsonHdr)); r.StatusCode != 401 {
		t.Fatalf("change with wrong current %d", r.StatusCode)
	}
	r = do(t, "PUT", api+"/auth/password", `{"current":"Secret pass","password":"New Secret 2"}`, withCookie(setter, jsonHdr))
	if r.StatusCode != 200 {
		t.Fatalf("change %d", r.StatusCode)
	}
	changer := cookieOf(t, r)
	if r := do(t, "GET", api+"/settings", "", withCookie(other, nil)); r.StatusCode != 401 {
		t.Fatalf("other session survived the change: %d", r.StatusCode)
	}
	if r := do(t, "GET", api+"/settings", "", withCookie(changer, nil)); r.StatusCode != 200 {
		t.Fatalf("changer logged out: %d", r.StatusCode)
	}

	// Switch off: everything open again.
	r = do(t, "PUT", api+"/auth/password", `{"current":"New Secret 2","password":""}`, withCookie(changer, jsonHdr))
	if r.StatusCode != 200 {
		t.Fatalf("off %d", r.StatusCode)
	}
	if r := do(t, "GET", api+"/settings", "", nil); r.StatusCode != 200 {
		t.Fatalf("after off %d", r.StatusCode)
	}
}

func setPassword(t *testing.T, st *store.Store, pw string) {
	t.Helper()
	h, err := auth.Hash(pw)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecret(auth.PasswordSecret, h); err != nil {
		t.Fatal(err)
	}
}

func TestMCPTokenOpensAPI(t *testing.T) {
	srv, st := newTestServer(t, nil)
	setPassword(t, st, "Secret pass")
	if _, err := st.Update(func(s *store.Settings) { s.MCP.Enabled = true; s.MCP.Access = "read" }); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecret(store.MCPTokenSecret, "tok-123"); err != nil {
		t.Fatal(err)
	}
	bearer := map[string]string{"Authorization": "Bearer tok-123", "Content-Type": "application/json"}
	if r := do(t, "GET", srv.URL+"/api/v1/settings", "", bearer); r.StatusCode != 200 {
		t.Fatalf("GET with token %d", r.StatusCode)
	}
	if s := statusOf(t, srv.URL, bearer); !s.LoggedIn {
		t.Fatalf("status with token %+v", s)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/settings", `{"mqttSlow":1}`, bearer); r.StatusCode != 401 {
		t.Fatalf("PUT with a read-only token %d", r.StatusCode)
	}
	if _, err := st.Update(func(s *store.Settings) { s.MCP.Access = "control" }); err != nil {
		t.Fatal(err)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/settings", `{"mqttSlow":1}`, bearer); r.StatusCode != 200 {
		t.Fatalf("PUT with a control token %d", r.StatusCode)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/settings", "", map[string]string{"Authorization": "Bearer wrong"}); r.StatusCode != 401 {
		t.Fatalf("wrong token %d", r.StatusCode)
	}
}

func TestIngressNeedsNoLogin(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setPassword(t, st, "Secret pass")
	h := New(Config{Store: st, Hub: hub.New(nil, nil, nil)})
	ing := httptest.NewServer(Ingress(h, "127.0.0.1"))
	defer ing.Close()
	if r := do(t, "GET", ing.URL+"/api/v1/settings", "", nil); r.StatusCode != 200 {
		t.Fatalf("ingress %d", r.StatusCode)
	}
	if s := statusOf(t, ing.URL, nil); !s.AuthEnabled || !s.LoggedIn || !s.Ingress {
		t.Fatalf("ingress status %+v", s)
	}
	direct := httptest.NewServer(h)
	defer direct.Close()
	if r := do(t, "GET", direct.URL+"/api/v1/settings", "", nil); r.StatusCode != 401 {
		t.Fatalf("LAN port %d", r.StatusCode)
	}
}

func TestWrongPasswordsSlowDown(t *testing.T) {
	srv, st := newTestServer(t, nil)
	setPassword(t, st, "Secret pass")
	var e authError
	for i := 1; i <= auth.FreeTries; i++ {
		r := do(t, "POST", srv.URL+"/api/v1/auth/login", `{"password":"wrong"}`, jsonHdr)
		e = authError{}
		decode(t, r, &e)
		if r.StatusCode != 401 {
			t.Fatalf("try %d: %d", i, r.StatusCode)
		}
	}
	if e.RetryAfter != 1 {
		t.Fatalf("after %d tries %+v", auth.FreeTries, e)
	}
	// Right away again, even with the right password: wait first.
	r := do(t, "POST", srv.URL+"/api/v1/auth/login", `{"password":"Secret pass"}`, jsonHdr)
	if r.StatusCode != http.StatusTooManyRequests || r.Header.Get("Retry-After") == "" {
		t.Fatalf("no wait: %d", r.StatusCode)
	}
}

func TestResetPassword(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setPassword(t, st, "Secret pass")
	if err := st.SaveJSON(SessionsFile, map[string]auth.Session{"x": {LastUsed: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := ResetPassword(st); err != nil {
		t.Fatal(err)
	}
	if h, set, _ := st.Secret(auth.PasswordSecret); set || h != "" {
		t.Fatal("password still set")
	}
	var saved map[string]auth.Session
	if _, err := st.LoadJSON(SessionsFile, &saved); err != nil || len(saved) != 0 {
		t.Fatalf("sessions %v %v", saved, err)
	}
}
