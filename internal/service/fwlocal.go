package service

// Firmware through ShellyLanMan (DECISIONS §34): for devices without internet
// access, ShellyLanMan compares each device with Shelly's index itself, marks the
// ones with a newer stable firmware, and tells a device to update from a link on
// this server (Gen1 /ota?url=, Gen2+ Shelly.Update {url}). The file is the verified
// copy of the local download (localfw.go), fetched before the device asks for it.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// LocalCheckEvery is how often the index is asked in local mode (DECISIONS P25-4);
// the Firmware page asks too when it opens.
var LocalCheckEvery = 24 * time.Hour

// deviceLinkTTL: a device fetches at once; a deferred update makes a new link when it runs.
const deviceLinkTTL = time.Hour

// ErrNoDeviceAddress: no address devices can fetch from (no port known, no route).
var ErrNoDeviceAddress = errors.New("no address on which devices reach ShellyLanMan: set one in Settings → Firmware")

type fwLocalState struct {
	mu    sync.Mutex
	last  time.Time // last index check of all devices
	kick  chan struct{}
	once  sync.Once
	links map[string]linkClaims // short device links: /fw/<hex>/<file>
}

func (m *Devices) fwLocalKick() chan struct{} {
	m.fwLocal.once.Do(func() { m.fwLocal.kick = make(chan struct{}, 1) })
	return m.fwLocal.kick
}

// FirmwareModeChanged: Settings → Firmware changed; local mode checks the index at once,
// device mode puts back what the devices say at their next refresh.
func (m *Devices) FirmwareModeChanged() {
	m.fwLocal.mu.Lock()
	m.fwLocal.last = time.Time{}
	m.fwLocal.mu.Unlock()
	select {
	case m.fwLocalKick() <- struct{}{}:
	default:
	}
}

func (m *Devices) localMode() bool { return m.store.Settings().Firmware.Local() }

// fwLocalLoop asks the index for every device type in local mode, once a day.
func (m *Devices) fwLocalLoop(ctx context.Context) {
	kick := m.fwLocalKick()
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	first := time.After(30 * time.Second) // let discovery find the devices first
	for {
		select {
		case <-ctx.Done():
			return
		case <-first:
		case <-tick.C:
		case <-kick:
		}
		if !m.localMode() {
			continue
		}
		m.fwLocal.mu.Lock()
		due := time.Since(m.fwLocal.last) >= LocalCheckEvery
		m.fwLocal.mu.Unlock()
		if due {
			m.refreshLocal(ctx)
		}
	}
}

// refreshLocal asks the index for each device type once and marks the devices again.
func (m *Devices) refreshLocal(ctx context.Context) {
	m.fwLocal.mu.Lock()
	m.fwLocal.last = time.Now()
	m.fwLocal.mu.Unlock()
	x := m.FirmwareSource()
	asked := map[string]bool{}
	for _, e := range m.entries() {
		d, info := m.infoOf(e)
		gen, key, _, ok := fwKey(d, info)
		if !ok || asked[gen+"/"+key] {
			continue
		}
		asked[gen+"/"+key] = true
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		if _, err := x.Latest(cctx, gen, key); err != nil {
			m.log.Debug("firmware index", "gen", gen, "key", key, "err", err)
		}
		cancel()
	}
	m.remarkAll()
}

// remarkAll applies the cached index to every device and sends the ones that changed.
func (m *Devices) remarkAll() {
	for _, e := range m.entries() {
		m.mu.Lock()
		before := e.dev
		m.localUpdateMark(e, &e.dev)
		d := e.dev
		m.mu.Unlock()
		if d.UpdateAvailable != before.UpdateAvailable || d.UpdateVersion != before.UpdateVersion {
			m.emit(EventDeviceUpsert, d)
		}
	}
}

func (m *Devices) entries() []*entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*entry, 0, len(m.devs))
	for _, e := range m.devs {
		out = append(out, e)
	}
	return out
}

// localUpdateMark: in local mode the update mark comes from the index (cached, never
// fetched here), not from the device, which cannot reach Shelly. Callers hold m.mu.
func (m *Devices) localUpdateMark(e *entry, d *model.Device) {
	if !m.localMode() {
		return
	}
	gen, key, cur, ok := fwKey(*d, shellyInfoOf(e))
	if !ok {
		return // BLU and unmanaged devices keep what their gateway or they say
	}
	d.UpdateAvailable, d.UpdateVersion = false, ""
	if l := m.FirmwareSource().Cached(gen, key); l != nil && cur != "" && firmware.Compare(l.Version, cur) > 0 {
		d.UpdateAvailable, d.UpdateVersion = true, l.Version
	}
}

// shellyInfoOf: the /shelly answer with the running firmware from the last
// configuration read (/shelly is read once; an update changes the version). Callers hold m.mu.
func shellyInfoOf(e *entry) shellyInfo {
	info := shellyInfo{e.info.Type, e.info.App, e.info.FW, e.info.FWID, e.info.Ver}
	if len(e.rawConfig) == 0 {
		return info
	}
	cfg := parseJSON(e.rawConfig)
	if e.info.Gen == 0 {
		if fw := cfg.Get("fw").Text(); fw != "" {
			info.FW = fw
		}
	} else if id := cfg.Path("sys", "device", "fw_id"); id.Kind() == ojson.String && id.Text() != "" {
		info.FWID, info.Ver = id.Text(), ""
	}
	return info
}

// localFirmwareInfo replaces the device's own check in local mode: the newest stable
// version in the index, no beta (DECISIONS P25-2).
func (m *Devices) localFirmwareInfo(ctx context.Context, e *entry, fw fwInfo) fwInfo {
	m.mu.Lock()
	d, info := e.dev, shellyInfoOf(e)
	m.mu.Unlock()
	gen, key, cur, ok := fwKey(d, info)
	if !ok {
		return fw
	}
	if fw.current != "" {
		cur = fw.current
	}
	fw.stable, fw.stableBuild, fw.beta, fw.betaBuild, fw.local, fw.newer = "", "", "", "", true, false
	l, err := m.FirmwareSource().Latest(ctx, gen, key)
	if err == nil && cur != "" && firmware.Compare(l.Version, cur) > 0 {
		fw.stable, fw.stableBuild, fw.newer = l.Version, l.Build, true
		if fw.stableBuild == "" {
			fw.stableBuild = l.Version
		}
	}
	return fw
}

// sendLocalUpdate tells a device to update from this server: the file is fetched and
// verified first, so the device does not wait on Shelly's servers.
func (m *Devices) sendLocalUpdate(ctx context.Context, e *entry, conn *shelly.Conn) error {
	m.mu.Lock()
	d, info := e.dev, shellyInfoOf(e)
	m.mu.Unlock()
	gen, key, cur, ok := fwKey(d, info)
	if !ok {
		return ErrNoLocalFW
	}
	x := m.FirmwareSource()
	l, err := x.Latest(ctx, gen, key)
	if err != nil {
		return err
	}
	if cur != "" && firmware.Compare(l.Version, cur) <= 0 {
		return ErrUpToDate
	}
	if _, err := x.File(ctx, l); err != nil {
		return err
	}
	base, err := m.deviceBase(d)
	if err != nil {
		return err
	}
	u := base + "/fw/" + m.deviceToken(linkClaims{gen, key, l.Version, time.Now().Add(deviceLinkTTL).Unix()}) + "/" + url.PathEscape(l.FileName())
	m.log.Info("firmware update from ShellyLanMan", "device", d.ID, "version", l.Version, "url", u)
	if gen == "1" {
		return g1cmd(ctx, conn, "/ota?url="+url.QueryEscape(u))
	}
	return g2call(ctx, conn, "Shelly.Update", map[string]string{"url": u})
}

// deviceBase: the address set in Settings → Firmware, else this server's address on the
// way to the device (the local address the system picks for it) with the web port.
func (m *Devices) deviceBase(d model.Device) (string, error) {
	if b := m.store.Settings().Firmware.DeviceBaseURL; b != "" {
		return strings.TrimRight(b, "/"), nil
	}
	if m.Port == nil || m.Port() == 0 {
		return "", ErrNoDeviceAddress
	}
	c, err := net.Dial("udp", net.JoinHostPort(d.IP, "80")) // no packet is sent
	if err != nil {
		return "", ErrNoDeviceAddress
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP
	if ip.IsLoopback() || ip.IsUnspecified() {
		return "", ErrNoDeviceAddress
	}
	return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(m.Port())), nil
}

// deviceToken: a short link for a device (Gen1 has little room for a URL), kept in memory.
func (m *Devices) deviceToken(c linkClaims) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s := &m.fwLocal
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.links == nil {
		s.links = map[string]linkClaims{}
	}
	now := time.Now().Unix()
	for k, v := range s.links {
		if now > v.Exp {
			delete(s.links, k)
		}
	}
	s.links[tok] = c
	return tok
}

func (m *Devices) deviceClaims(tok string) (linkClaims, bool) {
	s := &m.fwLocal
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.links[tok]
	return c, ok
}
