// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// the discovery flow, replacement rules, follow-up discovery (range extender,
// BLU) and refresh scheduling of model/Devices.java and model/DevicesStore.java.

// Package service is the service layer: everything a user can do, independent
// of HTTP. The web API, and later an MCP server or a Home Assistant
// integration, are thin clients of it (ARCHITECTURE.md §2.3).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wimmme/shellylanman/internal/discovery"
	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/store"
)

// Event types sent to browsers.
const (
	EventDeviceUpsert  = "device.upsert"
	EventDeviceRemoved = "device.removed"
	EventDevicesReset  = "devices.reset"
	EventScanState     = "scan.state"
)

// Timings. ShellyScanner: errorsReconnect after 30 s, ghostsReconnect after 45 s.
var (
	ProbeTimeout     = 5 * time.Second
	PresenceInterval = 60 * time.Second // refresh rate while no browser is connected (DECISIONS Q8)
	errorsRetryAfter = 30 * time.Second
	// errorsRetryEvery: devices that could not be read keep being retried
	// (the original retries once, 30 s after the scan starts — O33, extension agreed 2026-09-28).
	errorsRetryEvery = 2 * time.Minute
	ghostsRetryAfter = 45 * time.Second
	archiveSaveEvery = 5 * time.Second
	// After a rescan, devices that were listed are probed at their last address
	// once discovery had a moment to find them (searchProbeAfter), and those
	// still not found leave "searching" when the IP scan ends, or after
	// searchWindow in the mDNS modes (DECISIONS P12-7).
	searchProbeAfter = 3 * time.Second
	searchWindow     = 30 * time.Second
)

// ScanState is what the UI shows about discovery.
type ScanState struct {
	Mode       string `json:"mode"`
	Scanning   bool   `json:"scanning"`            // IP scan in progress
	MDNSActive bool   `json:"mdnsActive"`          // browser running
	MDNSError  string `json:"mdnsError,omitempty"` // browser could not start
	Instances  int    `json:"mdnsInstances"`       // resolved _http._tcp instances
	StartedAt  int64  `json:"startedAt"`           // unix ms of the last (re)scan
}

// Devices owns the device list.
type Devices struct {
	store  *store.Store
	client *shelly.Client
	emit   func(typ string, data any)
	log    *slog.Logger

	// IPScanPort is the port probed by an IP scan (80; tests change it).
	IPScanPort int

	mu        sync.Mutex
	base      context.Context
	runCancel context.CancelFunc
	run       context.Context
	browser   *discovery.Browser
	devs      map[string]*entry
	inflight  map[string]bool
	archive   map[string]store.ArchivedDevice
	dirty     bool
	viewers   bool
	wake      chan struct{}
	done      chan struct{} // closed when the final archive save is done
	scan      ScanState

	deferred deferredQueue // tasks for devices that were off line (Phase 5)
	fw       fwState       // firmware updates being followed (Phase 7)
	fwOnce   sync.Once

	fwIndex     *firmware.Index // Shelly firmware index for the local download (Phase 7)
	fwIndexOnce sync.Once

	ring        sampleRing // chart samples (Phase 8)
	samplesOnce sync.Once

	scenes sceneStore // named action lists, run on request (Phase 11, MCP)
}

type entry struct {
	dev       model.Device
	conn      *shelly.Conn // nil for ghosts and unmanaged-with-error devices
	info      shelly.Info
	blu       *bluLink
	ext       bool // Gen2+ range extender enabled
	cancel    context.CancelFunc
	now       chan struct{} // request an immediate full refresh
	paused    bool          // refresh paused by the user (logs dialog)
	rebooting bool          // refresh paused after a reboot command
	g1Reboot  bool          // Gen1: a setting that needs a reboot was changed (eco mode)
	busy      bool          // refresh paused while a backup or restore runs

	rawConfig, rawStatus, rawPeriph json.RawMessage            // last answers: table fields and "stored data" of sleeping devices
	rawActions, rawHooks, rawComps  json.RawMessage            // Gen1 /settings/actions, Gen2+ Webhook.List, XT1 components
	rawShelly                       json.RawMessage            // the /shelly answer
	stored                          map[string]json.RawMessage // other answers kept for a sleeping battery device
}

// bluLink connects a BLU device to its gateway.
type bluLink struct {
	gw    *shelly.Conn
	gwID  string
	index string // component index, e.g. "200"
	trv   bool
	keys  string // Shelly.GetComponents keys for BTHome devices
	known []byte // BTHomeDevice.GetKnownObjects
	hooks []byte // the gateway's Webhook.List (BTHome button actions)

	trvTarget *float64 // TRV target just set: kept over the next remote status read
}

// NewDevices creates the service. emit receives events for the browsers.
func NewDevices(st *store.Store, client *shelly.Client, emit func(string, any), log *slog.Logger) *Devices {
	if log == nil {
		log = slog.Default()
	}
	if emit == nil {
		emit = func(string, any) {}
	}
	return &Devices{
		store: st, client: client, emit: emit, log: log, IPScanPort: 80,
		devs: map[string]*entry{}, inflight: map[string]bool{}, archive: map[string]store.ArchivedDevice{},
		wake: make(chan struct{}), done: make(chan struct{}),
	}
}

// Start begins discovery according to the settings and runs until ctx ends.
func (m *Devices) Start(ctx context.Context) {
	m.mu.Lock()
	m.base = ctx
	m.mu.Unlock()
	if arc, err := m.store.LoadArchive(); err != nil {
		m.log.Error("archive", "err", err)
	} else {
		m.mu.Lock()
		for _, a := range arc {
			m.archive[archiveID(a)] = a
		}
		m.mu.Unlock()
	}
	m.loadDeferred()
	go m.archiveLoop(ctx)
	m.Rescan()
}

// Rescan clears the list and discovers again with the current settings
// (ShellyScanner: Rescan; archive ghosts are re-added when the archive is on).
// Unlike ShellyScanner, the devices that were listed stay as "searching" until
// they are found again or the search ends (DECISIONS P12-7).
func (m *Devices) Rescan() {
	st := m.store.Settings()
	m.mu.Lock()
	if m.base == nil {
		m.mu.Unlock()
		return
	}
	if m.runCancel != nil {
		m.runCancel()
	}
	prev := m.devs
	for _, e := range prev {
		if e.cancel != nil {
			e.cancel()
		}
	}
	m.devs = map[string]*entry{}
	m.inflight = map[string]bool{}
	run, cancel := context.WithCancel(m.base)
	m.run, m.runCancel = run, cancel
	m.scan = ScanState{Mode: st.Scan.Mode, StartedAt: time.Now().UnixMilli()}
	if st.Archive.Use {
		for id, a := range m.archive {
			m.devs[id] = &entry{dev: ghostDevice(a)}
		}
	}
	// The devices that were listed stay, as "searching", until they are found
	// again or the search is over (P12-7). Offline mode does not search.
	if st.Scan.Mode != store.ScanOffline {
		for id, e := range prev {
			if e.dev.Status == model.StatusGhost {
				continue
			}
			d := e.dev
			d.Status, d.Error = model.StatusSearching, ""
			m.devs[id] = &entry{dev: d}
		}
	}
	m.mu.Unlock()

	m.emit(EventDevicesReset, m.List())
	m.emitScan()

	switch st.Scan.Mode {
	case store.ScanFull, store.ScanLocal:
		b := &discovery.Browser{Log: m.log}
		if st.Scan.Mode == store.ScanLocal {
			b.Interfaces = []string{st.Scan.Interface}
		}
		m.mu.Lock()
		m.browser = b
		m.scan.MDNSActive = true
		m.mu.Unlock()
		go func() {
			err := b.Run(run, func(in discovery.Instance) {
				m.mu.Lock()
				m.scan.Instances++
				m.mu.Unlock()
				m.emitScan()
				m.handle(run, net.JoinHostPort(in.IP.String(), strconv.Itoa(in.Port)), in.Name, true)
			})
			m.mu.Lock()
			m.scan.MDNSActive = false
			if err != nil && run.Err() == nil {
				m.scan.MDNSError = err.Error()
				m.log.Error("mdns", "err", err)
			}
			m.mu.Unlock()
			m.emitScan()
		}()
	case store.ScanIP:
		endAfter := searchProbeAfter + ProbeTimeout // the last address probes are answered
		m.mu.Lock()
		m.scan.Scanning = true
		m.mu.Unlock()
		m.emitScan()
		go func() {
			discovery.IPScan(run, st.Scan.Ranges, m.IPScanPort, 32, func(ctx context.Context, addr string) {
				m.handle(ctx, addr, hostOf(addr), false)
			})
			m.mu.Lock()
			m.scan.Scanning = false
			m.mu.Unlock()
			m.emitScan()
			m.after(run, endAfter, m.endSearch)
		}()
	case store.ScanOffline:
		// archive only
	}

	if st.Scan.Mode != store.ScanOffline {
		go m.after(run, searchProbeAfter, m.probeSearching)
	}
	if st.Scan.Mode == store.ScanFull || st.Scan.Mode == store.ScanLocal {
		go m.after(run, searchWindow, m.endSearch)
	}
	go m.after(run, errorsRetryAfter, m.retryErrorsLoop)
	// Auto reload applies to the mDNS modes only (PanelStore tooltip; Devices.scannerInit).
	if st.Archive.Use && st.Archive.AutoReload && (st.Scan.Mode == store.ScanFull || st.Scan.Mode == store.ScanLocal) {
		go m.after(run, ghostsRetryAfter, m.retryGhosts)
	}
}

func (m *Devices) after(ctx context.Context, d time.Duration, fn func(context.Context)) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
		fn(ctx)
	}
}

// ScanState returns the discovery state.
func (m *Devices) ScanState() ScanState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scan
}

func (m *Devices) emitScan() { m.emit(EventScanState, m.ScanState()) }

// List returns all devices sorted by IP (the table's default order).
func (m *Devices) List() []model.Device {
	m.mu.Lock()
	out := make([]model.Device, 0, len(m.devs))
	for _, e := range m.devs {
		out = append(out, e.dev)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return lessAddr(out[i], out[j]) })
	return out
}

// Get returns one device.
func (m *Devices) Get(id string) (model.Device, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.devs[id]
	if !ok {
		return model.Device{}, false
	}
	return e.dev, true
}

// SetViewers switches between full-rate and presence polling (DECISIONS Q8).
func (m *Devices) SetViewers(n int) {
	m.mu.Lock()
	active := n > 0
	changed := active != m.viewers
	m.viewers = active
	if changed {
		close(m.wake)
		m.wake = make(chan struct{})
	}
	m.mu.Unlock()
}

// ---- discovery -------------------------------------------------------------

// handle probes addr and creates the device (Devices.create). With force, a
// host named shelly* that does not answer like a Shelly is still listed as an
// unmanaged device with the error, so it is visible.
func (m *Devices) handle(ctx context.Context, addr, hint string, force bool) {
	m.mu.Lock()
	if m.inflight[addr] {
		m.mu.Unlock()
		return
	}
	m.inflight[addr] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.inflight, addr)
		m.mu.Unlock()
	}()

	info, raw, err := m.client.Probe(ctx, addr, ProbeTimeout)
	if err != nil {
		if force && (strings.HasPrefix(hint, "shelly") || strings.HasPrefix(hint, "Shelly")) {
			m.addUnmanaged(addr, hint, err)
		}
		return
	}
	m.create(ctx, addr, info, hint, raw)
}

func (m *Devices) create(ctx context.Context, addr string, info shelly.Info, hint string, rawShelly []byte) {
	mdl := model.Lookup(info)
	ip, port := splitAddr(addr)
	gen1 := info.Gen == 0
	e := &entry{info: info, now: make(chan struct{}, 1), rawShelly: rawShelly}
	e.conn = m.client.Conn(addr, gen1)
	e.dev = model.Device{
		ID: model.NormalizeMAC(info.MAC), MAC: info.MAC, Gen: info.Generation(),
		TypeID: mdl.TypeID, TypeName: mdl.TypeName, Managed: mdl.Known, Battery: mdl.Battery,
		Hostname: hint, Name: info.Name, IP: ip, Port: port, Status: model.StatusReading,
	}
	if !gen1 {
		e.dev.Hostname = info.ID
	}
	if e.dev.ID == "" {
		return
	}
	e.conn.SetCredentials(m.credentialsFor(e.dev.ID))

	m.refreshConfig(ctx, e)
	if e.dev.Status != model.StatusLogin || gen1 {
		m.refreshStatus(ctx, e)
	}
	if !m.upsert(e) {
		return
	}

	// Range extender: devices behind it answer on ip:mport (Devices.create).
	if info.Gen >= 2 && (e.ext || e.dev.Status == model.StatusLogin) {
		go m.extenderClients(ctx, e)
	}
	// BLU: Pro, Gen3 and Gen4 mains-powered devices (not the Gen2 BLU Gateway,
	// exactly as ShellyScanner; FEATURE_PARITY O11).
	if info.Gen >= 2 && !mdl.Battery && (mdl.Pro || info.Gen >= 3) && e.dev.Status == model.StatusOnline {
		go m.discoverBLU(ctx, e)
	}
}

func (m *Devices) extenderClients(ctx context.Context, e *entry) {
	var resp struct {
		APClients []struct {
			MPort int `json:"mport"`
		} `json:"ap_clients"`
	}
	if err := e.conn.GetJSON(ctx, "/rpc/WiFi.ListAPClients", &resp); err != nil {
		return
	}
	for _, c := range resp.APClients {
		if c.MPort > 0 {
			addr := net.JoinHostPort(e.dev.IP, strconv.Itoa(c.MPort))
			go m.handle(ctx, addr, e.dev.Hostname+"-EX:"+strconv.Itoa(c.MPort), false)
		}
	}
}

// addUnmanaged lists a Shelly that could not be identified
// (ShellyGenericUnmanagedImpl: generation "-", type "Generic", status error).
func (m *Devices) addUnmanaged(addr, hint string, cause error) {
	ip, port := splitAddr(addr)
	id := model.MACFromHostname(hint)
	if id == "" {
		id = "addr:" + addr
	}
	e := &entry{dev: model.Device{
		ID: id, MAC: id, Gen: model.GenGeneric, TypeID: "", TypeName: "Generic", Hostname: hint,
		IP: ip, Port: port, Status: model.StatusError, Error: cause.Error(),
	}}
	m.upsert(e)
}

// upsert adds or replaces a device by ID (Devices.newDevice): a recognised
// device is not replaced by an unmanaged one; notes and keyword survive.
// Returns false if e was discarded.
func (m *Devices) upsert(e *entry) bool {
	m.mu.Lock()
	if m.run == nil || m.run.Err() != nil {
		m.mu.Unlock()
		return false
	}
	old, exists := m.devs[e.dev.ID]
	if exists && !e.dev.Managed && old.dev.Managed && old.dev.Status != model.StatusGhost && old.conn != nil {
		m.mu.Unlock()
		return false
	}
	if exists {
		if old.cancel != nil {
			old.cancel()
		}
		e.dev.Note, e.dev.Keyword = old.dev.Note, old.dev.Keyword
	} else if a, ok := m.archive[e.dev.ID]; ok {
		e.dev.Note, e.dev.Keyword = a.Note, a.Keyword
	}
	m.devs[e.dev.ID] = e
	m.dirty = true
	run := m.run
	m.mu.Unlock()

	m.emit(EventDeviceUpsert, e.dev)
	m.updated(e.dev)
	if e.conn != nil {
		ctx, cancel := context.WithCancel(run)
		m.mu.Lock()
		e.cancel = cancel
		m.mu.Unlock()
		go m.poll(ctx, e)
	}
	return true
}

// retryErrorsLoop retries failed devices now and then every errorsRetryEvery.
func (m *Devices) retryErrorsLoop(ctx context.Context) {
	m.retryErrors(ctx)
	t := time.NewTicker(errorsRetryEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.retryErrors(ctx)
		}
	}
}

// retryErrors re-creates unmanaged devices that failed (Devices.errorsReconnect).
func (m *Devices) retryErrors(ctx context.Context) {
	for _, d := range m.List() {
		if !d.Managed && d.Error != "" {
			go m.handle(ctx, d.Address(), d.Hostname, false)
		}
	}
}

// probeSearching asks every device still "searching" at its last address
// (P12-7): a device that answers is listed again at once. Battery devices
// sleep and BLU devices come back with their gateway, so they are not asked.
func (m *Devices) probeSearching(ctx context.Context) {
	for _, d := range m.List() {
		if d.Status == model.StatusSearching && !d.Battery && d.Gen != model.GenBLU && d.Gen != model.GenBTHome {
			go m.handle(ctx, d.Address(), d.Hostname, false)
		}
	}
}

// endSearch ends a rescan's search: a device still "searching" becomes a ghost
// when the archive knows it, else it leaves the list (P12-7).
func (m *Devices) endSearch(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	use := m.store.Settings().Archive.Use
	var upserts []model.Device
	var removed []string
	m.mu.Lock()
	for id, e := range m.devs {
		if e.dev.Status != model.StatusSearching {
			continue
		}
		if a, ok := m.archive[id]; ok && use {
			g := ghostDevice(a)
			g.Note, g.Keyword = e.dev.Note, e.dev.Keyword
			m.devs[id] = &entry{dev: g}
			upserts = append(upserts, g)
		} else {
			delete(m.devs, id)
			removed = append(removed, id)
		}
	}
	m.mu.Unlock()
	for _, d := range upserts {
		m.emit(EventDeviceUpsert, d)
	}
	for _, id := range removed {
		m.emit(EventDeviceRemoved, map[string]string{"id": id})
	}
}

// retryGhosts probes archived devices at their last address
// (Devices.ghostsReconnect): not battery devices, not BLU.
func (m *Devices) retryGhosts(ctx context.Context) {
	for i, d := range m.List() {
		if d.Status == model.StatusGhost && !d.Battery && d.Gen != model.GenBLU && d.Gen != model.GenBTHome {
			addr := d.Address()
			time.AfterFunc(time.Duration(i*4)*time.Millisecond, func() { m.handle(ctx, addr, hostOf(addr), false) })
		}
	}
}

// ---- refresh ---------------------------------------------------------------

func (m *Devices) interval() (time.Duration, <-chan struct{}) {
	st := m.store.Settings()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.viewers {
		return time.Duration(st.Scan.RefreshSeconds) * time.Second, m.wake
	}
	return PresenceInterval, m.wake
}

// poll refreshes one device until ctx ends (Devices.scheduleRefresh): status
// every interval, configuration every ConfigTics-th time.
func (m *Devices) poll(ctx context.Context, e *entry) {
	tics := 0
	first := true
	for {
		iv, wake := m.interval()
		if first {
			iv += time.Duration(rand.IntN(1000)) * time.Millisecond // spread the herd
			first = false
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(iv):
		case <-wake:
			continue
		case <-e.now:
			tics = 1 << 30 // force a configuration refresh
		}
		m.mu.Lock()
		paused := e.paused || e.rebooting || e.busy
		m.mu.Unlock()
		if paused {
			continue
		}
		if tics++; tics >= m.store.Settings().Scan.ConfigTics {
			m.refreshConfig(ctx, e)
			tics = 0
		}
		m.refreshStatus(ctx, e)
	}
}

// Refresh re-reads the given devices now (all when ids is empty).
func (m *Devices) Refresh(ids []string) {
	m.mu.Lock()
	var targets []*entry
	for id, e := range m.devs {
		if len(ids) == 0 || contains(ids, id) {
			targets = append(targets, e)
		}
	}
	run := m.run
	m.mu.Unlock()
	for _, e := range targets {
		m.mu.Lock()
		d := e.dev
		m.mu.Unlock()
		switch {
		case d.Status == model.StatusGhost:
		case e.conn == nil: // unmanaged with error: try to identify again
			go m.handle(run, d.Address(), d.Hostname, false)
		default:
			m.setStatus(e, model.StatusReading)
			select {
			case e.now <- struct{}{}:
			default:
			}
		}
	}
}

// Reload re-creates a device from its address (ShellyScanner: Reload / Login).
// For a BLU device the gateway is reloaded.
func (m *Devices) Reload(id string) bool {
	m.mu.Lock()
	e, ok := m.devs[id]
	run := m.run
	var addr, host string
	if ok {
		addr, host = e.dev.Address(), e.dev.Hostname
		if e.blu != nil {
			if gw, gok := m.devs[e.blu.gwID]; gok {
				addr, host = gw.dev.Address(), gw.dev.Hostname
			}
		}
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	go m.handle(run, addr, host, false)
	return true
}

// Remove deletes a stored (ghost) device from the list and the archive.
func (m *Devices) Remove(id string) error {
	m.mu.Lock()
	e, ok := m.devs[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	if e.dev.Status != model.StatusGhost {
		m.mu.Unlock()
		return errors.New("only stored devices can be removed")
	}
	delete(m.devs, id)
	delete(m.archive, id)
	m.dirty = true
	m.mu.Unlock()
	m.emit(EventDeviceRemoved, map[string]string{"id": id})
	return nil
}

// ErrNotFound: no such device.
var ErrNotFound = errors.New("no such device")

func (m *Devices) setStatus(e *entry, s model.Status) {
	m.mu.Lock()
	changed := e.dev.Status != s
	e.dev.Status = s
	d := e.dev
	m.mu.Unlock()
	if changed {
		m.emit(EventDeviceUpsert, d)
		m.updated(d)
	}
}

// updated runs after a device changed: a device that is on line runs its
// next deferred task (DeferrablesContainer listens to UPDATE events).
func (m *Devices) updated(d model.Device) {
	m.fwUpdated(d)
	m.recordSample(d)
	if d.Status == model.StatusOnline && m.deferredWaiting(d.ID) {
		m.runDeferred(d.ID)
	}
}

// poke asks for an immediate full refresh without changing the status.
func (m *Devices) poke(e *entry) {
	select {
	case e.now <- struct{}{}:
	default:
	}
}

// apply runs fn on the device under the lock and emits if anything changed.
func (m *Devices) apply(e *entry, fn func(d *model.Device)) {
	m.mu.Lock()
	before := e.dev
	fn(&e.dev)
	after := e.dev
	current := m.devs[e.dev.ID] == e
	if current && !reflect.DeepEqual(before, after) {
		m.dirty = true
	}
	m.mu.Unlock()
	if current && !reflect.DeepEqual(before, after) {
		m.emit(EventDeviceUpsert, after)
	}
	if current {
		m.updated(after)
	}
}

func statusOf(err error) (model.Status, string) {
	switch {
	case err == nil:
		return model.StatusOnline, ""
	case errors.Is(err, shelly.ErrUnauthorized):
		return model.StatusLogin, ""
	case shelly.IsOffline(err):
		return model.StatusOffline, ""
	default:
		return model.StatusError, err.Error()
	}
}

func (m *Devices) refreshConfig(ctx context.Context, e *entry) {
	if e.blu != nil {
		m.refreshBLU(ctx, e, true)
		return
	}
	path := "/rpc/Shelly.GetConfig"
	if e.info.Gen == 0 {
		path = "/settings"
	}
	raw, err := e.conn.Get(ctx, path)
	var periph, actions, hooks []byte
	if err == nil && e.info.Gen == 0 && inputActionModels[e.dev.TypeID] {
		actions, _ = e.conn.Get(ctx, "/settings/actions") // Actions.fillSettings
	}
	if err == nil && e.info.Gen != 0 && inputActionModels[e.dev.TypeID] {
		hooks, _ = e.conn.Get(ctx, "/rpc/Webhook.List") // Webhooks.fillSettings
	}
	if err == nil && e.info.Gen != 0 {
		var cfg struct {
			Sys struct {
				Device struct {
					AddonType string `json:"addon_type"`
				} `json:"device"`
			} `json:"sys"`
		}
		_ = json.Unmarshal(raw, &cfg)
		if parse.UsesAddon(e.dev.TypeID, cfg.Sys.Device.AddonType) {
			periph, _ = e.conn.Get(ctx, "/rpc/SensorAddon.GetPeripherals")
		}
	}
	m.apply(e, func(d *model.Device) {
		d.Status, d.Error = statusOf(err)
		if err != nil {
			return
		}
		e.rawConfig, e.rawPeriph, e.rawActions, e.rawHooks = raw, periph, actions, hooks
		m.reparse(e, d)
		d.LastSeen = time.Now().UnixMilli()
	})
}

func (m *Devices) refreshStatus(ctx context.Context, e *entry) {
	if e.blu != nil {
		m.refreshBLU(ctx, e, false)
		return
	}
	path := "/rpc/Shelly.GetStatus"
	if e.info.Gen == 0 {
		path = "/status"
	}
	raw, err := e.conn.Get(ctx, path)
	var comps []byte
	if keys := parse.XT1Keys(e.info.Svc0Type); err == nil && keys != nil {
		kb, _ := json.Marshal(keys)
		comps, _ = e.conn.Get(ctx, "/rpc/Shelly.GetComponents?keys="+url.QueryEscape(string(kb)))
	}
	m.apply(e, func(d *model.Device) {
		d.Status, d.Error = statusOf(err)
		if err != nil {
			return
		}
		e.rawStatus, e.rawComps = raw, comps
		m.reparse(e, d)
		d.LastSeen = time.Now().UnixMilli()
	})
}

// reparse recomputes the table fields from the last configuration and status
// (ShellyScanner: fillSettings + fillStatus). Callers hold m.mu.
func (m *Devices) reparse(e *entry, d *model.Device) {
	if e.info.Gen == 0 {
		var cfg struct {
			Device struct {
				Hostname string `json:"hostname"`
			} `json:"device"`
		}
		if json.Unmarshal(e.rawConfig, &cfg) == nil && cfg.Device.Hostname != "" {
			d.Hostname = cfg.Device.Hostname
		}
		d.ApplyReadings(parse.Gen1(parse.Gen1Input{TypeID: d.TypeID, DeviceName: d.Name, Settings: e.rawConfig, Status: e.rawStatus, Actions: e.rawActions}))
		d.RebootRequired = e.g1Reboot // Gen1 does not report it; the original remembers its own changes
		return
	}
	r := parse.Gen2(parse.Gen2Input{TypeID: d.TypeID, DeviceName: d.Name, Config: e.rawConfig, Status: e.rawStatus, Peripherals: e.rawPeriph,
		Webhooks: e.rawHooks, Variant: e.info.Svc0Type, Components: e.rawComps})
	e.ext = r.RangeExtender
	d.ApplyReadings(r)
}

// Models whose inputs show their configured actions in the Command column:
// Gen1 i3 and Button 1 (Actions), Gen2+ i4 (Webhooks).
var inputActionModels = map[string]bool{"SHIX3-1": true, "SHBTN-2": true, "PlusI4": true, "I4G3": true}

// ---- helpers ----------------------------------------------------------------

func splitAddr(addr string) (string, int) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 80
	}
	port, _ := strconv.Atoi(p)
	return host, port
}

func hostOf(addr string) string {
	h, _ := splitAddr(addr)
	return h
}

// lessAddr orders by IPv4 numerically, then port (InetAddressAndPort.compareTo).
func lessAddr(a, b model.Device) bool {
	ia, ib := net.ParseIP(a.IP).To4(), net.ParseIP(b.IP).To4()
	if ia != nil && ib != nil {
		for i := 0; i < 4; i++ {
			if ia[i] != ib[i] {
				return ia[i] < ib[i]
			}
		}
	} else if a.IP != b.IP {
		return a.IP < b.IP
	}
	if a.Port != b.Port {
		return a.Port < b.Port
	}
	return a.ID < b.ID
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
