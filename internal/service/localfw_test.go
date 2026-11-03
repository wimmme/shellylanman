package service

import (
	"archive/zip"
	"bytes"
	"context"
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
	"github.com/wimmme/shellylanman/internal/store"
)

// fakeGen1Index: Shelly's Gen1 index with one stable and one beta for the Plug S, both
// files served and valid.
func fakeGen1Index(t *testing.T, build string) *httptest.Server {
	t.Helper()
	const betaBuild = "20260201-000000/v1.16.0-beta1-gabcdef0"
	zipOf := func(build string) []byte {
		bin := []byte("firmware " + build)
		sum := sha256.Sum256(bin)
		m, _ := json.Marshal(map[string]any{"build_id": build, "parts": map[string]any{"fw": map[string]string{"src": "p.bin", "cs_sha256": hex.EncodeToString(sum[:])}}})
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("plug/manifest.json")
		w.Write(m)
		w, _ = zw.Create("plug/p.bin")
		w.Write(bin)
		zw.Close()
		return buf.Bytes()
	}
	stable, beta := zipOf(build), zipOf(betaBuild)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			json.NewEncoder(w).Encode(map[string]any{"isok": true, "data": map[string]any{"SHPLG-S": map[string]string{"url": srv.URL + "/SHPLG-S.zip", "version": build,
				"beta_url": srv.URL + "/beta/SHPLG-S.zip", "beta_ver": betaBuild}}})
		case "/SHPLG-S.zip":
			w.Write(stable)
		case "/beta/SHPLG-S.zip":
			w.Write(beta)
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
	link, err := m.LocalDownload(ctx, "AABBCC000001", "http://192.168.1.2:3082/", "")
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
	if l, _ := m.LocalDownload(ctx, "AABBCC000001", "http://localhost:3082", ""); l == nil || l.Warning != "localhost" {
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
	rows, _ := m.FirmwareIndex(ctx, nil)
	if len(rows) != 1 || rows[0].Newer || rows[0].Beta != "1.16.0-beta1" || !rows[0].BetaNew {
		t.Fatalf("rows %+v", rows)
	}
	// The wizard may flash the same version again (DECISIONS P29-3): marked, not refused.
	if l, err := m.LocalDownload(ctx, "AABBCC000001", "http://h", ""); err != nil || !l.Reflash || l.Version != "1.14.0" {
		t.Fatalf("re-flash %+v %v", l, err)
	}
	if l, err := m.LocalDownload(ctx, "AABBCC000001", "http://h", StageBeta); err != nil || l.Reflash || l.Version != "1.16.0-beta1" {
		t.Fatalf("beta %+v %v", l, err)
	}
	// An update through the API still only goes to a newer version.
	st.Update(localMode)
	if res, _ := m.FirmwareUpdate(ctx, []FirmwareRequest{{ID: "AABBCC000001", Stage: StageStable}}); res[0].Result == ResultOK {
		t.Fatalf("update to the same version %+v", res)
	}
}

func TestSetNote(t *testing.T) {
	m, st, ctx := newService(t, nil)
	plugS(t, m, ctx, nil, "AABBCC000001")
	long := strings.Repeat("é", 40)
	if err := m.SetNote(context.Background(), "AABBCC000001", "line 1\nline 2", long); err != nil {
		t.Fatal(err)
	}
	d, _ := m.Get("AABBCC000001")
	if d.Note != "line 1\nline 2" || len([]rune(d.Keyword)) != 32 {
		t.Fatalf("note %q keyword %q", d.Note, d.Keyword)
	}
	if err := m.SetNote(context.Background(), "NOPE", "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	st.Update(func(s *store.Settings) { s.Archive.Use = false })
	if err := m.SetNote(context.Background(), "AABBCC000001", "x", ""); !errors.Is(err, ErrNoArchive) {
		t.Fatalf("archive off: %v", err)
	}
}
