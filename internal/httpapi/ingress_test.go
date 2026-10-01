package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/store"
)

func TestIngressOnlyFromSupervisorAndMarked(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := New(Config{Store: st, Hub: hub.New(nil, nil, nil)})

	// The httptest client connects from 127.0.0.1: allowed only when that is the Supervisor.
	refused := httptest.NewServer(Ingress(h, "172.30.32.2"))
	t.Cleanup(refused.Close)
	if r := do(t, "GET", refused.URL+"/api/v1/status", "", nil); r.StatusCode != http.StatusForbidden {
		t.Fatalf("other client: %d", r.StatusCode)
	}

	ing := httptest.NewServer(Ingress(h, "127.0.0.1"))
	t.Cleanup(ing.Close)
	var s Status
	decode(t, do(t, "GET", ing.URL+"/api/v1/status", "", nil), &s)
	if !s.Ingress {
		t.Fatalf("status through ingress: %+v", s)
	}
	// State changes keep the same-origin check: the Supervisor forwards the browser's Host and Origin.
	hdr := map[string]string{"Content-Type": "application/json", "Origin": ing.URL}
	if r := do(t, "PUT", ing.URL+"/api/v1/settings", `{"language":"nl"}`, hdr); r.StatusCode != http.StatusOK {
		t.Fatalf("same-origin PUT through ingress: %d", r.StatusCode)
	}
	hdr["Origin"] = "https://evil.example"
	if r := do(t, "PUT", ing.URL+"/api/v1/settings", `{"language":"en"}`, hdr); r.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin through ingress: %d", r.StatusCode)
	}

	direct := httptest.NewServer(h)
	t.Cleanup(direct.Close)
	decode(t, do(t, "GET", direct.URL+"/api/v1/status", "", nil), &s)
	if s.Ingress {
		t.Fatal("direct request marked as ingress")
	}
}
