package shelly

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", rel))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseInfoGen1AndGen2(t *testing.T) {
	g1, err := ParseInfo(readFixture(t, "gen1/SHPLG-S/shelly.json"))
	if err != nil || g1.Generation() != "1" || g1.Type != "SHPLG-S" || g1.MAC != "AABBCC000001" || g1.AuthEnabled() {
		t.Fatalf("gen1 info = %+v, %v", g1, err)
	}
	g2, err := ParseInfo(readFixture(t, "gen2/Plus1/shelly.json"))
	if err != nil || g2.Generation() != "2" || g2.App != "Plus1" || g2.Model != "SNSW-001X16EU" || g2.ID == "" {
		t.Fatalf("gen2 info = %+v, %v", g2, err)
	}
	if _, err := ParseInfo([]byte(`{"hello":"world"}`)); !errors.Is(err, ErrNotShelly) {
		t.Fatalf("non-Shelly accepted: %v", err)
	}
	xt1, _ := ParseInfo([]byte(`{"mac":"X","gen":3,"app":"XT1","svc0":{"type":"linkedgo-st-802-hvac"}}`))
	if xt1.Svc0Type != "linkedgo-st-802-hvac" {
		t.Fatalf("svc0.type not read: %+v", xt1)
	}
}

func TestParseChallenge(t *testing.T) {
	ds, ok := parseChallenge(`Digest qop="auth", realm="shellyplus1-aabbcc000001", nonce="1770886322", algorithm=SHA-256`)
	if !ok || ds.realm != "shellyplus1-aabbcc000001" || ds.nonce != "1770886322" || ds.qop != "auth" {
		t.Fatalf("challenge = %+v, %v", ds, ok)
	}
	if _, ok := parseChallenge(`Basic realm="x"`); ok {
		t.Fatal("basic challenge accepted as digest")
	}
	if _, ok := parseChallenge(`Digest realm="x", nonce="1", algorithm=MD5`); ok {
		t.Fatal("MD5 accepted")
	}
}

func TestDigestResponseKnownVector(t *testing.T) {
	// Pins the RFC 7616 SHA-256 formula: HA1=sha256("admin:realm:pw"),
	// HA2=sha256("GET:/rpc/Shelly.GetStatus"). Real-device verification happens
	// on hardware (docs/hardware-tests.md).
	got := DigestResponse("admin", "realm", "pw", "GET", "/rpc/Shelly.GetStatus", "123", "00000001", "abc", "auth")
	ha1 := sha256hex("admin:realm:pw")
	ha2 := sha256hex("GET:/rpc/Shelly.GetStatus")
	want := sha256hex(ha1 + ":123:00000001:abc:auth:" + ha2)
	if got != want || len(got) != 64 {
		t.Fatalf("response %s, want %s", got, want)
	}
}

// digestServer is a minimal protected Gen2 endpoint checking the digest response.
func digestServer(t *testing.T, password string) (*httptest.Server, *atomic.Int32) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		h := r.Header.Get("Authorization")
		if h == "" {
			w.Header().Set("WWW-Authenticate", `Digest qop="auth", realm="dev1", nonce="42", algorithm=SHA-256`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p := parseParams(strings.TrimPrefix(h, "Digest "))
		want := DigestResponse(p["username"], "dev1", password, r.Method, p["uri"], "42", p["nc"], p["cnonce"], p["qop"])
		if p["response"] != want || p["uri"] != r.URL.RequestURI() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestDigestAuthRoundTrip(t *testing.T) {
	srv, _ := digestServer(t, "secret")
	c := NewClient().Conn(strings.TrimPrefix(srv.URL, "http://"), false)

	if _, err := c.Get(context.Background(), "/rpc/Shelly.GetStatus"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("without credentials: %v", err)
	}
	c.SetCredentials(&Credentials{Password: "wrong"})
	if _, err := c.Get(context.Background(), "/rpc/Shelly.GetStatus"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	c.SetCredentials(&Credentials{Password: "secret"})
	b, err := c.Get(context.Background(), "/rpc/Shelly.GetStatus?x=1")
	if err != nil || string(b) != `{"ok":true}` {
		t.Fatalf("right password: %q %v", b, err)
	}
	// Second call reuses the nonce (nc incremented) without a new challenge.
	if _, err := c.Get(context.Background(), "/rpc/Shelly.GetConfig"); err != nil {
		t.Fatal(err)
	}
}

func TestBasicAuthGen1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient().Conn(strings.TrimPrefix(srv.URL, "http://"), true)
	if _, err := c.Get(context.Background(), "/settings"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	c.SetCredentials(&Credentials{User: "admin", Password: "pw"})
	if _, err := c.Get(context.Background(), "/settings"); err != nil {
		t.Fatal(err)
	}
}

func TestPacingAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/KVS.GetMany":
			w.Write([]byte(`{"code":-114,"message":"Method KVS.GetMany failed: No such component"}`))
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := NewClient().Conn(strings.TrimPrefix(srv.URL, "http://"), false)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), "/x"); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 2*Pacing {
		t.Fatalf("3 requests took %v; pacing not applied", el)
	}
	var api *APIError
	if _, err := c.Get(context.Background(), "/rpc/KVS.GetMany"); !errors.As(err, &api) || api.Code != -114 {
		t.Fatalf("error in body not detected: %v", err)
	}
	if _, err := c.Get(context.Background(), "/missing"); !errors.As(err, &api) || api.HTTPStatus != 404 {
		t.Fatalf("404 not an APIError: %v", err)
	}
	srv.Close()
	if _, err := c.Get(context.Background(), "/x"); !IsOffline(err) {
		t.Fatalf("closed server not offline: %v", err)
	}
}

// A device's realm, nonce and opaque are quoted, so they cannot add header
// parameters or lines of their own.
func TestDigestHeaderQuotesDeviceValues(t *testing.T) {
	d := &digestState{realm: `shelly", evil="1`, nonce: "n\r\nX-Injected: 1", opaque: `o\`}
	h := d.header("GET", "/rpc/Shelly.GetStatus", "pw")
	if strings.ContainsAny(h, "\r\n") || strings.Contains(h, `evil="1"`) {
		t.Fatalf("header not quoted: %s", h)
	}
	if !strings.Contains(h, `realm="shelly\", evil=\"1"`) || !strings.HasSuffix(h, `opaque="o\\"`) {
		t.Fatalf("header = %s", h)
	}
}
