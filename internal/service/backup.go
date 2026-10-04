// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// controller/BackupAction and controller/RestoreAction (single and
// multi-device restore, deferred backup/restore, reboot offer).

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/sbk"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Deferred task types of this phase.
const (
	TaskBackup  = "BACKUP"
	TaskRestore = "RESTORE"
)

// ResultStored: a sleeping battery device was backed up from stored data.
const ResultStored = "stored"

// BackupFile is one stored backup.
type BackupFile struct {
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"` // file name in the device's folder
	Hostname string `json:"hostname"`
	Time     int64  `json:"time"` // unix ms
	Size     int64  `json:"size"`
}

const stampLayout = "20060102-150405"

var safeName = regexp.MustCompile(`^[\w\-.]+\.sbk$`)

func (m *Devices) backupDir(id string) string {
	return filepath.Join(m.store.Dir(), "backups", strings.NewReplacer("/", "_", ":", "_", "\\", "_").Replace(id))
}

// Backups lists the stored backups of a device (all devices when id is empty), newest first.
func (m *Devices) Backups(id string) []BackupFile {
	var dirs []string
	if id != "" {
		dirs = []string{m.backupDir(id)}
	} else if list, err := os.ReadDir(filepath.Join(m.store.Dir(), "backups")); err == nil {
		for _, d := range list {
			if d.IsDir() {
				dirs = append(dirs, filepath.Join(m.store.Dir(), "backups", d.Name()))
			}
		}
	}
	var out []BackupFile
	for _, dir := range dirs {
		files, _ := os.ReadDir(dir)
		for _, f := range files {
			info, err := f.Info()
			if err != nil || f.IsDir() || !safeName.MatchString(f.Name()) {
				continue
			}
			host := strings.TrimSuffix(f.Name(), ".sbk")
			if len(host) > len(stampLayout)+1 { // <hostname>-<yyyymmdd-hhmmss>.sbk
				host = host[:len(host)-len(stampLayout)-1]
			}
			out = append(out, BackupFile{DeviceID: filepath.Base(dir), Name: f.Name(), Hostname: host,
				Time: info.ModTime().UnixMilli(), Size: info.Size()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out
}

// BackupData returns a stored backup file.
func (m *Devices) BackupData(id, name string) ([]byte, error) {
	if !safeName.MatchString(name) {
		return nil, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(m.backupDir(id), name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

// saveBackup writes data as <hostname>-<time>.sbk and applies the retention.
func (m *Devices) saveBackup(d model.Device, data []byte) (string, error) {
	dir := m.backupDir(d.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := strings.TrimSuffix(sbk.FileName(d.Hostname), ".sbk") + "-" + time.Now().Format(stampLayout) + ".sbk"
	tmp := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	if keep := m.store.Settings().BackupKeep; keep > 0 {
		list := m.Backups(d.ID)
		for _, old := range list[min(keep, len(list)):] {
			_ = os.Remove(filepath.Join(dir, old.Name))
		}
	}
	return name, nil
}

// sbkDevice describes a device for the backup engine.
func (m *Devices) sbkDevice(e *entry) *sbk.Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := e.dev
	sd := &sbk.Device{Conn: e.conn, Gen: d.Gen, TypeID: d.TypeID, App: e.info.App, Model: e.info.Model, Variant: e.info.Svc0Type,
		Hostname: d.Hostname, MAC: model.NormalizeMAC(d.MAC), SSID: d.SSID, Battery: d.Battery, Pro: model.Lookup(e.info).Pro}
	if e.info.MAC != "" {
		sd.MAC = model.NormalizeMAC(e.info.MAC)
	}
	if e.blu != nil {
		sd.Conn, sd.BLUIndex, sd.MAC = e.blu.gw, e.blu.index, d.MAC
		var objs struct {
			Objects []struct {
				Component string `json:"component"`
			} `json:"objects"`
		}
		_ = json.Unmarshal(e.blu.known, &objs)
		for _, o := range objs.Objects {
			var n int
			if _, err := fmt.Sscanf(o.Component, "bthomesensor:%d", &n); err == nil {
				sd.BLUSensors = append(sd.BLUSensors, n)
			}
		}
	}
	if d.Battery {
		sd.Stored = map[string][]byte{"/rpc/Shelly.GetConfig": e.rawConfig, "/rpc/Shelly.GetDeviceInfo": e.rawShelly}
		for k, v := range e.stored {
			sd.Stored[k] = v
		}
		if sd.Stored["/rpc/Shelly.GetDeviceInfo"] == nil {
			sd.Stored["/rpc/Shelly.GetDeviceInfo"] = e.stored["/rpc/Shelly.GetDeviceInfo?ident=true"]
		}
		if sd.Stored["/rpc/Webhook.List"] == nil && e.rawHooks != nil {
			sd.Stored["/rpc/Webhook.List"] = e.rawHooks
		}
	}
	return sd
}

// setBusy pauses (or resumes) the refresh of a device.
func (m *Devices) setBusy(e *entry, busy bool) {
	m.mu.Lock()
	e.busy = busy
	m.mu.Unlock()
	if !busy {
		m.poke(e)
	}
}

// entryFor returns a device's entry.
func (m *Devices) entryFor(id string) (*entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.devs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return e, nil
}

func devRef(d model.Device) DeviceRef { return ref(cfgTarget{d: d}) }

func waitingStatus(s model.Status) bool {
	return s == model.StatusOffline || s == model.StatusLogin || s == model.StatusGhost
}

// Backup backs up the given devices (BackupAction): one result line each —
// ok, stored (battery device asleep: stored data used), queued or fail.
func (m *Devices) Backup(ctx context.Context, ids []string) ([]ResultLine, error) {
	var out []ResultLine
	for _, id := range ids {
		e, err := m.entryFor(id)
		if err != nil {
			return nil, err
		}
		line, _ := m.backupOne(ctx, e, true)
		out = append(out, line)
	}
	return out, nil
}

func (m *Devices) backupOne(ctx context.Context, e *entry, mayQueue bool) (ResultLine, error) {
	m.mu.Lock()
	d := e.dev
	m.mu.Unlock()
	ref := devRef(d)
	fail := func(err error) (ResultLine, error) {
		m.mu.Lock()
		st := e.dev.Status
		m.mu.Unlock()
		if mayQueue && (waitingStatus(st) || e.conn == nil) {
			m.defer_(cfgTarget{e: e, d: d}, TaskBackup, struct{}{})
			return ResultLine{DeviceRef: ref, Result: ResultQueued}, nil
		}
		return ResultLine{DeviceRef: ref, Result: ResultFail, Message: msgOf(err)}, err
	}
	if e.conn == nil && e.blu == nil {
		return fail(ErrNoConnection)
	}
	if e.blu != nil && e.blu.relay {
		return fail(ErrRelayed)
	}
	m.setBusy(e, true)
	defer m.setBusy(e, false)
	data, stored, err := sbk.Backup(ctx, m.sbkDevice(e))
	if err != nil {
		if shelly.IsOffline(err) {
			m.setStatus(e, model.StatusOffline)
		}
		return fail(err)
	}
	if _, err := m.saveBackup(d, data); err != nil {
		return ResultLine{DeviceRef: ref, Result: ResultFail, Message: err.Error()}, err
	}
	if stored {
		return ResultLine{DeviceRef: ref, Result: ResultStored}, nil
	}
	return ResultLine{DeviceRef: ref, Result: ResultOK}, nil
}

// ---- restore -----------------------------------------------------------------------

// RestoreSource names the backup to restore: a stored file (of any device)
// or uploaded .sbk data (base64).
type RestoreSource struct {
	DeviceID string `json:"deviceId,omitempty"`
	File     string `json:"file,omitempty"`
	Upload   string `json:"upload,omitempty"`
}

func (m *Devices) sourceData(src RestoreSource) ([]byte, error) {
	if src.Upload != "" {
		b, err := base64.StdEncoding.DecodeString(src.Upload)
		if err != nil {
			return nil, fmt.Errorf("%w: upload is not base64", ErrInvalid)
		}
		return b, nil
	}
	return m.BackupData(src.DeviceID, src.File)
}

// RestorePlan is what the restore dialog asks and warns about.
type RestorePlan struct {
	Items []sbk.Item `json:"items"`
	Queue bool       `json:"queue"` // the device is not reachable: the restore will be queued
}

// RestoreCheck reads a backup and runs the checks of the original
// (restoreCheck).
func (m *Devices) RestoreCheck(ctx context.Context, id string, src RestoreSource) (*RestorePlan, error) {
	e, err := m.entryFor(id)
	if err != nil {
		return nil, err
	}
	data, err := m.sourceData(src)
	if err != nil {
		return nil, err
	}
	files, err := sbk.Read(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	items, queue, err := m.check(ctx, e, files)
	if err != nil {
		return nil, err
	}
	return &RestorePlan{Items: items, Queue: queue}, nil
}

// stored: an archived device (GhostDevice) — checked from the file alone,
// its restore is queued.
func stored(e *entry) bool {
	return e.dev.Status == model.StatusGhost || e.conn == nil && e.blu == nil
}

// check runs restoreCheck; an unreachable device is an error, as in the
// original (only the restore itself is queued).
func (m *Devices) check(ctx context.Context, e *entry, files sbk.Files) ([]sbk.Item, bool, error) {
	m.mu.Lock()
	ghost, relay := stored(e), e.blu != nil && e.blu.relay
	m.mu.Unlock()
	if relay {
		return nil, false, ErrRelayed
	}
	sd := m.sbkDevice(e)
	if ghost {
		return sbk.CheckStored(sd, files).Items(), true, nil
	}
	items := sbk.CheckRestore(ctx, sd, files).Items()
	if sd.Offline() {
		m.setStatus(e, model.StatusOffline)
		return nil, false, ErrNoConnection
	}
	return items, false, nil
}

// RestoreResult is the outcome of a restore.
type RestoreResult struct {
	Result   string   `json:"result"` // ok, queued, fail
	Problems []string `json:"problems,omitempty"`
	Reboot   bool     `json:"reboot,omitempty"` // settings need a reboot (the original offers one)
}

type restoreParams struct {
	Data    []byte      `json:"data"`
	Answers sbk.Answers `json:"answers"`
}

// Restore applies a backup with the user's answers (RestoreAction.restoreDevice).
func (m *Devices) Restore(ctx context.Context, id string, src RestoreSource, answers sbk.Answers) (RestoreResult, error) {
	e, err := m.entryFor(id)
	if err != nil {
		return RestoreResult{}, err
	}
	data, err := m.sourceData(src)
	if err != nil {
		return RestoreResult{}, err
	}
	return m.restoreData(ctx, e, data, answers, true)
}

func (m *Devices) restoreData(ctx context.Context, e *entry, data []byte, answers sbk.Answers, mayQueue bool) (RestoreResult, error) {
	files, err := sbk.Read(data)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	m.mu.Lock()
	d := e.dev
	m.mu.Unlock()
	queue := func() (RestoreResult, error) {
		m.defer_(cfgTarget{e: e, d: d}, TaskRestore, restoreParams{Data: data, Answers: answers})
		return RestoreResult{Result: ResultQueued}, nil
	}
	m.mu.Lock()
	ghost, relay := stored(e), e.blu != nil && e.blu.relay
	m.mu.Unlock()
	if relay {
		return RestoreResult{}, ErrRelayed
	}
	if ghost {
		if mayQueue {
			return queue()
		}
		return RestoreResult{Result: ResultFail, Problems: []string{"Status-" + string(model.StatusGhost)}}, nil
	}
	m.setBusy(e, true)
	sd := m.sbkDevice(e)
	res := sbk.Restore(ctx, sd, files, answers)
	m.setBusy(e, false)
	problems := sbk.Problems(res)
	if len(problems) == 0 {
		return RestoreResult{Result: ResultOK, Reboot: sd.RestartRequired() || d.RebootRequired}, nil
	}
	if sd.Offline() {
		m.setStatus(e, model.StatusOffline)
	}
	m.mu.Lock()
	st := e.dev.Status
	m.mu.Unlock()
	if mayQueue && (st == model.StatusOffline || st == model.StatusLogin) { // the error came from the device being unreachable
		return queue()
	}
	return RestoreResult{Result: ResultFail, Problems: problems}, nil
}

// RestoreMulti restores each device from its newest stored backup, without
// questions (multi-device restore: no passwords, scripts overwritten and
// enabled like the backup, no reboot offer). A check that would ask a
// "restore host X?" question or reports an error stops that device.
func (m *Devices) RestoreMulti(ctx context.Context, ids []string) ([]ResultLine, error) {
	var out []ResultLine
	for _, id := range ids {
		e, err := m.entryFor(id)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		d := e.dev
		m.mu.Unlock()
		ref := devRef(d)
		list := m.Backups(id)
		if len(list) == 0 {
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: "noBackup"})
			continue
		}
		data, err := m.BackupData(id, list[0].Name)
		if err != nil {
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: err.Error()})
			continue
		}
		files, err := sbk.Read(data)
		if err != nil {
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: err.Error()})
			continue
		}
		items, _, err := m.check(ctx, e, files)
		if err != nil {
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: msgOf(err)})
			continue
		}
		stop := ""
		for _, it := range items {
			if it.Type == "pre" || it.Type == "error" {
				stop = it.Key
				break
			}
		}
		if stop != "" {
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: stop})
			continue
		}
		r, err := m.restoreData(ctx, e, data, sbk.Multi(), true)
		switch {
		case err != nil:
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: err.Error()})
		case r.Result == ResultFail:
			out = append(out, ResultLine{DeviceRef: ref, Result: ResultFail, Message: strings.Join(r.Problems, "; ")})
		default:
			out = append(out, ResultLine{DeviceRef: ref, Result: r.Result})
		}
	}
	return out, nil
}

// runBackupRestoreTask executes a deferred backup or restore.
func (m *Devices) runBackupRestoreTask(ctx context.Context, e *entry, typ, raw string) string {
	switch typ {
	case TaskBackup:
		line, err := m.backupOne(ctx, e, false)
		if err != nil {
			return msgOf(err)
		}
		if line.Result == ResultFail {
			return line.Message
		}
		return ""
	case TaskRestore:
		var p restoreParams
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return err.Error()
		}
		r, err := m.restoreData(ctx, e, p.Data, p.Answers, false)
		if err != nil {
			return err.Error()
		}
		return strings.Join(r.Problems, "\n")
	}
	return "unknown task " + typ
}
