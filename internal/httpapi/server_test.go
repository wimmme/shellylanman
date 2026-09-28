package httpapi

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/listen"
	"github.com/wimmme/shellylanman/internal/store"
)

type fakeListener struct {
	info  listen.Info
	moved []int
}

func (f *fakeListener) Info() listen.Info { return f.info }
func (f *fakeListener) Move(p int) error {
	if f.info.Fixed {
		return listen.ErrFixed
	}
	f.moved = append(f.moved, p)
	f.info.Port = p
	return nil
}

func TestServerPort(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fl := &fakeListener{info: listen.Info{Port: 3082}}
	srv := httptest.NewServer(New(Config{Store: st, Hub: hub.New(nil, nil, nil), Listener: fl, Static: fstest.MapFS{"index.html": {Data: []byte("x")}}}))
	defer srv.Close()

	var info listen.Info
	decode(t, do(t, "GET", srv.URL+"/api/v1/server", "", nil), &info)
	if info.Port != 3082 || info.Fixed {
		t.Fatalf("info %+v", info)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/server", `{"port":8095}`, jsonHdr); r.StatusCode != 428 {
		t.Fatalf("move without confirm: %d", r.StatusCode)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/server", `{"port":8095,"confirm":true}`, jsonHdr); r.StatusCode != 200 || len(fl.moved) != 1 || fl.moved[0] != 8095 {
		t.Fatalf("move: %d %v", r.StatusCode, fl.moved)
	}
	fl.info.Fixed = true
	if r := do(t, "PUT", srv.URL+"/api/v1/server", `{"port":9000,"confirm":true}`, jsonHdr); r.StatusCode != 409 {
		t.Fatalf("fixed address moved: %d", r.StatusCode)
	}
}
