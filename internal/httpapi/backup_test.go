package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSBK(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"settings.json": `{"device":{"type":"SHPLG-S","hostname":"shellyplug-s-AABBCC000001"}}`, "actions.json": `{"actions":{}}`} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestBackupEndpoints(t *testing.T) {
	srv, _, st := newDeviceServerStore(t)
	dir := filepath.Join(st.Dir(), "backups", "AABBCC000001")
	os.MkdirAll(dir, 0o700)
	data := testSBK(t)
	os.WriteFile(filepath.Join(dir, "shellyplug-s-AABBCC000001-20260101-120000.sbk"), data, 0o600)

	// The archived device is a ghost: its backup is queued.
	r := do(t, "POST", srv.URL+"/api/v1/backup", `{"ids":["AABBCC000001"]}`, jsonHdr)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `"result":"queued"`) {
		t.Fatalf("backup: %d %s", r.StatusCode, b)
	}
	var list []map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/backups?id=AABBCC000001", "", nil), &list)
	if len(list) != 1 || list[0]["hostname"] != "shellyplug-s-AABBCC000001" {
		t.Fatalf("list %v", list)
	}
	r = do(t, "GET", srv.URL+"/api/v1/devices/AABBCC000001/backups/shellyplug-s-AABBCC000001-20260101-120000.sbk", "", nil)
	got, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !bytes.Equal(got, data) || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download %d %q", r.StatusCode, r.Header.Get("Content-Disposition"))
	}
	if r := do(t, "GET", srv.URL+"/api/v1/devices/AABBCC000001/backups/..%2Fsecret.key", "", nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("traversal: %d", r.StatusCode)
	}

	src := `{"deviceId":"AABBCC000001","file":"shellyplug-s-AABBCC000001-20260101-120000.sbk"}`
	r = do(t, "POST", srv.URL+"/api/v1/devices/AABBCC000001/restore/check", `{"source":`+src+`}`, jsonHdr)
	b, _ = io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `"queue":true`) || !strings.Contains(string(b), `"items":[]`) {
		t.Fatalf("check: %d %s", r.StatusCode, b)
	}
	up := `{"upload":"` + base64.StdEncoding.EncodeToString([]byte("junk")) + `"}`
	if r := do(t, "POST", srv.URL+"/api/v1/devices/AABBCC000001/restore/check", `{"source":`+up+`}`, jsonHdr); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("junk upload: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/devices/AABBCC000001/restore", `{"source":`+src+`}`, jsonHdr); r.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("restore without confirm: %d", r.StatusCode)
	}
	r = do(t, "POST", srv.URL+"/api/v1/devices/AABBCC000001/restore", `{"source":`+src+`,"answers":{},"confirm":true}`, jsonHdr)
	b, _ = io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `"result":"queued"`) {
		t.Fatalf("restore: %d %s", r.StatusCode, b)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/restore/multi", `{"ids":["AABBCC000001"]}`, jsonHdr); r.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("multi without confirm: %d", r.StatusCode)
	}
	r = do(t, "POST", srv.URL+"/api/v1/restore/multi", `{"ids":["AABBCC000001"],"confirm":true}`, jsonHdr)
	b, _ = io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `"result":"queued"`) {
		t.Fatalf("multi: %d %s", r.StatusCode, b)
	}
}

func TestFirmwareEndpoints(t *testing.T) {
	srv, _ := newDeviceServer(t)
	var rows []map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/firmware", "", nil), &rows)
	if len(rows) != 1 || rows[0]["known"] != false {
		t.Fatalf("rows %v", rows)
	}
	if r := do(t, "GET", srv.URL+"/api/v1/firmware?ids=NOPE", "", nil); r.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown: %d", r.StatusCode)
	}
	body := `{"items":[{"id":"AABBCC000001","stage":"any"}]}`
	if r := do(t, "POST", srv.URL+"/api/v1/firmware/update", body, jsonHdr); r.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("without confirm: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/firmware/update", `{"items":[{"id":"AABBCC000001","stage":"x"}],"confirm":true}`, jsonHdr); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad stage: %d", r.StatusCode)
	}
	r := do(t, "POST", srv.URL+"/api/v1/firmware/update", `{"items":[{"id":"AABBCC000001","stage":"any"}],"confirm":true}`, jsonHdr)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), `"result":"queued"`) {
		t.Fatalf("update: %d %s", r.StatusCode, b)
	}
}

func TestLocalFirmwareEndpoints(t *testing.T) {
	srv, _, st := newDeviceServerStore(t)
	var rows []map[string]any
	decode(t, do(t, "GET", srv.URL+"/api/v1/firmware/index", "", nil), &rows)
	if len(rows) != 0 { // the archived device has no /shelly data
		t.Fatalf("index rows %v", rows)
	}
	if r := do(t, "POST", srv.URL+"/api/v1/firmware/AABBCC000001/local", "", nil); r.StatusCode != http.StatusConflict {
		t.Fatalf("no local firmware: %d", r.StatusCode)
	}
	if r := do(t, "GET", srv.URL+"/fw/bogus/SHPLG-S.zip", "", nil); r.StatusCode != http.StatusGone {
		t.Fatalf("bad token: %d", r.StatusCode)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/settings", `{"phoneBaseURL":"ftp://x"}`, jsonHdr); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad phone address: %d", r.StatusCode)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/settings", `{"phoneBaseURL":"http://192.168.0.12:3082/"}`, jsonHdr); r.StatusCode != 200 {
		t.Fatalf("phone address: %d", r.StatusCode)
	}
	if got := st.Settings().PhoneBaseURL; got != "http://192.168.0.12:3082" {
		t.Fatalf("stored %q", got)
	}
}

func TestNoteEndpoint(t *testing.T) {
	srv, devs := newDeviceServer(t)
	if r := do(t, "PUT", srv.URL+"/api/v1/devices/AABBCC000001/note", `{"note":"n","keyword":"kw"}`, jsonHdr); r.StatusCode != http.StatusNoContent {
		t.Fatalf("note: %d", r.StatusCode)
	}
	if d, _ := devs.Get("AABBCC000001"); d.Note != "n" || d.Keyword != "kw" {
		t.Fatalf("device %+v", d)
	}
	if r := do(t, "PUT", srv.URL+"/api/v1/devices/NOPE/note", `{"note":"n"}`, jsonHdr); r.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown: %d", r.StatusCode)
	}
}
