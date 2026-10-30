package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
)

// What happens to the devices themselves is logged too (DECISIONS P26-6), as
// "event device.<what>" lines (actions are "action …"):
//
//   - device.offline (warning) and device.online, with how long it was gone.
//     Only for a device that was on line; a reboot ShellyLanMan sent passes
//     through "reading" and logs nothing. Battery devices sleep: not logged.
//   - device.restart (warning): the uptime went down while ShellyLanMan did
//     not restart it (a power cut, a crash, the watchdog).
//   - device.new: a device not in the archive (only with the archive in use);
//     device.address: its IP address changed.
//   - device.login (warning): the device refuses ShellyLanMan's credentials.
//   - device.firmware: the device (or, in local mode, the index) offers a newer
//     stable firmware, once per version.

// expectRestartFor: how long after a reboot or update a lower uptime is
// ShellyLanMan's own doing (a firmware update is followed for 15 minutes).
const expectRestartFor = 20 * time.Minute

type watched struct {
	status        model.Status
	uptime        int
	ip            string
	update        string    // the firmware version last logged as available
	offlineLogged bool      // device.offline was logged: log device.online when it is back
	offlineSince  time.Time //
}

type watchState struct {
	mu       sync.Mutex
	devs     map[string]*watched
	restarts map[string]time.Time // when ShellyLanMan last restarted the device (reboot, update)
}

// expectRestart notes that ShellyLanMan restarts device id now.
func (m *Devices) expectRestart(id string) {
	w := &m.watching
	w.mu.Lock()
	if w.restarts == nil {
		w.restarts = map[string]time.Time{}
	}
	w.restarts[id] = time.Now()
	w.mu.Unlock()
}

func (m *Devices) event(level slog.Level, name string, d model.Device, attrs ...any) {
	args := []any{"device", d.ID}
	if n := descName(d); n != "" {
		args = append(args, "name", n)
	}
	if d.IP != "" {
		args = append(args, "ip", d.IP)
	}
	m.log.Log(context.Background(), level, "event "+name, append(args, attrs...)...)
}

// watch compares d with what was seen before and logs what happened; it runs
// on every device update (updated).
func (m *Devices) watch(d model.Device) {
	if d.ID == "" || strings.HasPrefix(d.ID, "addr:") { // not identified: no MAC to follow
		return
	}
	w := &m.watching
	w.mu.Lock()
	if w.devs == nil {
		w.devs = map[string]*watched{}
	}
	prev, seen := w.devs[d.ID]
	if !seen {
		prev = &watched{status: d.Status, uptime: -1, ip: d.IP}
		w.devs[d.ID] = prev
	}
	restartedAt, expected := w.restarts[d.ID]
	expected = expected && time.Since(restartedAt) < expectRestartFor
	type line struct {
		level slog.Level
		name  string
		attrs []any
	}
	var lines []line
	add := func(level slog.Level, name string, attrs ...any) { lines = append(lines, line{level, name, attrs}) }

	if !seen {
		if d.Status != model.StatusGhost && m.store.Settings().Archive.Use {
			m.mu.Lock()
			_, archived := m.archive[d.ID]
			m.mu.Unlock()
			if !archived {
				add(slog.LevelInfo, "device.new", "type", d.TypeName, "gen", d.Gen)
			}
		}
		if d.Status == model.StatusLogin {
			add(slog.LevelWarn, "device.login")
		}
	} else if d.Status != prev.status {
		switch {
		case d.Status == model.StatusOffline && prev.status == model.StatusOnline && !d.Battery:
			level, attrs := slog.LevelWarn, []any(nil)
			if expected {
				level, attrs = slog.LevelInfo, []any{"note", "restarting after a reboot or update by ShellyLanMan"}
			}
			add(level, "device.offline", attrs...)
			prev.offlineLogged, prev.offlineSince = true, time.Now()
		case d.Status == model.StatusOnline && prev.offlineLogged:
			add(slog.LevelInfo, "device.online", "after", time.Since(prev.offlineSince).Round(time.Second).String())
			prev.offlineLogged = false
		case d.Status == model.StatusLogin:
			add(slog.LevelWarn, "device.login")
		}
		prev.status = d.Status
	}

	if d.Status == model.StatusOnline && d.Uptime >= 0 {
		if prev.uptime >= 0 && d.Uptime < prev.uptime && !d.Battery {
			if expected {
				delete(w.restarts, d.ID) // its restart came: the next one is not ours
			} else {
				add(slog.LevelWarn, "device.restart", "uptimeBefore", (time.Duration(prev.uptime) * time.Second).String())
			}
		}
		prev.uptime = d.Uptime
	}
	if d.IP != "" && prev.ip != "" && d.IP != prev.ip {
		add(slog.LevelInfo, "device.address", "from", prev.ip)
	}
	if d.IP != "" {
		prev.ip = d.IP
	}
	if d.UpdateVersion != "" && d.UpdateVersion != prev.update && d.Status != model.StatusGhost {
		add(slog.LevelInfo, "device.firmware", "available", d.UpdateVersion)
		prev.update = d.UpdateVersion // one line per version, also when the mark comes and goes
	}
	w.mu.Unlock()

	for _, l := range lines {
		m.event(l.level, l.name, d, l.attrs...)
	}
}
