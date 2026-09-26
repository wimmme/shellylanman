package service

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/sim"
)

func TestReadingsFlowIntoTheDevice(t *testing.T) {
	m, _, ctx := newService(t, nil)
	addr := startSim(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	m.handle(ctx, addr, "shellyplug-s-aabbcc000001", true)
	d := waitDevice(t, m, "AABBCC000001", func(d model.Device) bool { return online(d) && len(d.Meters) > 0 })
	if d.InternalTemp == nil || d.Uptime <= 0 || d.RSSI == 0 || d.LogMode == "" || len(d.Modules) != 1 || d.Modules[0].Kind != "relay" {
		t.Fatalf("Gen1 readings missing: %+v", d)
	}
	g3 := startSim(t, fixtureDir(t, "gen3/MiniPMG3", nil), nil)
	m.handle(ctx, g3, "shellypmminig3-x", true)
	var info struct {
		MAC string `json:"mac"`
	}
	b := readTestdata(t, "gen3/MiniPMG3/shelly.json")
	json.Unmarshal(b, &info)
	dm := waitDevice(t, m, info.MAC, func(d model.Device) bool { return online(d) && len(d.Meters) == 1 })
	if got := len(dm.Meters[0].Values); got != 4 || dm.TypeName != "Shelly Mini PM G3" {
		t.Fatalf("MiniPMG3 meters %+v", dm.Meters)
	}
}

func readTestdata(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + rel)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInfoRequestsAndStoredData(t *testing.T) {
	m, _, ctx := newService(t, nil)
	d, _ := sim.New(fixtureDir(t, "gen1/SHPLG-S", nil))
	srv := httptest.NewServer(d)
	m.handle(ctx, strings.TrimPrefix(srv.URL, "http://"), "shellyplug-s-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", online)

	reqs, err := m.InfoRequests("AABBCC000001")
	if err != nil || len(reqs) != 4 || reqs[1].Name != "settings" || reqs[2].Path != "/settings/actions" {
		t.Fatalf("Gen1 info requests %+v, %v", reqs, err)
	}
	res, err := m.Info(context.Background(), "AABBCC000001", 3)
	if err != nil || res.Stored || !strings.Contains(string(res.Data), `"relays"`) {
		t.Fatalf("live /status: %s %v %v", res.Data, res.Stored, err)
	}
	// The device goes away: /status comes from the stored answer.
	srv.Close()
	res, err = m.Info(context.Background(), "AABBCC000001", 3)
	if err != nil || !res.Stored || !strings.Contains(string(res.Data), `"relays"`) {
		t.Fatalf("stored /status: %v %v", res.Stored, err)
	}
	if _, err := m.Info(context.Background(), "AABBCC000001", 2); err == nil {
		t.Fatal("/settings/actions of an offline device should fail (nothing stored)")
	}
}

func TestGen2InfoRequestsAndPause(t *testing.T) {
	m, _, ctx := newService(t, nil)
	addr := startSim(t, fixtureDir(t, "gen2/Plus1", nil), nil)
	m.handle(ctx, addr, "shellyplus1-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", online)
	reqs, _ := m.InfoRequests("AABBCC000001")
	if len(reqs) != 11 || reqs[0].Name != "Shelly.GetDeviceInfo" {
		t.Fatalf("Gen2 info requests %+v", reqs)
	}
	res, err := m.Info(context.Background(), "AABBCC000001", 1)
	if err != nil || !strings.Contains(string(res.Data), `"sys"`) {
		t.Fatalf("GetConfig: %v", err)
	}
	if err := m.SetPaused("AABBCC000001", true); err != nil {
		t.Fatal(err)
	}
	if d, _ := m.Get("AABBCC000001"); !d.Paused {
		t.Fatal("pause not reflected")
	}
}

func TestLogStreamRelaysLines(t *testing.T) {
	m, _, ctx := newService(t, nil)
	addr := startSim(t, fixtureDir(t, "gen2/Plus1", nil), nil)
	m.handle(ctx, addr, "shellyplus1-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", online)
	lctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got := make(chan string, 1)
	go m.LogStream(lctx, "AABBCC000001", func(b json.RawMessage) {
		select {
		case got <- string(b):
		default:
		}
		cancel()
	})
	select {
	case l := <-got:
		if !strings.Contains(l, `"level":2`) {
			t.Fatalf("log line %s", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no log line relayed")
	}
	if err := m.LogStream(context.Background(), "nope", nil); err != ErrNotFound {
		t.Fatalf("unknown device: %v", err)
	}
}
