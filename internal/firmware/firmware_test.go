package firmware

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mkzip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	return buf.Bytes()
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func gen1Zip(t *testing.T, build string, corrupt bool) []byte {
	bin := "firmware-bytes"
	m, _ := json.Marshal(map[string]any{"build_id": build, "name": "shelly-plug-s",
		"parts": map[string]any{"fw": map[string]string{"src": "shelly-plug-s.bin", "cs_sha256": hash([]byte(bin))}}})
	if corrupt {
		bin = "tampered"
	}
	return mkzip(t, map[string]string{"shelly-plug-s-1.0/manifest.json": string(m), "shelly-plug-s-1.0/shelly-plug-s.bin": bin})
}

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.14.0", "1.14.0", 0}, {"1.14.1", "1.14.0", 1}, {"1.9.0", "1.10.0", -1}, {"2.0.1", "1.7.5", 1},
		{"1.14.1-rc1", "1.14.1", -1}, {"v1.10.4", "1.10.4", 0}, {"1.14", "1.14.0", 0},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGen1IndexArchiveAndFiles(t *testing.T) {
	good := gen1Zip(t, "20230913-113421/v1.14.0-gcb84623", false)
	archived := gen1Zip(t, "20220101-000000/v1.12.2-gabc", false)
	var srv *httptest.Server
	hits := map[string]int{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		switch {
		case r.URL.Path == "/index":
			json.NewEncoder(w).Encode(map[string]any{"isok": true, "data": map[string]any{
				"SHPLG-S": map[string]string{"url": srv.URL + "/gen1/SHPLG-S.zip", "version": "20230913-113421/v1.14.0-gcb84623"}}})
		case r.URL.Path == "/gen1/SHPLG-S.zip":
			w.Write(good)
		case r.URL.Path == "/archive.php":
			w.Write([]byte(`[{"version":"v1.9.4","file":"SHOLD.zip"},{"version":"v1.12.2","file":"SHOLD.zip"},{"version":"v1.13.0-rc1","file":"SHOLD.zip"},{"version":"v1.11.0-1PMfix","file":"SHOLD.zip"},{"version":"v1.10.0","file":"SHOLD.zip"}]`))
		case r.URL.Path == "/version/v1.12.2/SHOLD.zip":
			w.Write(archived)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	x := New(t.TempDir())
	x.Gen1URL, x.ArchiveURL, x.ArchiveFileURL, x.HTTP = srv.URL+"/index", srv.URL+"/archive.php?type=", srv.URL+"/version/", srv.Client()
	ctx := context.Background()

	l, err := x.Latest(ctx, "1", "SHPLG-S")
	if err != nil || l.Version != "1.14.0" || l.Source != SourceShelly || l.FileName() != "SHPLG-S.zip" {
		t.Fatalf("official %+v %v", l, err)
	}
	p, err := x.File(ctx, l)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, good) {
		t.Fatal("cached file differs")
	}
	x.Latest(ctx, "1", "SHPLG-S")
	x.File(ctx, l)
	if hits["/index"] != 1 || hits["/gen1/SHPLG-S.zip"] != 1 {
		t.Fatalf("index and file are cached: %v", hits)
	}

	a, err := x.Latest(ctx, "1", "SHOLD") // not in the official index
	if err != nil || a.Version != "1.12.2" || a.Source != SourceArchive {
		t.Fatalf("archive %+v %v", a, err)
	}
	if _, err := x.File(ctx, a); err != nil {
		t.Fatal(err)
	}
}

func TestVerify(t *testing.T) {
	l := &Latest{Gen: "1", Key: "SHPLG-S", Version: "1.14.0", Build: "20230913-113421/v1.14.0-gcb84623"}
	if err := Verify(l, gen1Zip(t, l.Build, true)); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt part: %v", err)
	}
	if err := Verify(l, gen1Zip(t, "20220101-000000/v1.12.2-gabc", false)); err == nil {
		t.Fatal("other build accepted")
	}
	if err := Verify(l, []byte("not a zip")); err == nil {
		t.Fatal("garbage accepted")
	}
	g2 := mkzip(t, map[string]string{"manifest.json": `{"name":"Plus1","version":"1.7.5","build_id":"20260311-095850/1.7.5-g9979d16"}`, "Plus1.bin": "x"})
	ok := &Latest{Gen: "2", Key: "Plus1", Version: "1.7.5", sha256: hash(g2)}
	if err := Verify(ok, g2); err != nil {
		t.Fatal(err)
	}
	wrongApp := *ok
	wrongApp.Key = "Pro1"
	if err := Verify(&wrongApp, g2); err == nil {
		t.Fatal("other app accepted")
	}
	wrongHash := *ok
	wrongHash.sha256 = strings.Repeat("0", 64)
	if err := Verify(&wrongHash, g2); err == nil {
		t.Fatal("hash mismatch accepted")
	}
}

func TestGen2IndexOverPinnedTLS(t *testing.T) {
	fw := mkzip(t, map[string]string{"manifest.json": `{"name":"MiniPMG3","version":"2.0.1","build_id":"20260923-075534/2.0.1-ge1a198b"}`})
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/update/MiniPMG3":
			json.NewEncoder(w).Encode(map[string]any{"stable": map[string]string{"version": "2.0.1", "build_id": "20260923-075534/2.0.1-ge1a198b",
				"url": srv.URL + "/gen2-ntest/MiniPMG3/" + hash(fw)},
				"alt": map[string]any{"S1PMG4ZB": map[string]any{"stable": map[string]string{"version": "9.9.9"}}}})
		case "/update/Evil":
			json.NewEncoder(w).Encode(map[string]any{"stable": map[string]string{"version": "1.0", "url": "https://example.com/x/Evil/" + strings.Repeat("a", 64)}})
		case "/gen2-ntest/MiniPMG3/" + hash(fw):
			w.Write(fw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	x := New(filepath.Join(t.TempDir(), "fw"))
	x.Gen2URL, x.Gen2Files = srv.URL+"/update/", srv.URL+"/"
	// The pinned client refuses the test server (no pin for it).
	x.Pinned = PinnedClient(map[string]string{"updates.shelly.cloud": "x"})
	if _, err := x.Latest(context.Background(), "3", "MiniPMG3"); err == nil {
		t.Fatal("unpinned server accepted")
	}
	x = New(filepath.Join(t.TempDir(), "fw"))
	x.Gen2URL, x.Gen2Files, x.Pinned = srv.URL+"/update/", srv.URL+"/", srv.Client()
	l, err := x.Latest(context.Background(), "3", "MiniPMG3")
	if err != nil || l.Version != "2.0.1" || l.FileName() != "MiniPMG3-2.0.1.zip" {
		t.Fatalf("gen2 %+v %v", l, err)
	}
	if _, err := x.File(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	if _, err := x.Latest(context.Background(), "2", "Evil"); err == nil {
		t.Fatal("foreign file URL accepted")
	}
}

func TestEvictKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	x := New(dir)
	x.Keep = 1
	for i, n := range []string{"a", "b"} {
		p := filepath.Join(dir, "gen1", n, "1.0", n+".zip")
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte("x"), 0o600)
		if i == 0 {
			old := mustTime(t)
			os.Chtimes(p, old, old)
		}
	}
	x.evict()
	if _, err := os.Stat(filepath.Join(dir, "gen1", "a", "1.0", "a.zip")); !os.IsNotExist(err) {
		t.Fatal("oldest not evicted")
	}
	if _, err := os.Stat(filepath.Join(dir, "gen1", "b", "1.0", "b.zip")); err != nil {
		t.Fatal("newest evicted")
	}
}

func mustTime(t *testing.T) (tm time.Time) {
	t.Helper()
	return time.Now().Add(-time.Hour)
}

// TestLive runs against Shelly's real servers (SHELLYLANMAN_LIVE=1); CI skips it.
func TestLive(t *testing.T) {
	if os.Getenv("SHELLYLANMAN_LIVE") == "" {
		t.Skip("set SHELLYLANMAN_LIVE=1 to query Shelly's servers")
	}
	x := New(t.TempDir())
	ctx := context.Background()
	for _, c := range [][2]string{{"1", "SHPLG-S"}, {"2", "Plus1"}, {"3", "MiniPMG3"}, {"4", "Mini1PMG4"}} {
		l, err := x.Latest(ctx, c[0], c[1])
		if err != nil {
			t.Errorf("%s: %v", c[1], err)
			continue
		}
		p, err := x.File(ctx, l)
		t.Logf("%s: %s (%s) %s → %s %v", c[1], l.Version, l.Source, l.URL, filepath.Base(p), err)
		if err != nil {
			t.Error(err)
		}
	}
}
