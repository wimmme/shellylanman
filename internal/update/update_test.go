package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wimmme/shellylanman/internal/store"
)

func TestCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v1.3.0-rc1","prerelease":true,"html_url":"u-rc"},{"tag_name":"v1.2.0","html_url":"u-120"},{"tag_name":"v1.1.0","html_url":"u-110"},{"tag_name":"v9.9.9","draft":true}]`))
	}))
	defer srv.Close()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var events int
	c := New(st, "v1.1.0", func(Status) { events++ })
	c.URL = srv.URL

	if s := c.Check(context.Background()); s.Mode != Never || s.Latest != "" {
		t.Fatalf("off by default: %+v", s)
	}
	st.Update(func(s *store.Settings) { s.UpdateCheck = Stable })
	if s := c.Check(context.Background()); !s.Newer || s.Latest != "v1.2.0" || s.URL != "u-120" {
		t.Fatalf("stable: %+v", s)
	}
	st.Update(func(s *store.Settings) { s.UpdateCheck = All })
	if s := c.Check(context.Background()); s.Latest != "v1.3.0-rc1" {
		t.Fatalf("all: %+v", s)
	}
	st.Update(func(s *store.Settings) { s.UpdateCheck = Stable; s.SkipVersion = "v1.2.0" })
	if s := c.Check(context.Background()); s.Newer || !s.Skipped {
		t.Fatalf("skipped: %+v", s)
	}
	dev := New(st, "dev", nil)
	dev.URL = srv.URL
	if s := dev.Check(context.Background()); s.Newer {
		t.Fatalf("a dev build is never told to update: %+v", s)
	}
	if events < 3 {
		t.Fatalf("change events: %d", events)
	}
}
