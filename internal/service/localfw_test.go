package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/firmware"
)

func fakeGen1Index(t *testing.T, build string) *httptest.Server {
	t.Helper()
	bin := []byte("firmware")
	sum := sha256.Sum256(bin)
	m, _ := json.Marshal(map[string]any{"build_id": build, "parts": map[string]any{"fw": map[string]string{"src": "p.bin", "cs_sha256": hex.EncodeToString(sum[:])}}})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("plug/manifest.json")
	w.Write(m)
	w, _ = zw.Create("plug/p.bin")
	w.Write(bin)
	zw.Close()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			json.NewEncoder(w).Encode(map[string]any{"isok": true, "data": map[string]any{"SHPLG-S": map[string]string{"url": srv.URL + "/SHPLG-S.zip", "version": build}}})
		case "/SHPLG-S.zip":
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLocalFirmwareDownload(t *testing.T) {
	m, st, ctx := newService(t, nil)
	plugS(t, m, ctx, map[string]string{"shelly.json": `{"type":"SHPLG-S","mac":"AABBCC000001","auth":false,"fw":"20230913-113421/v1.14.0-gcb84623"}`}, "AABBCC000001")
	srv := fakeGen1Index(t, "20260101-000000/v1.15.0-gabcdef0")
	x := firmware.New(st.Dir() + "/firmware")
	x.Gen1URL, x.HTTP = srv.URL+"/index", srv.Client()
	x.ArchiveURL = srv.URL + "/none?type="
	m.SetFirmwareSource(x)

	rows, err := m.FirmwareIndex(ctx, nil)
	if err != nil || len(rows) != 1 || rows[0].Latest != "1.15.0" || rows[0].Current != "1.14.0" || !rows[0].Newer || rows[0].Source != "shelly" {
		t.Fatalf("index rows %+v %v", rows, err)
	}
	link, err := m.LocalDownload(ctx, "AABBCC000001", "http://192.168.1.2:3082/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link.URL, "http://192.168.1.2:3082/fw/") || !strings.HasSuffix(link.URL, "/SHPLG-S.zip") ||
		!strings.HasPrefix(link.QR, "data:image/png;base64,") || link.Version != "1.15.0" || link.Warning != "" {
		t.Fatalf("link %+v", link)
	}
	token := strings.Split(strings.TrimPrefix(link.URL, "http://192.168.1.2:3082/fw/"), "/")[0]
	path, name, err := m.FirmwareFile(ctx, token)
	if err != nil || name != "SHPLG-S.zip" {
		t.Fatalf("file %s %s %v", path, name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.FirmwareFile(ctx, token[:len(token)-3]+"xyz"); !errors.Is(err, ErrLinkExpired) {
		t.Fatalf("tampered token: %v", err)
	}
	if l, _ := m.LocalDownload(ctx, "AABBCC000001", "http://localhost:3082"); l == nil || l.Warning != "localhost" {
		t.Fatalf("localhost warning %+v", l)
	}
}

func TestLocalFirmwareUpToDate(t *testing.T) {
	m, st, ctx := newService(t, nil)
	plugS(t, m, ctx, map[string]string{"shelly.json": `{"type":"SHPLG-S","mac":"AABBCC000001","auth":false,"fw":"20230913-113421/v1.14.0-gcb84623"}`}, "AABBCC000001")
	srv := fakeGen1Index(t, "20230913-113421/v1.14.0-gcb84623")
	x := firmware.New(st.Dir() + "/firmware")
	x.Gen1URL, x.HTTP = srv.URL+"/index", srv.Client()
	m.SetFirmwareSource(x)
	if rows, _ := m.FirmwareIndex(ctx, nil); len(rows) != 1 || rows[0].Newer {
		t.Fatalf("rows %+v", rows)
	}
	if _, err := m.LocalDownload(ctx, "AABBCC000001", "http://h"); !errors.Is(err, ErrUpToDate) {
		t.Fatalf("up to date: %v", err)
	}
}
