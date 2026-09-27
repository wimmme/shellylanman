package service

import (
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
)

func TestShortVersion(t *testing.T) {
	// The examples in FirmwareManager.VERSION_PATTERN's comment.
	for in, want := range map[string]string{
		"20210429-100340/v1.10.4-g3f94cd7":        "1.10.4",
		"20211222-144927/0.9.2-beta2-gc538a83":    "0.9.2-beta2",
		"20231107-162425/v1.14.1-rc1-g0617c15":    "1.14.1-rc1",
		"20211223-144928/v2.0.5@3f0fcbbe":         "2.0.5",
		"20240825-205857/2.2.0-b13b6e07-beta7":    "2.2.0-beta7",
		"20241031-171026/2.3.0-d508f135-beta4-QA": "2.3.0-beta4-QA",
		"1.7.5": "1.7.5",
		"":      "",
	} {
		if got := ShortVersion(in); got != want {
			t.Errorf("ShortVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFirmwareRowsAndUpdate(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	g2 := plus1(t, m, ctx, map[string]string{
		"rpc_Shelly.CheckForUpdate.json": `{"stable":{"version":"1.8.0","build_id":"20260901-000000/1.8.0-gabcdef0"},"beta":{"version":"1.9.0-beta1","build_id":"20261001-000000/1.9.0-beta1-g1234567"}}`,
	}, nil)

	rows, err := m.Firmware(ctx, []string{"AABBCC000001", "AABBCC000002"})
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	r1, r2 := rows[0], rows[1]
	if !r1.Known || !r1.Valid || r1.Current != "1.14.0" || r1.Stable != "" || r1.Beta != "1.14.1-rc1" || r1.Preselect {
		t.Fatalf("gen1 row %+v", r1)
	}
	if !r2.Valid || r2.Current != "1.7.5" || r2.Stable != "1.8.0" || r2.Beta != "1.9.0-beta1" || !r2.Preselect {
		t.Fatalf("gen2 row %+v", r2)
	}
	if !hasCall(g1, "GET /ota/check") {
		t.Fatalf("gen1 asks the device to check: %v", g1.Calls())
	}

	res, err := m.FirmwareUpdate(ctx, []FirmwareRequest{{ID: "AABBCC000001", Stage: StageBeta}, {ID: "AABBCC000002", Stage: StageStable}})
	if err != nil || res[0].Result != ResultOK || res[1].Result != ResultOK {
		t.Fatalf("update %+v %v", res, err)
	}
	if !hasCall(g1, "GET /ota?beta=true") || !hasCall(g2, `RPC Shelly.Update {"stage":"stable"}`) {
		t.Fatalf("update calls %v %v", g1.Calls(), g2.Calls())
	}

	// Progress over the device's RPC WebSocket.
	waitFor(t, "ws client", func() bool { return g2.RPCClients() > 0 })
	g2.Notify(map[string]any{"component": "sys", "event": "ota_progress", "progress_percent": 42})
	track := func(id string, f func(*fwTrack) bool) bool {
		tr := m.fwTracks()
		tr.mu.Lock()
		defer tr.mu.Unlock()
		x, ok := tr.tracks[id]
		return ok && f(x)
	}
	waitFor(t, "progress", func() bool { return track("AABBCC000002", func(x *fwTrack) bool { return x.progress == 42 }) })
	g2.Notify(map[string]any{"component": "sys", "event": "ota_success"})
	waitFor(t, "rebooting", func() bool { return track("AABBCC000002", func(x *fwTrack) bool { return x.rebooting }) })
	e, _ := m.entryFor("AABBCC000002")
	if row := m.firmwareRow(e, fwInfo{}, true, true); !row.Updating || !row.Rebooting || row.Progress != 42 || row.Preselect {
		t.Fatalf("row while rebooting %+v", row)
	}
	// Back on line: no longer followed.
	m.setStatus(e, model.StatusOffline)
	m.setStatus(e, model.StatusOnline)
	waitFor(t, "back on line", func() bool { return !track("AABBCC000002", func(*fwTrack) bool { return true }) })

	if _, err := m.FirmwareUpdate(ctx, []FirmwareRequest{{ID: "AABBCC000002", Stage: "nightly"}}); err == nil {
		t.Fatal("unknown stage accepted")
	}
}

func TestFirmwareUpdateOfArchivedDeviceIsQueued(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plus1(t, m, ctx, nil, nil)
	e, _ := m.entryFor("AABBCC000002")
	m.mu.Lock()
	e.paused = true
	m.mu.Unlock()
	m.setStatus(e, model.StatusGhost)
	rows, _ := m.Firmware(ctx, []string{"AABBCC000002"})
	if rows[0].Known || rows[0].Queued {
		t.Fatalf("archived row %+v", rows[0])
	}
	res, _ := m.FirmwareUpdate(ctx, []FirmwareRequest{{ID: "AABBCC000002", Stage: StageAny}})
	if res[0].Result != ResultQueued {
		t.Fatalf("queued %+v", res)
	}
	if l := m.Deferred(); len(l) != 1 || l[0].Type != TaskFWUpdate || l[0].Description != "fwStable" {
		t.Fatalf("deferred %+v", l)
	}
	if rows, _ := m.Firmware(ctx, []string{"AABBCC000002"}); !rows[0].Queued {
		t.Fatalf("row shows the request %+v", rows[0])
	}
}
