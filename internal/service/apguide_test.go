package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/firmware"
)

func TestWiFiQR(t *testing.T) {
	for in, want := range map[string]string{
		"ShellyPlugSG3-54320467CBD4": "WIFI:T:nopass;S:ShellyPlugSG3-54320467CBD4;;",
		`a;b,c:d"e\f`:                `WIFI:T:nopass;S:a\;b\,c\:d\"e\\f;;`,
	} {
		if got := WiFiQR(in); got != want {
			t.Errorf("WiFiQR(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAPGuideByName(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g, err := m.APGuide(ctx, "", "ShellyPlugSG3-54320467CBD4")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Recognised || g.Gen != "2" || g.Key != "PlugSG3" || g.Model != "Plug S G3" || g.MAC != "54320467CBD4" || g.ID != "" {
		t.Fatalf("guide %+v", g)
	}
	if !strings.HasPrefix(g.JoinQR, "data:image/png;base64,") || !strings.HasPrefix(g.PageQR, "data:image/png;base64,") || g.PageURL != "http://192.168.33.1" {
		t.Fatalf("codes: %+v", g)
	}
	// A name that says nothing: not recognised, but the codes are there (the user picks the model).
	g, err = m.APGuide(ctx, "", "MyShellyThing")
	if err != nil || g.Recognised || g.Key != "" || g.JoinQR == "" || g.MAC != "" {
		t.Fatalf("unknown name: %+v %v", g, err)
	}
	// An unknown model whose name still ends in a MAC: the MAC is kept.
	if g, _ = m.APGuide(ctx, "", "Whatever-80646F838136"); g.MAC != "80646F838136" || g.Recognised {
		t.Fatalf("mac only: %+v", g)
	}
	if _, err := m.APGuide(ctx, "", "  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := m.APGuide(ctx, "NOPE", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown device: %v", err)
	}
}

func TestAPGuideOfADevice(t *testing.T) {
	m, _, ctx := newService(t, nil)
	// A Gen1 plug whose access point is switched off, as /settings says.
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "gen1", "SHPLG-S", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	settings["wifi_ap"] = map[string]any{"enabled": false, "ssid": "shellyplug-s-AABBCC000001", "key": ""}
	over, _ := json.Marshal(settings)
	plugS(t, m, ctx, map[string]string{"settings.json": string(over), "shelly.json": `{"type":"SHPLG-S","mac":"AABBCC000001","auth":false,"fw":"20230913-113421/v1.14.0-gcb84623"}`}, "AABBCC000001")

	g, err := m.APGuide(ctx, "AABBCC000001", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.SSID != "shellyplug-s-AABBCC000001" || g.Assumed || g.APEnabled == nil || *g.APEnabled || g.Gen != "1" || g.Key != "SHPLG-S" || g.ID != "AABBCC000001" || !g.Recognised {
		t.Fatalf("guide %+v", g)
	}
	// The MAC alone finds the same device, and a name with that MAC links to it.
	if g2, err := m.APGuide(ctx, "", "aa:bb:cc:00:00:01"); err != nil || g2.ID != "AABBCC000001" || g2.SSID != g.SSID {
		t.Fatalf("by MAC: %+v %v", g2, err)
	}
	if g3, err := m.APGuide(ctx, "", "shellyplug-s-AABBCC000001"); err != nil || g3.ID != "AABBCC000001" || g3.Key != "SHPLG-S" {
		t.Fatalf("by name: %+v %v", g3, err)
	}
}

func TestLocalDownloadModel(t *testing.T) {
	m, st, ctx := newService(t, nil)
	srv := fakeGen1Index(t, "20260101-000000/v1.15.0-gabcdef0")
	x := firmware.New(st.Dir() + "/firmware")
	x.Gen1URL, x.HTTP = srv.URL+"/index", srv.Client()
	x.ArchiveURL = srv.URL + "/none?type="
	m.SetFirmwareSource(x)

	link, err := m.LocalDownloadModel(ctx, "1", "SHPLG-S", "http://192.168.1.2:3082")
	if err != nil {
		t.Fatal(err)
	}
	if link.Version != "1.15.0" || link.Current != "" || link.Model != "PlugS" || link.Name != "PlugS" || !strings.HasSuffix(link.URL, "/SHPLG-S.zip") || link.Warning != "" {
		t.Fatalf("link %+v", link)
	}
	token := strings.Split(strings.TrimPrefix(link.URL, "http://192.168.1.2:3082/fw/"), "/")[0]
	if _, name, err := m.FirmwareFile(ctx, token); err != nil || name != "SHPLG-S.zip" {
		t.Fatalf("file %s %v", name, err)
	}
	for _, bad := range [][2]string{{"3", "PlugSG3"}, {"", "x"}, {"1", " "}} {
		if _, err := m.LocalDownloadModel(ctx, bad[0], bad[1], "http://x"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: %v", bad, err)
		}
	}
	if _, err := m.LocalDownloadModel(ctx, "1", "SHNOPE-1", "http://x"); err == nil {
		t.Error("a model the index does not have gives an error")
	}
}
