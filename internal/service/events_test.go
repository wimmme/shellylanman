package service

import (
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

func TestDeviceEvents(t *testing.T) {
	m, _, _ := newService(t, func(s *store.Settings) { s.Archive.Use = true })
	l := captureLog(m)
	dev := model.Device{ID: "AABBCC0000E1", Name: "Pump", TypeName: "Plug S", Gen: "1", IP: "192.168.0.86", Status: model.StatusOnline, Uptime: 500}
	step := func(fn func(d *model.Device)) {
		fn(&dev)
		m.watch(dev)
	}
	count := func(s string) int { return strings.Count(l.String(), s) }

	m.watch(dev)
	if got := l.find(`msg="event device.new"`); !strings.Contains(got, "device=AABBCC0000E1 name=Pump ip=192.168.0.86 type=\"Plug S\" gen=1") {
		t.Fatalf("new device: %q", got)
	}
	step(func(d *model.Device) { d.Uptime = 560 })
	step(func(d *model.Device) { d.Status = model.StatusOffline })
	if l.find("level=WARN", `msg="event device.offline"`) == "" {
		t.Fatalf("offline:\n%s", l)
	}
	step(func(d *model.Device) { d.Status, d.Uptime = model.StatusOnline, 12 })
	if l.find(`msg="event device.online"`, "after=") == "" || l.find("level=WARN", `msg="event device.restart"`, "uptimeBefore=9m20s") == "" {
		t.Fatalf("back after a power cut:\n%s", l)
	}

	// A reboot by ShellyLanMan: through "reading", a lower uptime is expected.
	m.expectRestart(dev.ID)
	step(func(d *model.Device) { d.Status = model.StatusReading })
	step(func(d *model.Device) { d.Status = model.StatusOffline })
	step(func(d *model.Device) { d.Status, d.Uptime = model.StatusOnline, 3 })
	if count("device.offline") != 1 || count("device.restart") != 1 || count("device.online") != 1 {
		t.Fatalf("own reboot logged as an event:\n%s", l)
	}

	step(func(d *model.Device) { d.IP = "192.168.0.99" })
	if l.find(`msg="event device.address"`, "ip=192.168.0.99", "from=192.168.0.86") == "" {
		t.Fatalf("address:\n%s", l)
	}
	step(func(d *model.Device) { d.UpdateVersion = "1.14.1" })
	step(func(d *model.Device) { d.UpdateVersion = "" })
	step(func(d *model.Device) { d.UpdateVersion = "1.14.1" })
	if count("device.firmware") != 1 || l.find(`msg="event device.firmware"`, "available=1.14.1") == "" {
		t.Fatalf("firmware once per version:\n%s", l)
	}
	step(func(d *model.Device) { d.Status = model.StatusLogin })
	if l.find("level=WARN", `msg="event device.login"`) == "" {
		t.Fatalf("login:\n%s", l)
	}

	// Battery devices sleep: no offline/online, no restart.
	bat := model.Device{ID: "AABBCC0000E2", Battery: true, Status: model.StatusOnline, Uptime: 100}
	m.watch(bat)
	bat.Status = model.StatusOffline
	m.watch(bat)
	bat.Status, bat.Uptime = model.StatusOnline, 1
	m.watch(bat)
	if l.find("AABBCC0000E2", "device.offline") != "" || l.find("AABBCC0000E2", "device.restart") != "" {
		t.Fatalf("battery device:\n%s", l)
	}
}
