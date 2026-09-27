// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// blu/BTHomeDevice (restoreCheck, restore), blu/BluTRV (restoreCheck,
// restore), blu/modules/ScheduleManagerTRV.restore, SensorsCollection.deleteAll
// and g2/modules/Webhooks (delete, restore with a new cid).

package sbk

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/ojson"
)

func checkBTHome(d *Device, f Files) Check {
	res := Check{}
	info := f[bluFile]
	if !info.Exists() || info.Get("type").Str("?") != d.TypeID {
		res.put(ErrModel, "")
		return res
	}
	if mac := info.Get("mac").Str("?"); mac != d.MAC {
		res.put(PreRestoreHost, "mac: "+mac)
	}
	return res
}

func checkTRV(d *Device, f Files) Check {
	res := Check{}
	remote := f["Shelly.GetRemoteDeviceInfo.json"]
	info := remote.Get("device_info")
	if !remote.Exists() || !info.Exists() || info.Get("app").Text() != d.TypeID {
		res.put(ErrModel, "")
		return res
	}
	if host := info.Get("id").Text(); host != d.Hostname {
		res.put(PreRestoreHost, host)
	}
	return res
}

// deleteHooks: Webhooks.delete — the gateway's hooks of origin.cid.
func deleteHooks(ctx context.Context, d *Device, origin string, cid int) {
	list, err := d.get(ctx, "/rpc/Webhook.List")
	if err != nil {
		return
	}
	for _, h := range list.Get("hooks").Items() {
		if h.Get("cid").Int() == cid && strings.HasPrefix(h.Get("event").Text(), origin+".") {
			d.post(ctx, "Webhook.Delete", ojson.Obj("id", h.Get("id").Int()))
		}
	}
}

// restoreHooks: Webhooks.restore(eventType, storedCid, newCid) — recreate the
// stored hooks of one component on its (possibly new) cid.
func restoreHooks(ctx context.Context, d *Device, stored *ojson.Value, origin string, storedCid, newCid int, e *errs) {
	for _, h := range stored.Get("hooks").Items() {
		if h.Get("cid").Int() != storedCid || !strings.HasPrefix(h.Get("event").Text(), origin+".") {
			continue
		}
		hook := h.Clone()
		hook.Remove("id")
		hook.Set("cid", ojson.Int(newCid))
		if r := d.post(ctx, "Webhook.Create", hook); r != nil {
			s := "Action \"" + h.Get("name").Text() + "\" - error: " + *r
			e.add(&s)
		} else {
			e.add(nil)
		}
	}
}

// restoreBTHome: the device configuration on the gateway, its webhooks, its
// sensors (re-added for this address, with their webhooks) and the groups
// that referred to the old sensor keys.
func restoreBTHome(ctx context.Context, d *Device, f Files, e *errs) {
	curComps, err := d.paged(ctx, "/rpc/Shelly.GetComponents?dynamic_only=true&include=[%22status%22]", "components")
	if err != nil {
		e.msg(ErrUnknown)
		return
	}
	groups := map[string]*ojson.Value{}
	for _, c := range curComps.Get("components").Items() {
		if k := c.Get("key").Text(); strings.HasPrefix(k, "group:") {
			groups[k] = c.Path("status", "value").Clone()
		}
	}
	fileIndex := f[bluFile].Get("index").Text()
	fileComps := f["Shelly.GetComponents.json"].Get("components").Items()
	hooks := f["Webhook.List.json"]
	cur, _ := strconv.Atoi(d.BLUIndex)
	fileAddr := ""
	for _, c := range fileComps {
		if c.Get("key").Text() != "bthomedevice:"+fileIndex {
			continue
		}
		config := c.Get("config").Clone()
		config.Remove("id")
		config.Remove("addr") // the device may be registered under another address
		e.add(d.post(ctx, "BTHomeDevice.SetConfig", ojson.Obj("id", cur, "config", config)))
		fileAddr = c.Path("config", "addr").Text()
		deleteHooks(ctx, d, "bthomedevice", cur)
		fi, _ := strconv.Atoi(fileIndex)
		restoreHooks(ctx, d, hooks, "bthomedevice", fi, cur, e)
		break
	}
	// SensorsCollection.deleteAll: stops at the first error.
	var delErr *string
	for _, id := range d.BLUSensors {
		if r := d.post(ctx, "BTHome.DeleteSensor", ojson.Obj("id", id)); r != nil {
			delErr = r
			break
		}
	}
	e.add(delErr)
	renamed := map[string]string{}
	for _, c := range fileComps {
		key := c.Get("key").Text()
		if !strings.HasPrefix(key, "bthomesensor:") || c.Path("config", "addr").Text() != fileAddr {
			continue
		}
		config := c.Get("config").Clone()
		config.Set("addr", ojson.Str(d.MAC))
		res, err := d.call(ctx, "BTHome.AddSensor", ojson.Obj("config", config))
		if err != nil {
			e.msg(ErrUnknown)
			return
		}
		newKey := res.Get("added").Text()
		oldIdx, _ := strconv.Atoi(strings.TrimPrefix(key, "bthomesensor:"))
		newIdx, _ := strconv.Atoi(strings.TrimPrefix(newKey, "bthomesensor:"))
		restoreHooks(ctx, d, hooks, "bthomesensor", oldIdx, newIdx, e)
		renamed[key] = newKey
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, g := range keys {
		members := groups[g]
		change := false
		for i, m := range members.Items() {
			if n, ok := renamed[m.Text()]; ok {
				members.SetIdx(i, ojson.Str(n))
				change = true
			}
		}
		if change {
			id, _ := strconv.Atoi(strings.TrimPrefix(g, "group:"))
			e.add(d.post(ctx, "Group.Set", ojson.Obj("id", id, "value", members)))
		}
	}
}

// restoreTRV: name, remote Sys.ui / Temperature / TRV configuration through
// BluTrv.Call, the schedule rules and the webhooks.
func restoreTRV(ctx context.Context, d *Device, f Files, e *errs) {
	idx, _ := strconv.Atoi(d.BLUIndex)
	stored := f["Shelly.GetConfig.json"]
	e.add(d.post(ctx, "BluTrv.SetConfig", ojson.Obj("id", idx, "config", ojson.Obj("name", stored.Get("name")))))
	remote := f["Shelly.GetRemoteConfig.json"].Get("config")
	call := func(method string, params *ojson.Value) *string {
		return d.post(ctx, "BluTrv.Call", ojson.Obj("id", idx, "method", method, "params", params))
	}
	e.add(call("Sys.SetConfig", ojson.Obj("id", 0, "config", ojson.Obj("ui", remote.Path("sys", "ui")))))
	temp := remote.Get("temperature:0").Clone()
	temp.Remove("id")
	e.add(call("Temperature.SetConfig", ojson.Obj("id", 0, "config", temp)))
	trv := remote.Get("trv:0").Clone()
	trv.Remove("id")
	e.add(call("TRV.SetConfig", ojson.Obj("id", 0, "config", trv)))
	// ScheduleManagerTRV.restore: remove the existing rules, add the stored ones.
	existing, err := d.get(ctx, "/rpc/BluTrv.Call?id="+d.BLUIndex+"&method=%22TRV.ListScheduleRules%22&params=%7B%22id%22:0%7D")
	if err != nil {
		e.msg(ErrUnknown)
		return
	}
	for _, r := range existing.Get("rules").Items() {
		e.add(call("TRV.RemoveScheduleRule", ojson.Obj("id", 0, "rule_id", r.Get("rule_id"))))
	}
	for _, r := range f["TRV.ListScheduleRules.json"].Get("rules").Items() {
		rule := r.Clone()
		rule.Remove("rule_id")
		e.add(call("TRV.AddScheduleRule", ojson.Obj("id", 0, "rule", rule)))
	}
	storedID := stored.Get("id").Int()
	deleteHooks(ctx, d, "blutrv", idx)
	restoreHooks(ctx, d, f["Webhook.List.json"], "blutrv", storedID, idx, e)
}
