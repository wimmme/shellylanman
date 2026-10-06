package httpapi

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/store"
)

func TestServerInfo(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ports := Ports{Source: "app", App: true, Ingress: "172.30.32.1:62487", MCPLocal: "127.0.0.1:8097"}
	srv := httptest.NewServer(New(Config{Store: st, Hub: hub.New(nil, nil, nil), Port: func() int { return 3090 }, Ports: ports,
		Static: fstest.MapFS{"index.html": {Data: []byte("x")}}}))
	defer srv.Close()
	var got ServerInfo
	decode(t, do(t, "GET", srv.URL+"/api/v1/server", "", nil), &got)
	if got.Port != 3090 || got.Ports != ports {
		t.Fatalf("server info %+v", got)
	}
	var stt Status
	decode(t, do(t, "GET", srv.URL+"/api/v1/status", "", nil), &stt)
	if stt.LocalURL != "http://127.0.0.1:8097" {
		t.Fatalf("status localUrl %q", stt.LocalURL)
	}
	// The port is set outside ShellyLanMan now: no way to change it here.
	if r := do(t, "PUT", srv.URL+"/api/v1/server", `{"port":4000,"confirm":true}`, jsonHdr); r.StatusCode != 405 {
		t.Fatalf("PUT /api/v1/server %d", r.StatusCode)
	}
}
