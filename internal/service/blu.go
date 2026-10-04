// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// BLU discovery through gateways (model/Devices.create, newBluDevice) and the
// identity of BLU devices (blu/BTHomeDevice, blu/BluTRV, blu/modules/SensorsCollection).

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/shelly"
)

const (
	keyBTHomeDevice = "bthomedevice:"
	keyBTHomeSensor = "bthomesensor:"
	keyBluTRV       = "blutrv:"
)

type component struct {
	Key    string          `json:"key"`
	Status json.RawMessage `json:"status"`
	Config json.RawMessage `json:"config"`
	Attrs  struct {
		ModelID int `json:"model_id"`
	} `json:"attrs"`
}

// getComponents reads a paged Shelly.GetComponents (offset/total), like
// ShellyScanner's JsonPageIterator.
func getComponents(ctx context.Context, c *shelly.Conn, query string) ([]component, error) {
	var all []component
	offset := 0
	for {
		var page struct {
			Components []component `json:"components"`
			Offset     int         `json:"offset"`
			Total      int         `json:"total"`
		}
		sep := "?"
		if strings.Contains(query, "?") {
			sep = "&"
		}
		path := "/rpc/Shelly.GetComponents" + query
		if offset > 0 {
			path += sep + "offset=" + strconv.Itoa(offset)
		}
		if err := c.GetJSON(ctx, path, &page); err != nil {
			return all, err
		}
		all = append(all, page.Components...)
		offset = page.Offset + len(page.Components)
		if len(page.Components) == 0 || offset >= page.Total {
			return all, nil
		}
	}
}

// discoverBLU lists the gateway's BLU TRVs and BTHome devices. TRVs first: a
// TRV also has a bthomedevice component, which must not become a second row.
func (m *Devices) discoverBLU(ctx context.Context, gw *entry) {
	comps, err := getComponents(ctx, gw.conn, "?dynamic_only=true")
	if err != nil {
		m.log.Debug("blu discovery", "gateway", gw.dev.Hostname, "err", err)
		return
	}
	trvMACs := map[string]bool{}
	for _, c := range comps {
		if strings.HasPrefix(c.Key, keyBluTRV) {
			if e := m.newTRV(ctx, gw, c); e != nil {
				trvMACs[e.dev.ID] = true
				m.upsertBLU(e, gw)
			}
		}
	}
	for _, c := range comps {
		if strings.HasPrefix(c.Key, keyBTHomeDevice) {
			var cfg struct {
				Addr string `json:"addr"`
			}
			_ = json.Unmarshal(c.Config, &cfg)
			if trvMACs[model.NormalizeMAC(cfg.Addr)] {
				continue
			}
			if e := m.newBTHome(ctx, gw, c); e != nil {
				m.upsertBLU(e, gw)
			}
		}
	}
	m.refreshRelay(ctx, gw) // the devices it only relays (P14-1)
}

func bluBase(gw *entry, mac, index string) *entry {
	return &entry{
		now:  make(chan struct{}, 1),
		conn: gw.conn,
		dev: model.Device{
			ID: model.NormalizeMAC(mac), MAC: mac, IP: gw.dev.IP, Port: gw.dev.Port,
			Parent: gw.dev.ID, Managed: true, Status: model.StatusReading,
		},
		blu: &bluLink{gw: gw.conn, gwID: gw.dev.ID, index: index},
	}
}

// newBTHome builds a BTHome device (BTHomeDevice constructor + init).
func (m *Devices) newBTHome(ctx context.Context, gw *entry, c component) *entry {
	var cfg struct {
		Addr string `json:"addr"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(c.Config, &cfg)
	index := strings.TrimPrefix(c.Key, keyBTHomeDevice)
	e := bluBase(gw, cfg.Addr, index)
	e.dev.Gen = model.GenBTHome
	e.dev.TypeID = fmt.Sprintf("BLU%d", c.Attrs.ModelID)
	e.dev.TypeName = model.BLUTypeName(c.Attrs.ModelID)
	e.dev.Name = cfg.Name

	// Known objects give the sensor components and the "s<obj_id>…" part of
	// ShellyScanner's host name: "B" + sorted obj ids + "-" + address.
	var objs struct {
		Objects []struct {
			ObjID     int    `json:"obj_id"`
			Component string `json:"component"`
		} `json:"objects"`
	}
	keys := []string{c.Key}
	var ids []int
	if raw, err := gw.conn.Get(ctx, "/rpc/BTHomeDevice.GetKnownObjects?id="+index); err == nil && json.Unmarshal(raw, &objs) == nil {
		e.blu.known = raw
		for _, o := range objs.Objects {
			if strings.HasPrefix(o.Component, keyBTHomeSensor) {
				ids = append(ids, o.ObjID)
				keys = append(keys, o.Component)
			}
		}
	}
	sort.Ints(ids)
	var full strings.Builder
	for _, id := range ids {
		full.WriteString("s" + strconv.Itoa(id))
	}
	e.dev.Hostname = "B" + full.String() + "-" + cfg.Addr
	kb, _ := json.Marshal(keys)
	e.blu.keys = url.QueryEscape(string(kb))
	applyBTHomeStatus(&e.dev, c.Status)
	m.refreshBLU(ctx, e, true) // init: sensors, status and button webhooks
	return e
}

// newTRV builds a BLU TRV (BluTRV constructor + init).
func (m *Devices) newTRV(ctx context.Context, gw *entry, c component) *entry {
	index := strings.TrimPrefix(c.Key, keyBluTRV)
	var cfg struct {
		Addr string `json:"addr"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(c.Config, &cfg)
	if cfg.Addr == "" {
		return nil
	}
	e := bluBase(gw, cfg.Addr, index)
	e.blu.trv = true
	e.dev.Gen = model.GenBLU
	e.dev.TypeID = "BluTRV"
	e.dev.TypeName = "Blu TRV"
	e.dev.Name = cfg.Name
	e.dev.Hostname = cfg.Addr
	var info struct {
		DeviceInfo struct {
			ID string `json:"id"`
		} `json:"device_info"`
	}
	if err := gw.conn.GetJSON(ctx, "/rpc/BluTrv.GetRemoteDeviceInfo?id="+index, &info); err == nil && info.DeviceInfo.ID != "" {
		e.dev.Hostname = info.DeviceInfo.ID
	}
	m.refreshBLU(ctx, e, true) // AbstractBTHomeDevice.init: refreshSettings + refreshStatus
	return e
}

// applyBTHomeStatus: a BLU device is offline until its gateway has heard it
// (AbstractBTHomeDevice.getStatus: rssi < 0 → online).
func applyBTHomeStatus(d *model.Device, raw json.RawMessage) {
	var st struct {
		RSSI        int     `json:"rssi"`
		LastUpdated float64 `json:"last_updated_ts"`
	}
	_ = json.Unmarshal(raw, &st)
	if st.RSSI < 0 {
		d.Status = model.StatusOnline
	} else {
		d.Status = model.StatusOffline
	}
	if st.LastUpdated > 0 {
		d.LastSeen = int64(st.LastUpdated * 1000)
	}
}

func (m *Devices) refreshBLU(ctx context.Context, e *entry, config bool) {
	if e.blu.relay { // read with its gateway (refreshRelay); here only end a "reading"
		m.apply(e, func(d *model.Device) {
			if d.Status == model.StatusReading {
				d.Status = model.StatusOffline
				if d.LastSeen > 0 {
					d.Status = model.StatusOnline
				}
			}
		})
		return
	}
	if e.blu.trv {
		// BluTRV.refreshStatus: GetStatus + GetRemoteStatus; refreshSettings: GetConfig + GetRemoteConfig.
		st, err := e.blu.gw.Get(ctx, "/rpc/BluTrv.GetStatus?id="+e.blu.index)
		var remote []byte
		if err == nil {
			remote, _ = e.blu.gw.Get(ctx, "/rpc/BluTrv.GetRemoteStatus?id="+e.blu.index)
		}
		var cfg struct {
			Name string `json:"name"`
		}
		var remoteCfg []byte
		if err == nil && config {
			if err = e.blu.gw.GetJSON(ctx, "/rpc/BluTrv.GetConfig?id="+e.blu.index, &cfg); err == nil {
				remoteCfg, _ = e.blu.gw.Get(ctx, "/rpc/BluTrv.GetRemoteConfig?id="+e.blu.index)
			}
		}
		m.apply(e, func(d *model.Device) {
			if err != nil {
				d.Status, d.Error = statusOf(err)
				return
			}
			d.Error = ""
			if config {
				d.Name = cfg.Name
				e.rawConfig = remoteCfg
			}
			e.rawStatus, e.rawPeriph = st, remote
			applyBTHomeStatus(d, st)
			r := parse.BluTRV(parse.BluTRVInput{Name: d.Name, Status: st, RemoteStatus: remote, RemoteConfig: e.rawConfig})
			if t := e.blu.trvTarget; t != nil && len(r.Modules) == 1 { // BluTRV.tempChanged
				r.Modules[0].Target = t
				e.blu.trvTarget = nil
			}
			d.ApplyReadings(r)
		})
		return
	}
	var hooks []byte
	if config { // BTHomeDevice.refreshSettings: webhooks.fillBTHomesensorSettings
		hooks, _ = e.blu.gw.Get(ctx, "/rpc/Webhook.List")
	}
	comps, err := getComponents(ctx, e.blu.gw, "?keys="+e.blu.keys)
	m.apply(e, func(d *model.Device) {
		if err != nil {
			d.Status, d.Error = statusOf(err)
			return
		}
		d.Error = ""
		found := false
		for _, c := range comps {
			if c.Key == keyBTHomeDevice+e.blu.index {
				found = true
				applyBTHomeStatus(d, c.Status)
				var cfg struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(c.Config, &cfg) == nil {
					d.Name = cfg.Name
				}
			}
		}
		if !found {
			d.Status = model.StatusOffline
		}
		raw, _ := json.Marshal(map[string]any{"components": comps})
		e.rawStatus = raw
		if hooks != nil {
			e.blu.hooks = hooks
		}
		d.ApplyReadings(parse.BTHome(parse.BTHomeInput{Index: e.blu.index, KnownObjects: e.blu.known, Components: raw, Webhooks: e.blu.hooks}))
	})
}

// upsertBLU applies ShellyScanner's rules for a BLU device seen by several
// gateways (Devices.newBluDevice): a TRV replaces a plain BTHome row; an
// existing row from another gateway gets this gateway as an alternative parent.
func (m *Devices) upsertBLU(e *entry, gw *entry) {
	m.mu.Lock()
	old, exists := m.devs[e.dev.ID]
	if exists && old.dev.Status != model.StatusGhost && old.blu != nil {
		// newBluDevice: BTHome → TRV always; otherwise newer or same gateway,
		// but never a TRV back to a plain BTHome row.
		replace := old.blu.relay || (!old.blu.trv && e.blu.trv) ||
			((e.dev.LastSeen > old.dev.LastSeen || old.dev.Parent == e.dev.Parent) && !(old.blu.trv && !e.blu.trv))
		if !replace {
			if !contains(old.dev.Parents, gw.dev.Hostname) {
				old.dev.Parents = append(old.dev.Parents, gw.dev.Hostname)
			}
			d := old.dev
			m.mu.Unlock()
			m.emit(EventDeviceUpsert, d)
			return
		}
		if old.dev.Parent != e.dev.Parent {
			if p, ok := m.devs[old.dev.Parent]; ok && !contains(e.dev.Parents, p.dev.Hostname) {
				e.dev.Parents = append(e.dev.Parents, p.dev.Hostname)
			}
		}
	}
	m.mu.Unlock()
	if e.dev.LastSeen == 0 {
		e.dev.LastSeen = time.Now().UnixMilli()
	}
	m.upsert(e)
}
