// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// what the archive keeps per device (model/DevicesStore.store).

package service

import (
	"context"
	"sort"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

func archiveID(a store.ArchivedDevice) string {
	if id := model.NormalizeMAC(a.MAC); id != "" {
		return id
	}
	return "addr:" + a.IP
}

func ghostDevice(a store.ArchivedDevice) model.Device {
	return model.Device{
		ID: archiveID(a), MAC: a.MAC, Gen: a.Gen, TypeID: a.TypeID, TypeName: a.TypeName,
		Hostname: a.Host, Name: a.Name, IP: a.IP, Port: a.Port, SSID: a.SSID,
		Status: model.StatusGhost, Battery: a.Battery, LastSeen: a.Last, Managed: true,
		Note: a.Note, Keyword: a.Keyword,
	}
}

// archiveLoop saves the archive when it changed, and once more at shutdown.
// ShellyScanner writes it when the window closes; a server has no "exit"
// (FEATURE_PARITY A5, agreed).
func (m *Devices) archiveLoop(ctx context.Context) {
	defer close(m.done)
	t := time.NewTicker(archiveSaveEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.saveArchive()
			return
		case <-t.C:
			m.mu.Lock()
			dirty := m.dirty
			m.dirty = false
			m.mu.Unlock()
			if dirty {
				m.saveArchive()
			}
		}
	}
}

// saveArchive keeps, per device, the fresh data of devices that answered and
// the stored data of those that did not (not logged in, unmanaged with an
// error, ghosts), plus notes and keywords (DevicesStore.store).
func (m *Devices) saveArchive() {
	if !m.store.Settings().Archive.Use {
		return
	}
	m.mu.Lock()
	next := map[string]store.ArchivedDevice{}
	for id, e := range m.devs {
		d := e.dev
		old, had := m.archive[id]
		fresh := d.Status != model.StatusGhost && d.Status != model.StatusLogin && d.Managed && d.Error == ""
		switch {
		case fresh:
			a := toArchived(d)
			if had && d.LastSeen < old.Last {
				a.Last = old.Last
			}
			next[id] = a
		case had:
			old.IP, old.Port = d.IP, d.Port
			old.Note, old.Keyword = d.Note, d.Keyword
			next[id] = old
		case d.Status != model.StatusGhost && len(d.ID) == 12: // a real MAC: store what we know
			next[id] = toArchived(d)
		}
	}
	m.archive = next
	list := make([]store.ArchivedDevice, 0, len(next))
	for _, a := range next {
		list = append(list, a)
	}
	m.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].MAC < list[j].MAC })
	if err := m.store.SaveArchive(list); err != nil {
		m.log.Error("save archive", "err", err)
	}
}

func toArchived(d model.Device) store.ArchivedDevice {
	return store.ArchivedDevice{
		TypeID: d.TypeID, TypeName: d.TypeName, Host: d.Hostname, MAC: d.MAC, IP: d.IP, Port: d.Port,
		Name: d.Name, SSID: d.SSID, Last: d.LastSeen, Battery: d.Battery, Gen: d.Gen,
		Note: d.Note, Keyword: d.Keyword,
	}
}

// ClearArchive empties the archive (Settings → Archive) and rescans.
func (m *Devices) ClearArchive() error {
	m.mu.Lock()
	m.archive = map[string]store.ArchivedDevice{}
	m.mu.Unlock()
	if err := m.store.ClearArchive(); err != nil {
		return err
	}
	m.Rescan()
	return nil
}

// Wait blocks until the service has stopped (its context ended) and the
// archive has been saved a last time.
func (m *Devices) Wait() { <-m.done }
