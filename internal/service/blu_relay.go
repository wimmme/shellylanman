package service

// BLU devices a gateway only relays to the Shelly Cloud (BLE.CloudRelay.ListInfos):
// rows of their own, read only (ShellyLanMan's own, DECISIONS P14-1..3; see
// docs/blu-cloud-relay.md and docs/phase-14-blu-relay.md). ShellyScanner lists
// these devices only in its BLE dialog, without readings, name or model.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
)

// ErrRelayed: the device is only relayed by a gateway; there is nothing on it to
// read or write through the gateway.
var ErrRelayed = errors.New("a BLU device the gateway only relays: no settings, backup or control")

// identified: a type id set from the device's own model id ("BLU7"), not an estimate ("BLU").
var identified = regexp.MustCompile(`^BLU\d+$`)

type relayInfo struct {
	SData    map[string]string `json:"sdata"`
	LastSeen int64             `json:"last_seen"` // unix seconds
}

// relayCapable: gateways that relay BLU devices and run BTHome (Gen2 Pro, Gen3,
// Gen4), mains powered — the same gateways discoverBLU reads.
func relayCapable(e *entry) bool {
	return e.blu == nil && e.conn != nil && e.info.Gen >= 2 && !e.dev.Battery && (model.Lookup(e.info).Pro || e.info.Gen >= 3)
}

// refreshRelay reads a gateway's relayed BLU devices and updates their rows; a
// row this gateway heard last and no longer lists goes offline.
func (m *Devices) refreshRelay(ctx context.Context, gw *entry) {
	var listed []map[string]relayInfo
	for offset := 0; ; {
		var page struct {
			Devices []map[string]relayInfo `json:"devices"`
			Total   int                    `json:"total"`
		}
		if err := gw.conn.GetJSON(ctx, "/rpc/BLE.CloudRelay.ListInfos?offset="+strconv.Itoa(offset), &page); err != nil {
			return
		}
		listed = append(listed, page.Devices...)
		offset += len(page.Devices)
		if len(page.Devices) == 0 || offset >= page.Total {
			break
		}
	}
	seen := map[string]bool{}
	for _, item := range listed {
		for mac, info := range item {
			seen[model.NormalizeMAC(mac)] = true
			m.upsertRelay(gw, mac, info)
		}
	}
	m.mu.Lock()
	var gone []model.Device
	for id, e := range m.devs {
		if e.blu != nil && e.blu.relay && e.dev.Parent == gw.dev.ID && !seen[id] && e.dev.Status != model.StatusOffline {
			e.dev.Status = model.StatusOffline
			gone = append(gone, e.dev)
		}
	}
	m.mu.Unlock()
	for _, d := range gone {
		m.emit(EventDeviceUpsert, d)
	}
}

// applyRelay sets a relayed row from the gateway's last advertisement: readings,
// last heard, status (online once heard, DECISIONS P14-2) and — unless the model
// was identified — the model id the device sent (0xF0) or an estimate.
func applyRelay(d *model.Device, info relayInfo) {
	adv, err := parse.DecodeBTHomeBase64(info.SData["fcd2"])
	if err == nil {
		d.ApplyReadings(parse.BTHomeRelayed(adv))
	}
	if ls := info.LastSeen * 1000; ls > d.LastSeen {
		d.LastSeen = ls
	}
	d.Status = model.StatusOffline
	if d.LastSeen > 0 {
		d.Status = model.StatusOnline
	}
	switch {
	case identified.MatchString(d.TypeID):
	case err == nil && adv.ModelID() > 0:
		d.TypeID, d.TypeName = fmt.Sprintf("BLU%d", adv.ModelID()), model.BLUTypeName(adv.ModelID())
	case err == nil && adv.Estimate() != "":
		d.TypeID, d.TypeName = "BLU", adv.Estimate()+" ?"
	case d.TypeName == "":
		d.TypeID, d.TypeName = "BLU", "Blu"
	}
}

// upsertRelay adds or updates the row of one relayed device. A BTHome or TRV row
// for the same device wins; seen by several gateways, the row follows the one
// that heard it last and lists the others as alternative parents.
func (m *Devices) upsertRelay(gw *entry, mac string, info relayInfo) {
	id := model.NormalizeMAC(mac)
	m.mu.Lock()
	old, exists := m.devs[id]
	if exists && old.dev.Status != model.StatusGhost {
		if old.blu == nil || !old.blu.relay { // a BTHome/TRV row (or not a BLU at all)
			m.mu.Unlock()
			return
		}
		if old.dev.Parent != gw.dev.ID {
			if info.LastSeen*1000 <= old.dev.LastSeen {
				changed := !contains(old.dev.Parents, gw.dev.Hostname)
				if changed {
					old.dev.Parents = append(old.dev.Parents, gw.dev.Hostname)
				}
				d := old.dev
				m.mu.Unlock()
				if changed {
					m.emit(EventDeviceUpsert, d)
				}
				return
			}
			if p, ok := m.devs[old.dev.Parent]; ok && !contains(old.dev.Parents, p.dev.Hostname) {
				old.dev.Parents = append(old.dev.Parents, p.dev.Hostname)
			}
			old.dev.Parent, old.blu.gw, old.blu.gwID = gw.dev.ID, gw.conn, gw.dev.ID
			old.dev.IP, old.dev.Port = gw.dev.IP, gw.dev.Port
		}
		applyRelay(&old.dev, info)
		m.dirty = true
		d := old.dev
		m.mu.Unlock()
		m.emit(EventDeviceUpsert, d)
		m.updated(d)
		return
	}
	e := bluBase(gw, mac, "")
	e.blu.relay = true
	e.dev.Gen = model.GenBTHome
	e.dev.Relay = true
	e.dev.Hostname = "B-" + strings.ToLower(mac)
	if a, ok := m.archive[id]; ok { // name and identified model kept in the archive (P14-3)
		e.dev.Name = a.Name
		if identified.MatchString(a.TypeID) {
			e.dev.TypeID, e.dev.TypeName = a.TypeID, a.TypeName
		}
	}
	applyRelay(&e.dev, info)
	m.mu.Unlock()
	if e.dev.LastSeen == 0 {
		e.dev.LastSeen = time.Now().UnixMilli()
	}
	m.upsert(e)
}

// SetBLUName names a relayed BLU device (stored in the archive, P14-3).
func (m *Devices) SetBLUName(id, name string) error {
	e, err := m.entryFor(id)
	if err != nil {
		return err
	}
	if e.blu == nil || !e.blu.relay {
		return errors.New("only relayed BLU devices take a name here; others are named on the device")
	}
	m.apply(e, func(d *model.Device) { d.Name = strings.TrimSpace(name) })
	m.mu.Lock()
	m.dirty = true
	m.mu.Unlock()
	return nil
}

// setBLUModel stores the model a BLU device reported during an identification
// (P14-4) on its row; the archive keeps it.
func (m *Devices) setBLUModel(mac string, modelID int) bool {
	id := model.NormalizeMAC(mac)
	m.mu.Lock()
	e, ok := m.devs[id]
	if !ok || e.blu == nil || modelID <= 0 {
		m.mu.Unlock()
		return false
	}
	e.dev.TypeID, e.dev.TypeName = fmt.Sprintf("BLU%d", modelID), model.BLUTypeName(modelID)
	m.dirty = true
	d := e.dev
	m.mu.Unlock()
	m.emit(EventDeviceUpsert, d)
	return true
}
