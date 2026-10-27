package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

func localMode(s *store.Settings) {
	s.Firmware = store.FirmwareSettings{Mode: store.FirmwareLocal, DeviceBaseURL: "http://10.9.8.7:3082"}
}

func TestLocalModeGen1(t *testing.T) {
	m, st, ctx := newService(t, localMode)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	srv := fakeGen1Index(t, "20260101-000000/v1.15.0-gabcdef0")
	x := firmware.New(st.Dir() + "/firmware")
	x.Gen1URL, x.HTTP, x.ArchiveURL = srv.URL+"/index", srv.Client(), srv.URL+"/none?type="
	m.SetFirmwareSource(x)

	// The index, not the device, marks the device (the device's /status says nothing).
	m.refreshLocal(ctx)
	if d, _ := m.Get("AABBCC000001"); !d.UpdateAvailable || d.UpdateVersion != "1.15.0" {
		t.Fatalf("mark %+v", d)
	}
	rows, err := m.Firmware(ctx, []string{"AABBCC000001"})
	if err != nil || len(rows) != 1 || rows[0].Stable != "1.15.0" || rows[0].Beta != "" || !rows[0].Preselect || rows[0].Current != "1.14.0" {
		t.Fatalf("rows %+v %v", rows, err)
	}

	res, err := m.FirmwareUpdate(ctx, []FirmwareRequest{{ID: "AABBCC000001", Stage: StageStable}})
	if err != nil || res[0].Result != ResultOK {
		t.Fatalf("update %+v %v", res, err)
	}
	var got string
	for _, c := range g1.Calls() {
		if strings.HasPrefix(c, "GET /ota?url=") {
			got = c
		}
	}
	re := regexp.MustCompile(`^http://10\.9\.8\.7:3082/fw/([0-9a-f]{16})/SHPLG-S\.zip$`)
	u, _ := url.QueryUnescape(strings.TrimPrefix(got, "GET /ota?url="))
	mt := re.FindStringSubmatch(u)
	if mt == nil {
		t.Fatalf("update call %q in %v", got, g1.Calls())
	}
	if hasCall(g1, "GET /ota?update=true") {
		t.Fatal("asked the device to fetch from Shelly")
	}
	if _, name, err := m.FirmwareFile(ctx, mt[1]); err != nil || name != "SHPLG-S.zip" {
		t.Fatalf("device link: %s %v", name, err)
	}

	// Back to device mode: the device's own word at the next refresh.
	st.Update(func(s *store.Settings) { s.Firmware.Mode = store.FirmwareDevice })
	e, _ := m.entryFor("AABBCC000001")
	m.mu.Lock()
	m.reparse(e, &e.dev)
	d := e.dev
	m.mu.Unlock()
	if d.UpdateAvailable {
		t.Fatalf("device mode keeps the index mark %+v", d)
	}
}

func TestLocalModeGen2(t *testing.T) {
	m, st, ctx := newService(t, localMode)
	g2 := plus1(t, m, ctx, nil, nil)
	bin := []byte("firmware")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	w.Write([]byte(`{"name":"Plus1","version":"1.8.0","build_id":"20260901-000000/1.8.0-gabcdef0"}`))
	w, _ = zw.Create("p.bin")
	w.Write(bin)
	zw.Close()
	sum := sha256.Sum256(buf.Bytes())
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/update/Plus1":
			json.NewEncoder(w).Encode(map[string]any{"stable": map[string]string{"version": "1.8.0", "build_id": "20260901-000000/1.8.0-gabcdef0",
				"url": srv.URL + "/gen2/Plus1/" + hex.EncodeToString(sum[:])}})
		case "/gen2/Plus1/" + hex.EncodeToString(sum[:]):
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	x := firmware.New(st.Dir() + "/firmware")
	x.Gen2URL, x.Gen2Files, x.Pinned = srv.URL+"/update/", srv.URL+"/", srv.Client()
	m.SetFirmwareSource(x)

	// The device's own check offers a beta: local mode shows the index's stable only.
	rows, err := m.Firmware(ctx, []string{"AABBCC000002"})
	if err != nil || rows[0].Stable != "1.8.0" || rows[0].Beta != "" || !rows[0].Preselect {
		t.Fatalf("rows %+v %v", rows, err)
	}
	res, err := m.FirmwareUpdate(ctx, []FirmwareRequest{{ID: "AABBCC000002", Stage: StageStable}})
	if err != nil || res[0].Result != ResultOK {
		t.Fatalf("update %+v %v", res, err)
	}
	re := regexp.MustCompile(`^RPC Shelly\.Update \{"url":"http://10\.9\.8\.7:3082/fw/[0-9a-f]{16}/Plus1-1\.8\.0\.zip"\}$`)
	found := false
	for _, c := range g2.Calls() {
		found = found || re.MatchString(c)
	}
	if !found {
		t.Fatalf("update calls %v", g2.Calls())
	}
}

func TestDeviceBaseAuto(t *testing.T) {
	m, _, _ := newService(t, nil)
	if _, err := m.deviceBase(model.Device{IP: "127.0.0.1"}); err == nil {
		t.Fatal("no port: no address")
	}
	m.Port = func() int { return 3082 }
	if _, err := m.deviceBase(model.Device{IP: "127.0.0.1"}); err == nil {
		t.Fatal("loopback is not an address for devices")
	}
}
