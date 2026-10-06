package httpapi

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/logbuf"
	"github.com/wimmme/shellylanman/internal/store"
)

func TestLogEndpoint(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logs := logbuf.New(10)
	l := slog.New(logs.Handler(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	l.Info("first", "device", "Grondwaterpomp")
	l.Warn("second")
	srv := httptest.NewServer(New(Config{Store: st, Hub: hub.New(nil, nil, nil), Logs: logs,
		Static: fstest.MapFS{"index.html": {Data: []byte("x")}}}))
	defer srv.Close()

	var all LogResponse
	decode(t, do(t, "GET", srv.URL+"/api/v1/log", "", nil), &all)
	if len(all.Entries) != 2 || all.Entries[0].Msg != "first" || all.Entries[0].Attrs != "device=Grondwaterpomp" || all.Entries[1].Level != "WARN" {
		t.Fatalf("log %+v", all)
	}
	var rest LogResponse
	decode(t, do(t, "GET", srv.URL+"/api/v1/log?after="+itoa(all.Entries[0].Seq), "", nil), &rest)
	if len(rest.Entries) != 1 || rest.Entries[0].Msg != "second" {
		t.Fatalf("after: %+v", rest)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/log?after=x", "", nil); r.StatusCode != 400 {
		t.Fatalf("bad after: %d", r.StatusCode)
	}
	if r := do(t, "DELETE", srv.URL+"/api/v1/log", "", nil); r.StatusCode == 200 {
		t.Fatal("the log cannot be changed through the API")
	}
}

func TestLogEndpointWithoutBuffer(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	srv := httptest.NewServer(New(Config{Store: st, Hub: hub.New(nil, nil, nil), Static: fstest.MapFS{"index.html": {Data: []byte("x")}}}))
	defer srv.Close()
	var got LogResponse
	decode(t, do(t, "GET", srv.URL+"/api/v1/log", "", nil), &got)
	if got.Entries == nil || len(got.Entries) != 0 {
		t.Fatalf("an empty list, not null: %+v", got)
	}
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }
