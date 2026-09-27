package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/sbk"
	"github.com/wimmme/shellylanman/internal/sim"
	"github.com/wimmme/shellylanman/internal/store"
)

func TestBackupListAndRetention(t *testing.T) {
	m, _, ctx := newService(t, func(s *store.Settings) { s.BackupKeep = 2 })
	plugS(t, m, ctx, nil, "AABBCC000001")
	for i := 0; i < 3; i++ {
		res, err := m.Backup(ctx, []string{"AABBCC000001"})
		if err != nil || len(res) != 1 || res[0].Result != ResultOK {
			t.Fatalf("backup %d: %+v %v", i, res, err)
		}
		time.Sleep(1100 * time.Millisecond) // file names carry the second
	}
	list := m.Backups("AABBCC000001")
	if len(list) != 2 || !strings.HasPrefix(strings.ToLower(list[0].Name), "shellyplug-s-aabbcc000001-") || !strings.EqualFold(list[0].Hostname, "shellyplug-s-aabbcc000001") || list[0].Time < list[1].Time {
		t.Fatalf("retention keeps the newest 2: %+v", list)
	}
	if all := m.Backups(""); len(all) != 2 {
		t.Fatalf("all backups %+v", all)
	}
	data, err := m.BackupData("AABBCC000001", list[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := sbk.Read(data); err != nil || !f["settings.json"].Exists() || !f["actions.json"].Exists() {
		t.Fatalf("backup content %v", err)
	}
	if _, err := m.BackupData("AABBCC000001", "../secret.key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("path traversal: %v", err)
	}
}

func TestRestoreCheckAndApply(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, nil, nil)
	if _, err := m.Backup(ctx, []string{"AABBCC000002"}); err != nil {
		t.Fatal(err)
	}
	name := m.Backups("AABBCC000002")[0].Name
	src := RestoreSource{DeviceID: "AABBCC000002", File: name}

	plan, err := m.RestoreCheck(ctx, "AABBCC000002", src)
	if err != nil || plan.Queue {
		t.Fatalf("check %+v %v", plan, err)
	}
	for _, it := range plan.Items {
		if it.Type == "error" { // "pre": the fixture's GetDeviceInfo names another host
			t.Fatalf("own backup must not fail the checks: %+v", plan.Items)
		}
	}
	res, err := m.Restore(ctx, "AABBCC000002", src, sbk.Answers{})
	if err != nil || res.Result != ResultOK {
		t.Fatalf("restore %+v %v", res, err)
	}
	if !callPrefix(g2, "RPC Sys.SetConfig ") {
		t.Fatalf("no restore calls: %v", g2.Calls())
	}

	// Uploaded data: the same, but not base64 is refused.
	data, _ := m.BackupData("AABBCC000002", name)
	if _, err := m.RestoreCheck(ctx, "AABBCC000002", RestoreSource{Upload: base64.StdEncoding.EncodeToString(data)}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestoreCheck(ctx, "AABBCC000002", RestoreSource{Upload: "%%%"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad upload: %v", err)
	}
	if _, err := m.RestoreCheck(ctx, "AABBCC000002", RestoreSource{Upload: base64.StdEncoding.EncodeToString([]byte("nope"))}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("not a backup: %v", err)
	}
}

func TestRestoreOtherModelFailsInMulti(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plugS(t, m, ctx, nil, "AABBCC000001")
	plus1(t, m, ctx, nil, nil)
	if _, err := m.Backup(ctx, []string{"AABBCC000001"}); err != nil {
		t.Fatal(err)
	}
	g1 := m.Backups("AABBCC000001")[0].Name
	plan, err := m.RestoreCheck(ctx, "AABBCC000002", RestoreSource{DeviceID: "AABBCC000001", File: g1})
	if err != nil || len(plan.Items) == 0 || plan.Items[0].Key != sbk.ErrModel {
		t.Fatalf("Gen1 backup on a Plus1: %+v %v", plan, err)
	}
	res, err := m.RestoreMulti(ctx, []string{"AABBCC000001", "AABBCC000002"})
	if err != nil || len(res) != 2 || res[0].Result != ResultOK || res[1].Result != ResultFail || res[1].Message != "noBackup" {
		t.Fatalf("multi %+v %v", res, err)
	}
}

func TestBackupAndRestoreQueuedWhenOffline(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, nil, nil)
	m.Backup(ctx, []string{"AABBCC000002"})
	name := m.Backups("AABBCC000002")[0].Name
	m.mu.Lock()
	e := m.devs["AABBCC000002"]
	e.paused = true
	m.mu.Unlock()
	g2.SetDown(true)

	if res, _ := m.Backup(ctx, []string{"AABBCC000002"}); res[0].Result != ResultQueued {
		t.Fatalf("backup queued: %+v", res)
	}
	if _, err := m.RestoreCheck(ctx, "AABBCC000002", RestoreSource{DeviceID: "AABBCC000002", File: name}); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("the check needs the device: %v", err)
	}
	if r, err := m.Restore(ctx, "AABBCC000002", RestoreSource{DeviceID: "AABBCC000002", File: name}, sbk.Answers{}); err != nil || r.Result != ResultQueued {
		t.Fatalf("restore queued: %+v %v", r, err)
	}
	l := m.Deferred()
	if len(l) != 2 || l[0].Description != "backup" || l[1].Description != "restore" {
		t.Fatalf("deferred %+v", l)
	}
	g2.SetDown(false)
	m.setStatus(e, model.StatusOnline)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (m.Deferred()[0].Status != DefSuccess || m.Deferred()[1].Status != DefSuccess) {
		m.setStatus(e, model.StatusReading) // one task per device update
		m.setStatus(e, model.StatusOnline)
		time.Sleep(50 * time.Millisecond)
	}
	if l := m.Deferred(); l[0].Status != DefSuccess || l[1].Status != DefSuccess || len(m.Backups("AABBCC000002")) != 2 || !callPrefix(g2, "RPC Sys.SetConfig ") {
		t.Fatalf("deferred not run: %+v", l)
	}
}

func callPrefix(d *sim.Device, prefix string) bool {
	for _, c := range d.Calls() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
