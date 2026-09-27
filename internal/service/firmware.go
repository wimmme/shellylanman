// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// model/device/modules/FirmwareManager (getShortVersion), g1/modules/FirmwareManagerG1,
// g2/modules/FirmwareManagerG2, blu/modules/FirmwareManagerTRV and
// view/devsettings/PanelFWUpdate (rows, update, deferral, progress, back on line).

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// TaskFWUpdate is the deferred firmware update.
const TaskFWUpdate = "FW_UPDATE"

// EventFirmware carries a changed firmware row (progress, rebooting, back on line).
const EventFirmware = "firmware.row"

var versionPattern = regexp.MustCompile(`.*/v?([\.\d]+(?:(?:-beta.*)|(?:-rc.*))?)(?:-|@).*?(-beta\d*(-QA)?)?$`)

// ShortVersion is FirmwareManager.getShortVersion:
// "20241031-171026/2.3.0-d508f135-beta4-QA" → "2.3.0-beta4-QA".
func ShortVersion(fw string) string {
	m := versionPattern.FindStringSubmatch(fw)
	if m == nil {
		return fw
	}
	return m[1] + m[2]
}

// FirmwareRow is one line of the FW Update panel.
type FirmwareRow struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"` // extended host name
	Status model.Status `json:"status"`
	Gen    string       `json:"gen"`
	// Known: the device has a firmware manager; false for archived and
	// unmanaged devices, which can only get an "any" update (queued).
	Known        bool   `json:"known"`
	Valid        bool   `json:"valid"` // the check succeeded (else stored data or nothing)
	Current      string `json:"current,omitempty"`
	CurrentBuild string `json:"currentBuild,omitempty"`
	Stable       string `json:"stable,omitempty"`
	StableBuild  string `json:"stableBuild,omitempty"`
	Beta         string `json:"beta,omitempty"`
	BetaBuild    string `json:"betaBuild,omitempty"`
	Preselect    bool   `json:"preselect,omitempty"` // stable ticked when shown (newer build than the current one)
	Updating     bool   `json:"updating,omitempty"`
	Progress     int    `json:"progress"` // download %, -1 unknown
	Rebooting    bool   `json:"rebooting,omitempty"`
	Queued       bool   `json:"queued,omitempty"` // a deferred update is waiting
}

type fwInfo struct {
	current, currentBuild, stable, stableBuild, beta, betaBuild string
	updating, valid                                             bool
}

// fwTrack follows an update until the device is back (PanelFWUpdate.DeviceFirmware).
type fwTrack struct {
	uptime     int // uptime when the update was sent (Gen1: a lower uptime means rebooted)
	status     model.Status
	rebootTime time.Time // ota_success / scheduled_restart seen
	progress   int
	rebooting  bool
	cancel     context.CancelFunc
}

type fwState struct {
	mu     sync.Mutex
	tracks map[string]*fwTrack
}

func (m *Devices) fwTracks() *fwState {
	m.fwOnce.Do(func() { m.fw.tracks = map[string]*fwTrack{} })
	return &m.fw
}

// ---- reading -----------------------------------------------------------------------

func text(v *ojson.Value, def string) string {
	if v.Kind() == ojson.String || v.Kind() == ojson.Number || v.Kind() == ojson.Bool {
		return v.Text()
	}
	return def
}

func parseJSON(b []byte) *ojson.Value {
	if v, err := ojson.Parse(b); err == nil {
		return v
	}
	return ojson.Missed()
}

// checkFirmware runs the firmware manager of one device (FirmwareManagerG1/G2/TRV.init).
func (m *Devices) checkFirmware(ctx context.Context, e *entry) fwInfo {
	m.mu.Lock()
	d, conn, blu := e.dev, e.conn, e.blu
	rawStatus, rawShelly, rawConfig := e.rawStatus, e.rawShelly, e.rawConfig
	storedCheck := e.stored["/rpc/Shelly.CheckForUpdate"]
	m.mu.Unlock()
	var fw fwInfo
	switch {
	case blu != nil: // BLU TRV
		b, err := blu.gw.Get(ctx, "/rpc/BluTrv.GetRemoteDeviceInfo?id="+blu.index)
		if err != nil {
			return fwInfo{}
		}
		info := parseJSON(b).Get("device_info")
		fw.currentBuild = info.Get("fw_id").Text()
		if fw.current = text(info.Get("ver"), ""); !info.Get("ver").NonNull() {
			fw.current = ShortVersion(fw.currentBuild)
		}
		b, err = blu.gw.Get(ctx, "/rpc/BluTrv.CheckForUpdates?id="+blu.index)
		if err != nil {
			return fwInfo{}
		}
		if last := parseJSON(b).Get("fw_id").Text(); last != "" && last != fw.currentBuild {
			fw.stableBuild, fw.stable = last, ShortVersion(last)
		}
		fw.valid = true
	case d.Gen == "1":
		err := func() error {
			if conn == nil {
				return ErrNoConnection
			}
			_ = g1cmd(ctx, conn, "/ota/check")
			b, err := conn.Get(ctx, "/ota")
			if err != nil {
				return err
			}
			ota := parseJSON(b)
			fw = g1Update(ota)
			fw.updating = ota.Get("status").Text() == "updating"
			fw.valid = true
			return nil
		}()
		if err != nil {
			fw = fwInfo{}
			if d.Battery {
				if st := parseJSON(rawStatus); st.Get("update").Exists() {
					fw = g1Update(st.Get("update"))
				} else if sh := parseJSON(rawShelly); sh.Exists() {
					fw.current = sh.Get("fw").Text()
				}
			}
		}
	default: // Gen2+
		err := func() error {
			if conn == nil {
				return ErrNoConnection
			}
			b, err := conn.Get(ctx, "/rpc/Shelly.CheckForUpdate")
			if err != nil {
				return err
			}
			fw = g2Check(parseJSON(b))
			b, err = conn.Get(ctx, "/rpc/Shelly.GetDeviceInfo")
			if err != nil {
				return err
			}
			info := parseJSON(b)
			fw.currentBuild, fw.current = text(info.Get("fw_id"), ""), text(info.Get("ver"), "")
			fw.valid = true
			return nil
		}()
		if err != nil {
			fw = fwInfo{}
			if d.Battery {
				if storedCheck != nil {
					fw = g2Check(parseJSON(storedCheck))
				} else if st := parseJSON(rawStatus); st.Exists() {
					up := st.Path("sys", "available_updates")
					// The original also sets the current version to the stable one here (O27).
					fw.stableBuild = text(up.Path("stable", "version"), "")
					fw.current, fw.stable = fw.stableBuild, fw.stableBuild
					fw.betaBuild = text(up.Path("beta", "version"), "")
					fw.beta = fw.betaBuild
				}
				if sh := parseJSON(rawShelly); sh.Exists() {
					fw.currentBuild, fw.current = text(sh.Get("fw_id"), ""), text(sh.Get("ver"), "")
				} else if cfg := parseJSON(rawConfig); cfg.Exists() {
					fw.currentBuild = text(cfg.Path("sys", "device", "fw_id"), "")
					fw.current = ShortVersion(fw.currentBuild)
				}
			}
		}
	}
	return fw
}

// g1Update reads /ota (or /status "update"): Gen1 reports builds; the panel shows short versions.
func g1Update(ota *ojson.Value) fwInfo {
	var fw fwInfo
	fw.currentBuild = ota.Get("old_version").Text()
	fw.current = ShortVersion(fw.currentBuild)
	if ota.Get("has_update").Kind() == ojson.Bool && ota.Get("has_update").Bool() {
		fw.stableBuild = ota.Get("new_version").Text()
		fw.stable = ShortVersion(fw.stableBuild)
	}
	if b := ota.Get("beta_version"); b.Exists() && b.Text() != fw.currentBuild {
		fw.betaBuild = b.Text()
		fw.beta = ShortVersion(fw.betaBuild)
	}
	return fw
}

func g2Check(n *ojson.Value) fwInfo {
	return fwInfo{
		stableBuild: text(n.Path("stable", "build_id"), ""), stable: text(n.Path("stable", "version"), ""),
		betaBuild: text(n.Path("beta", "build_id"), ""), beta: text(n.Path("beta", "version"), ""),
	}
}

// fwCapable: devices shown in the panel (the settings dialog drops BTHome devices).
func fwCapable(d model.Device) bool { return d.Gen != model.GenBTHome }

// hasManager: getFWManager does not throw (not archived, not unmanaged).
func hasManager(e *entry) bool {
	return e.dev.Status != model.StatusGhost && e.dev.Managed && (e.conn != nil || e.blu != nil)
}

func (m *Devices) firmwareRow(e *entry, fw fwInfo, known, preselect bool) FirmwareRow {
	m.mu.Lock()
	d := e.dev
	m.mu.Unlock()
	row := FirmwareRow{ID: d.ID, Name: extendedHost(d), Status: d.Status, Gen: d.Gen, Known: known, Progress: -1}
	if !known {
		row.Queued = m.fwQueued(d.ID)
		return row
	}
	row.Valid, row.Current, row.CurrentBuild = fw.valid, fw.current, fw.currentBuild
	row.Stable, row.StableBuild, row.Beta, row.BetaBuild = fw.stable, fw.stableBuild, fw.beta, fw.betaBuild
	row.Updating = fw.updating
	// createTableRow: stable ticked when shown and newer than the current build.
	row.Preselect = preselect && fw.stableBuild != "" && (fw.currentBuild == "" || fw.stableBuild > fw.currentBuild)
	t := m.fwTracks()
	t.mu.Lock()
	if tr, ok := t.tracks[d.ID]; ok {
		row.Updating, row.Progress, row.Rebooting = true, tr.progress, tr.rebooting
		row.Preselect = false
	}
	t.mu.Unlock()
	return row
}

func (m *Devices) fwQueued(id string) bool {
	q := &m.deferred
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.tasks {
		if t.DeviceID == id && t.Type == TaskFWUpdate && t.Status == DefWaiting {
			return true
		}
	}
	return false
}

// Firmware checks the given devices (all when ids is empty) in parallel
// (PanelFWUpdate.showing / "Check").
func (m *Devices) Firmware(ctx context.Context, ids []string) ([]FirmwareRow, error) {
	var list []*entry
	m.mu.Lock()
	if len(ids) == 0 {
		for _, e := range m.devs {
			if fwCapable(e.dev) {
				list = append(list, e)
			}
		}
	} else {
		for _, id := range ids {
			e, ok := m.devs[id]
			if !ok {
				m.mu.Unlock()
				return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
			}
			if fwCapable(e.dev) {
				list = append(list, e)
			}
		}
	}
	m.mu.Unlock()
	rows := make([]FirmwareRow, len(list))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 35) // the original's thread pool
	for i, e := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i] = m.firmwareRowOf(ctx, e, true)
		}()
	}
	wg.Wait()
	return rows, nil
}

func (m *Devices) firmwareRowOf(ctx context.Context, e *entry, preselect bool) FirmwareRow {
	m.mu.Lock()
	known := hasManager(e)
	m.mu.Unlock()
	if !known {
		return m.firmwareRow(e, fwInfo{}, false, false)
	}
	return m.firmwareRow(e, m.checkFirmware(ctx, e), true, preselect)
}

// ---- update ------------------------------------------------------------------------

// Firmware update stages.
const (
	StageStable = "stable"
	StageBeta   = "beta"
	StageAny    = "any" // no information: queued, stable when run
)

// FirmwareRequest is one ticked cell.
type FirmwareRequest struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
}

// FirmwareUpdate sends the updates (PanelFWUpdate.apply); a result line per
// device: ok, queued (off line or archived) or fail with the device's message.
func (m *Devices) FirmwareUpdate(ctx context.Context, reqs []FirmwareRequest) ([]ResultLine, error) {
	var out []ResultLine
	for _, r := range reqs {
		if r.Stage != StageStable && r.Stage != StageBeta && r.Stage != StageAny {
			return nil, invalid("stage")
		}
		e, err := m.entryFor(r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, m.updateOne(ctx, e, r.Stage != StageBeta, true))
	}
	return out, nil
}

type fwParams struct {
	Stable bool `json:"stable"`
}

func (m *Devices) updateOne(ctx context.Context, e *entry, stable, mayQueue bool) ResultLine {
	m.mu.Lock()
	d := e.dev
	known := hasManager(e)
	m.mu.Unlock()
	ref := DeviceRef{ID: d.ID, Name: descName(d)}
	queue := func() ResultLine {
		m.defer_(cfgTarget{e: e, d: d}, TaskFWUpdate, fwParams{Stable: stable})
		return ResultLine{DeviceRef: ref, Result: ResultQueued}
	}
	if !known {
		if !mayQueue {
			return ResultLine{DeviceRef: ref, Result: ResultFail, Message: "Status-" + string(d.Status)}
		}
		return queue() // GhostDevice: always queued, to stable
	}
	m.startTrack(e, d)
	err := m.sendUpdate(ctx, e, stable)
	if err != nil {
		m.stopTrack(d.ID)
		m.mu.Lock()
		st := e.dev.Status
		m.mu.Unlock()
		if mayQueue && (st == model.StatusOffline || shelly.IsOffline(err)) {
			m.setStatus(e, model.StatusOffline)
			return queue()
		}
		return ResultLine{DeviceRef: ref, Result: ResultFail, Message: msgOf(err)}
	}
	m.emitFirmware(e)
	return ResultLine{DeviceRef: ref, Result: ResultOK}
}

// sendUpdate: /ota?update=true|beta=true (Gen1), Shelly.Update {stage} (Gen2+),
// BluTrv.UpdateFirmware (blocking: sent in the background, like the original).
func (m *Devices) sendUpdate(ctx context.Context, e *entry, stable bool) error {
	m.mu.Lock()
	d, conn, blu := e.dev, e.conn, e.blu
	m.mu.Unlock()
	switch {
	case blu != nil:
		go func() {
			bctx, cancel := context.WithTimeout(m.baseContext(), 10*time.Minute)
			defer cancel()
			if _, err := blu.gw.Get(bctx, "/rpc/BluTrv.UpdateFirmware?id="+blu.index); err != nil {
				m.log.Debug("BluTrv.UpdateFirmware", "device", d.ID, "err", err)
			}
		}()
		return nil
	case conn == nil:
		return ErrNoConnection
	case d.Gen == "1":
		if stable {
			return g1cmd(ctx, conn, "/ota?update=true")
		}
		return g1cmd(ctx, conn, "/ota?beta=true")
	default:
		stage := StageStable
		if !stable {
			stage = StageBeta
		}
		return g2call(ctx, conn, "Shelly.Update", map[string]string{"stage": stage})
	}
}

func (m *Devices) baseContext() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.run != nil {
		return m.run
	}
	return context.Background()
}

// ---- progress --------------------------------------------------------------------------

func (m *Devices) startTrack(e *entry, d model.Device) {
	t := m.fwTracks()
	ctx, cancel := context.WithTimeout(m.baseContext(), 15*time.Minute)
	t.mu.Lock()
	if old, ok := t.tracks[d.ID]; ok {
		old.cancel()
	}
	t.tracks[d.ID] = &fwTrack{uptime: d.Uptime, status: d.Status, progress: -1, cancel: cancel}
	t.mu.Unlock()
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded { // never came back: stop following it
			m.stopTrack(d.ID)
			m.emitFirmware(e)
		}
	}()
	if d.Gen != "1" {
		go m.followOTA(ctx, e)
	}
}

func (m *Devices) stopTrack(id string) {
	t := m.fwTracks()
	t.mu.Lock()
	if tr, ok := t.tracks[id]; ok {
		tr.cancel()
		delete(t.tracks, id)
	}
	t.mu.Unlock()
}

func (m *Devices) emitFirmware(e *entry) {
	m.emit(EventFirmware, m.firmwareRow(e, fwInfo{}, true, false))
}

// followOTA listens to the device's (BLU: the gateway's) RPC WebSocket for
// NotifyEvent ota_progress / ota_success / scheduled_restart (FMUpdateListener).
func (m *Devices) followOTA(ctx context.Context, e *entry) {
	m.mu.Lock()
	conn, component := e.conn, "sys"
	if e.blu != nil {
		gw := m.devs[e.blu.gwID]
		if gw == nil {
			m.mu.Unlock()
			return
		}
		conn = gw.conn
	}
	blu := e.blu
	id := e.dev.ID
	m.mu.Unlock()
	if conn == nil {
		return
	}
	if blu != nil { // the TRV reports as its BTHome device: "bthomedevice:<n>"
		b, err := blu.gw.Get(ctx, "/rpc/BluTrv.GetConfig?id="+blu.index)
		if err != nil {
			return
		}
		component = parseJSON(b).Get("trv").Text()
	}
	for attempt := 0; attempt < 5 && ctx.Err() == nil; attempt++ {
		if done := m.readOTA(ctx, conn.Addr(), id, component, e); done {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second): // the original reopens after a timeout
		}
	}
}

// readOTA returns true once the device announced its restart.
func (m *Devices) readOTA(ctx context.Context, addr, id, component string, e *entry) bool {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	c, _, err := websocket.Dial(dctx, "ws://"+addr+"/rpc", nil)
	cancel()
	if err != nil {
		return false
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"id":2,"src":"shellylanman","method":"Shelly.GetDeviceInfo"}`)); err != nil {
		return false
	}
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			return false
		}
		var msg struct {
			Method string `json:"method"`
			Params struct {
				Events []struct {
					Component string `json:"component"`
					Event     string `json:"event"`
					Progress  int    `json:"progress_percent"`
				} `json:"events"`
			} `json:"params"`
		}
		if json.Unmarshal(b, &msg) != nil || msg.Method != "NotifyEvent" {
			continue
		}
		for _, ev := range msg.Params.Events {
			if ev.Component != component {
				continue
			}
			switch ev.Event {
			case "ota_progress":
				m.trackSet(id, func(t *fwTrack) { t.progress = ev.Progress })
				m.emitFirmware(e)
			case "ota_success", "scheduled_restart":
				m.trackSet(id, func(t *fwTrack) { t.rebooting, t.rebootTime = true, time.Now() })
				m.emitFirmware(e)
				return true
			}
		}
	}
}

func (m *Devices) trackSet(id string, fn func(*fwTrack)) {
	t := m.fwTracks()
	t.mu.Lock()
	if tr, ok := t.tracks[id]; ok {
		fn(tr)
	}
	t.mu.Unlock()
}

// fwUpdated runs on every device update (PanelFWUpdate.update): a device
// being updated that is back on line — its status changed, it restarted
// more than 3 s ago, or its uptime went down — is checked again.
func (m *Devices) fwUpdated(d model.Device) {
	t := m.fwTracks()
	t.mu.Lock()
	tr, ok := t.tracks[d.ID]
	if !ok {
		t.mu.Unlock()
		return
	}
	changed := d.Status != tr.status ||
		(!tr.rebootTime.IsZero() && time.Since(tr.rebootTime) > 3*time.Second) ||
		(d.Uptime >= 0 && tr.uptime >= 0 && d.Uptime < tr.uptime)
	back := changed && d.Status == model.StatusOnline
	if changed {
		tr.status = d.Status
	}
	if back {
		tr.cancel()
		delete(t.tracks, d.ID)
	}
	t.mu.Unlock()
	if !back {
		return
	}
	e, err := m.entryFor(d.ID)
	if err != nil {
		return
	}
	go func() {
		row := m.firmwareRowOf(m.baseContext(), e, true)
		m.emit(EventFirmware, row)
	}()
}

// runFWTask executes a deferred firmware update.
func (m *Devices) runFWTask(ctx context.Context, e *entry, raw string) string {
	var p fwParams
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return err.Error()
	}
	line := m.updateOne(ctx, e, p.Stable, false)
	return line.Message
}
