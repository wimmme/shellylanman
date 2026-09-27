// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// model/device/RestoreMsg, RestoreAction (answers, error text),
// AbstractG1Device.restoreCheck/restore/restoreCommons and
// AbstractG2Device.restoreCheck/restore/restoreCommonConfig,
// AbstractProDevice.restoreCommonConfig, RestoreUtil.

package sbk

import (
	"context"
	"sort"
	"strings"

	"github.com/wimmme/shellylanman/internal/ojson"
)

// Restore messages (RestoreMsg), in the original's order.
const (
	PreRestoreHost       = "PRE_QUESTION_RESTORE_HOST"
	ErrModel             = "ERR_RESTORE_MODEL"
	ErrConf              = "ERR_RESTORE_CONF"
	ErrMsg               = "ERR_RESTORE_MSG"
	ErrUnknown           = "ERR_UNKNOWN"
	ErrModeCover         = "ERR_RESTORE_MODE_COVER"
	ErrModeTherm         = "ERR_RESTORE_MODE_THERM"
	ErrModeTriphase      = "ERR_RESTORE_MODE_TRIPHASE"
	ErrProfile           = "ERR_RESTORE_PROFILE"
	ErrPowerBase         = "ERR_RESTORE_POWER_BASE"
	WarnAddonCantInstall = "WARN_RESTORE_ADDON_CANT_INSTALL"
	WarnAddonInstall     = "WARN_RESTORE_ADDON_INSTALL"
	WarnAddonEnable      = "WARN_RESTORE_ADDON_ENABLE"
	WarnXmodIO           = "WARN_RESTORE_XMOD_IO"
	WarnBTHome           = "WARN_RESTORE_BTHOME"
	WarnLoRaEnable       = "WARN_RESTORE_LORA_ENABLE"
	AskLogin             = "RESTORE_LOGIN"
	AskWiFi1             = "RESTORE_WI_FI1"
	AskWiFi2             = "RESTORE_WI_FI2"
	AskWiFiAP            = "RESTORE_WI_FI_AP"
	AskMQTT              = "RESTORE_MQTT"
	AskOpenMQTT          = "RESTORE_OPEN_MQTT"
	QScriptsOverride     = "QUESTION_RESTORE_SCRIPTS_OVERRIDE"
	QScriptsEnable       = "QUESTION_RESTORE_SCRIPTS_ENABLE_LIKE_BACKED_UP"
	QScriptsSkip         = "QUESTION_RESTORE_SCRIPTS_SKIP"
)

var msgOrder = []string{PreRestoreHost, ErrModel, ErrConf, ErrMsg, ErrUnknown, ErrModeCover, ErrModeTherm, ErrModeTriphase, ErrProfile,
	ErrPowerBase, WarnAddonCantInstall, WarnAddonInstall, WarnAddonEnable, WarnXmodIO, WarnBTHome, WarnLoRaEnable, AskLogin, AskWiFi1,
	AskWiFi2, AskWiFiAP, AskMQTT, AskOpenMQTT, QScriptsOverride, QScriptsEnable, QScriptsSkip}

// MsgType classifies a message (RestoreMsg.Type).
func MsgType(key string) string {
	switch {
	case strings.HasPrefix(key, "PRE_"):
		return "pre"
	case strings.HasPrefix(key, "ERR_"):
		return "error"
	case strings.HasPrefix(key, "WARN_"):
		return "warn"
	}
	return "ask"
}

// Item is one result of the check.
type Item struct {
	Key   string   `json:"key"`
	Type  string   `json:"type"`
	Value string   `json:"value,omitempty"` // host name, SSID, user, script names
	Args  []string `json:"args,omitempty"`  // ERR_RESTORE_PROFILE: current and backup profile
}

// Check is the outcome of restoreCheck.
type Check map[string]Item

func (c Check) put(key, value string, args ...string) {
	c[key] = Item{Key: key, Type: MsgType(key), Value: value, Args: args}
}

// Items in the original's order.
func (c Check) Items() []Item {
	out := make([]Item, 0, len(c))
	for _, k := range msgOrder {
		if it, ok := c[k]; ok {
			out = append(out, it)
		}
	}
	return out
}

// Answers are the user's replies: RESTORE_LOGIN / WI_FI1 / WI_FI2 / WI_FI_AP /
// MQTT → password; QUESTION_RESTORE_SCRIPTS_* → "true".
type Answers map[string]string

func (a Answers) has(k string) bool { _, ok := a[k]; return ok }

// Multi: the answers of a multi-device restore (no passwords, scripts
// overwritten and enabled like the backup).
func Multi() Answers { return Answers{QScriptsOverride: "true", QScriptsEnable: "true"} }

// errs collects the per-step results (null = success, "->r_step:" markers).
type errs []string

func (e *errs) add(s *string) {
	if s == nil {
		*e = append(*e, "")
	} else {
		*e = append(*e, *s)
	}
}
func (e *errs) msg(s string) { *e = append(*e, s) }

// Problems returns the distinct error texts of a restore (RestoreAction.erroreMsg,
// before translation); empty means success.
func Problems(list []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range list {
		if s == "" || strings.HasPrefix(s, "->r_step:") || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// CheckRestore is restoreCheck.
func CheckRestore(ctx context.Context, d *Device, f Files) Check {
	switch d.Gen {
	case "1":
		return checkG1(ctx, d, f)
	case "bth":
		return checkBTHome(d, f)
	case "blu":
		return checkTRV(d, f)
	}
	return checkG2(ctx, d, f)
}

// Restore applies a backup; it returns the step results (see Problems).
func Restore(ctx context.Context, d *Device, f Files, a Answers) []string {
	d.offline, d.restartRequired = false, false
	var e errs
	switch d.Gen {
	case "1":
		restoreG1(ctx, d, f, a, &e)
	case "bth":
		restoreBTHome(ctx, d, f, &e)
	case "blu":
		restoreTRV(ctx, d, f, &e)
	default:
		restoreG2(ctx, d, f, a, &e)
	}
	return e
}

// ---- Gen1 -----------------------------------------------------------------------

func checkG1(ctx context.Context, d *Device, f Files) Check {
	res := Check{}
	s := f["settings.json"]
	if !s.Exists() || !s.Get("device").Exists() {
		res.put(ErrModel, "")
		return res
	}
	fileHost := s.Path("device", "hostname").Text()
	fileType := s.Path("device", "type").Text()
	if fileType != "" && fileType != d.TypeID {
		res.put(ErrModel, "")
		return res
	}
	sameHost := fileHost == d.Hostname
	if !sameHost {
		res.put(PreRestoreHost, fileHost)
	}
	if s.Path("login", "enabled").Bool() {
		res.put(AskLogin, s.Path("login", "username").Text())
	}
	if cur := d.currentConnection(ctx); cur != NetUnknown {
		if s.Path("wifi_sta", "enabled").Bool() && (sameHost || s.Path("wifi_sta", "ipv4_method").Text() == "dhcp") && cur != NetPrimary {
			res.put(AskWiFi1, s.Path("wifi_sta", "ssid").Text())
		}
		if s.Path("wifi_sta1", "enabled").Bool() && (sameHost || s.Path("wifi_sta1", "ipv4_method").Text() == "dhcp") && cur != NetSecondary {
			res.put(AskWiFi2, s.Path("wifi_sta1", "ssid").Text())
		}
	}
	if s.Path("mqtt", "enable").Bool() && s.Path("mqtt", "user").Text() != "" {
		res.put(AskMQTT, s.Path("mqtt", "user").Text())
	}
	return res
}

func restoreG1(ctx context.Context, d *Device, f Files, a Answers, e *errs) {
	s := f["settings.json"].Clone()
	actions := f["actions.json"]
	if fn := g1Models[d.TypeID]; fn != nil {
		fn(ctx, d, s, e)
	}
	if d.offline {
		if len(*e) == 0 {
			e.msg(ErrUnknown)
		}
		return
	}
	restoreCommonsG1(ctx, d, s, a, e)
	restoreActionsG1(ctx, d, actions, e)
	if roam := s.Get("ap_roaming"); roam.Exists() {
		e.add(d.cmd(ctx, "/settings?ap_roaming_enabled="+boolText(roam.Get("enabled"))+"&ap_roaming_threshold="+roam.Get("threshold").Text()))
	}
	cur := d.currentConnection(ctx)
	if sta1 := s.Get("wifi_sta1"); cur != NetSecondary && sta1.Exists() && (a.has(AskWiFi2) || !sta1.Get("enabled").Bool()) {
		e.add(restoreStaG1(ctx, d, "sta1", sta1, a[AskWiFi2]))
	}
	// last: the connection may drop after this
	if cur != NetPrimary && (a.has(AskWiFi1) || !s.Path("wifi_sta", "enabled").Bool()) {
		e.add(restoreStaG1(ctx, d, "sta", s.Get("wifi_sta"), a[AskWiFi1]))
	}
}

func boolText(v *ojson.Value) string {
	if v.Bool() {
		return "true"
	}
	return "false"
}

// restoreCommonsG1: cloud, the common /settings, login, MQTT (tz_dst and
// debug_enable are intentionally not restored).
func restoreCommonsG1(ctx context.Context, d *Device, s *ojson.Value, a Answers, e *errs) {
	e.add(d.cmd(ctx, "/settings/cloud?enabled="+s.Path("cloud", "enabled").Text()))
	pars := []string{"name", "discoverable", "timezone", "lat", "lng", "tzautodetect", "tz_utc_offset", "tz_dst_auto", "tz_dst_auto", "allow_cross_origin"}
	if s.Get("pon_wifi_reset").Exists() {
		pars = append(pars, "pon_wifi_reset")
	}
	coiot := ""
	if s.Get("coiot").Exists() {
		coiot = "&coiot_enable=" + s.Path("coiot", "enabled").Text() + "&coiot_update_period=" + s.Path("coiot", "update_period").Text() +
			"&coiot_peer=" + q(s.Path("coiot", "peer").Text())
	}
	sntp := ""
	if srv := s.Path("sntp", "server"); srv.Exists() {
		sntp = "&sntp_server=" + srv.Text()
	}
	e.add(d.cmd(ctx, "/settings?"+nodePars(s, pars...)+coiot+sntp))
	if pwd, ok := a[AskLogin]; ok {
		e.add(d.cmd(ctx, "/settings/login?enabled=true&username="+q(s.Path("login", "username").Text())+"&password="+q(pwd)))
	} else if !s.Path("login", "enabled").Bool() {
		e.add(d.cmd(ctx, "/settings/login?enabled=false"))
	}
	mqtt := s.Get("mqtt")
	if a.has(AskMQTT) || !mqtt.Get("enable").Bool() || mqtt.Get("user").Text() == "" {
		pwd, ok := a[AskMQTT]
		e.add(restoreMQTTG1(ctx, d, mqtt, pwd, ok))
	}
}

// restoreMQTTG1: MQTTManagerG1.restore.
func restoreMQTTG1(ctx context.Context, d *Device, m *ojson.Value, pwd string, hasPwd bool) *string {
	if !m.Get("enable").Bool() {
		return d.cmd(ctx, "/settings?mqtt_enable=false")
	}
	user := m.Get("user").Text()
	if !hasPwd { // MQTTManagerG1.set: user == null || pwd == null → both empty
		user, pwd = "", ""
	}
	cmd := "/settings?mqtt_enable=true&mqtt_server=" + q(m.Get("server").Text()) + "&mqtt_user=" + q(user) + "&mqtt_pass=" + q(pwd)
	if id := m.Get("id").Text(); id != "" {
		cmd += "&mqtt_id=" + q(id)
	}
	cmd += "&mqtt_reconnect_timeout_max=" + itoa(m.Get("reconnect_timeout_max").Int()) + "&mqtt_reconnect_timeout_min=" + itoa(m.Get("reconnect_timeout_min").Int())
	if cs := m.Get("clean_session").Text(); cs != "" {
		cmd += "&mqtt_clean_session=" + cs
	}
	cmd += "&mqtt_keep_alive=" + itoa(m.Get("keep_alive").Int()) + "&mqtt_max_qos=" + itoa(m.Get("max_qos").Int())
	if r := m.Get("retain").Text(); r != "" {
		cmd += "&mqtt_retain=" + r
	}
	cmd += "&mqtt_update_period=" + itoa(m.Get("update_period").Int())
	return d.cmd(ctx, cmd)
}

// restoreStaG1: WIFIManagerG1.restore.
func restoreStaG1(ctx context.Context, d *Device, net string, sta *ojson.Value, pwd string) *string {
	if !sta.Get("enabled").Bool() {
		return d.cmd(ctx, "/settings/"+net+"?enabled=false")
	}
	return d.cmd(ctx, "/settings/"+net+"?enabled=true&"+nodePars(sta, "ssid", "ipv4_method", "ip", "dns")+
		"&key="+q(pwd)+"&netmask="+sta.Get("mask").Text()+"&gateway="+sta.Get("gw").Text())
}

// restoreActionsG1: Actions.restore — one request per event and index.
func restoreActionsG1(ctx context.Context, d *Device, actions *ojson.Value, e *errs) {
	acts := actions.Get("actions")
	for _, name := range acts.Keys() {
		for _, entry := range acts.Get(name).Items() {
			cmd := "/settings/actions?name=" + name
			for _, k := range entry.Keys() {
				cmd += actionPar(k, entry.Get(k))
			}
			e.add(d.cmd(ctx, cmd))
		}
	}
}

func actionPar(name string, v *ojson.Value) string {
	if v.Kind() != ojson.Array {
		return "&" + name + "=" + q(v.Text())
	}
	if v.Len() == 0 {
		return "&" + name + "[]="
	}
	res := ""
	for i, it := range v.Items() {
		if it.Kind() == ojson.Object {
			for _, fk := range it.Keys() {
				res += "&" + name + "[" + itoa(i) + "][" + fk + "]=" + q(it.Get(fk).Text())
			}
		} else {
			res += "&" + name + "[]=" + q(it.Text())
		}
	}
	return res
}

// nodePars: AbstractG1Device.jsonNodeToURLPar.
func nodePars(n *ojson.Value, pars ...string) string {
	res := pars[0] + "=" + q(n.Get(pars[0]).Text())
	for _, p := range pars[1:] {
		res += "&" + p + "=" + q(n.Get(p).Text())
	}
	return res
}

// entryPars: AbstractG1Device.jsonEntrySetToURLPar — every member, in order.
func entryPars(n *ojson.Value) string {
	var parts []string
	for _, k := range n.Keys() {
		parts = append(parts, entryPar(k, n.Get(k)))
	}
	return strings.Join(parts, "&")
}

func entryPar(name string, v *ojson.Value) string {
	if v.Kind() != ojson.Array {
		return name + "=" + q(v.Text())
	}
	if v.Len() == 0 {
		return name + "[]="
	}
	var parts []string
	for _, it := range v.Items() {
		parts = append(parts, name+"[]="+q(it.Text()))
	}
	return strings.Join(parts, "&")
}

// ---- Gen2+ ------------------------------------------------------------------------

func checkG2(ctx context.Context, d *Device, f Files) Check {
	res := Check{}
	devInfo := f["Shelly.GetDeviceInfo.json"]
	if !devInfo.Exists() || !compatibleModels(devInfo, d) {
		res.put(ErrModel, "")
		return res
	}
	config := f["Shelly.GetConfig.json"]
	sameDevice := strings.ToUpper(devInfo.Get("mac").Text()) == d.MAC
	if !sameDevice {
		res.put(PreRestoreHost, devInfo.Get("id").Text())
	}
	checkDynamic(ctx, d, f, res)
	if devInfo.Get("auth_en").Bool() {
		res.put(AskLogin, "admin")
	}
	if cur := d.currentConnection(ctx); cur != NetUnknown {
		w := config.Path("wifi", "sta")
		if w.Get("enable").Bool() && (sameDevice || w.Get("ipv4mode").Text() == "dhcp") && cur != NetPrimary && !w.Get("is_open").Bool() {
			res.put(AskWiFi1, w.Get("ssid").Text())
		}
		w2 := config.Path("wifi", "sta1")
		if w2.Exists() && w2.Get("enable").Bool() && (sameDevice || w2.Get("ipv4mode").Text() == "dhcp") && cur != NetSecondary && !w2.Get("is_open").Bool() {
			res.put(AskWiFi2, w2.Get("ssid").Text())
		}
		ap := config.Path("wifi", "ap")
		if ap.Exists() && ap.Get("enable").Bool() && cur != NetAP && !ap.Get("is_open").Bool() {
			res.put(AskWiFiAP, ap.Get("ssid").Text())
		}
	}
	if m := config.Get("mqtt"); m.Exists() && m.Get("enable").Bool() && m.Get("user").Text() != "" {
		res.put(AskMQTT, m.Get("user").Text())
	}
	if stored := f["Script.List.json"]; stored.Exists() && stored.Get("scripts").Len() > 0 {
		existing := map[string]bool{}
		if cur, err := d.get(ctx, "/rpc/Script.List"); err == nil {
			for _, s := range cur.Get("scripts").Items() {
				existing[s.Get("name").Text()] = true
			}
		}
		var same, enabled []string
		for _, s := range stored.Get("scripts").Items() {
			n := s.Get("name").Text()
			if existing[n] {
				same = append(same, n)
			}
			if s.Get("enable").Bool() {
				enabled = append(enabled, n)
			}
		}
		if len(same) > 0 {
			res.put(QScriptsOverride, strings.Join(same, ", "))
		}
		if len(enabled) > 0 {
			res.put(QScriptsEnable, strings.Join(enabled, ", "))
		}
	}
	if m := g2Models(d); m != nil && m.check != nil {
		st := loadState(ctx, d)
		m.check(ctx, d, st, f, res)
	}
	return res
}

// compatibleModels: RestoreUtil.compatibleModels — same app (a Zigbee "ZB"
// suffix ignored), same model, or two apps of one compatibility group.
func compatibleModels(devInfo *ojson.Value, d *Device) bool {
	backApp := strings.TrimSuffix(devInfo.Get("app").Text(), "ZB")
	if backApp == d.App || (d.Model != "" && devInfo.Get("model").Text() == d.Model) {
		return true
	}
	for _, group := range compatibility {
		found := false
		for _, id := range group {
			if id == backApp || id == d.App {
				if found {
					return true
				}
				found = true
			}
		}
	}
	return false
}

var compatibility = [][]string{
	{"S1G3", "Mini1G3", "S1G4", "Mini1G4"},
	{"S1PMG3", "Mini1PMG3", "S1PMG4", "Mini1PMG4"},
	{"S2PMG3", "S2PMG4"},
	{"DimmerG3", "DimmerG4"},
	{"EMG3", "EMG4"},
}

func restoreG2(ctx context.Context, d *Device, f Files, a Answers, e *errs) {
	config := f["Shelly.GetConfig.json"].Clone()
	st := loadState(ctx, d)
	e.msg("->r_step:specific")
	if m := g2Models(d); m != nil && m.restore != nil {
		m.restore(ctx, d, st, f, config, e)
	}
	if d.offline {
		if len(*e) <= 1 {
			e.msg(ErrUnknown)
		}
		return
	}
	e.msg("->r_step:DynamicComponents")
	restoreDynamic(ctx, d, f, e)
	e.msg("->r_step:restoreCommonConfig")
	restoreCommonConfig(ctx, d, config, a, e)
	e.msg("->r_step:Scheduler")
	if sch := f["Schedule.List.json"]; sch.Exists() {
		e.add(d.post(ctx, "Schedule.DeleteAll", ojson.NewObject()))
		for _, job := range sch.Get("jobs").Items() {
			j := job.Clone()
			j.Remove("id")
			e.add(d.post(ctx, "Schedule.Create", j))
		}
	}
	e.msg("->r_step:Script")
	if !a.has(QScriptsSkip) {
		restoreScripts(ctx, d, f, a.has(QScriptsOverride), a.has(QScriptsEnable), e)
	}
	e.msg("->r_step:KVS")
	if kvs := f["KVS.GetMany.json"]; kvs.Exists() {
		restoreKVS(ctx, d, kvs, e)
	}
	e.msg("->r_step:Webhooks")
	e.add(d.post(ctx, "Webhook.DeleteAll", ojson.NewObject()))
	for _, h := range f["Webhook.List.json"].Get("hooks").Items() {
		hook := h.Clone()
		hook.Remove("id")
		if r := d.post(ctx, "Webhook.Create", hook); r != nil {
			s := "Action \"" + h.Get("name").Text() + "\" - error: " + *r
			e.add(&s)
		} else {
			e.add(nil)
		}
	}
	e.msg("->r_step:WIFIManagerG2")
	if cur := d.currentConnection(ctx); cur != NetUnknown {
		w2 := config.Path("wifi", "sta1")
		if w2.Exists() && (a.has(AskWiFi2) || w2.Get("is_open").Bool() || !w2.Get("enable").Bool()) && cur != NetSecondary {
			e.add(restoreStaG2(ctx, d, "sta1", w2, a[AskWiFi2]))
		}
		w := config.Path("wifi", "sta")
		if (a.has(AskWiFi1) || w.Get("is_open").Bool() || !w.Get("enable").Bool()) && cur != NetPrimary {
			e.add(restoreStaG2(ctx, d, "sta", w, a[AskWiFi1]))
		}
		ap := config.Path("wifi", "ap")
		if cur != NetAP && ap.Exists() && (a.has(AskWiFiAP) || ap.Get("is_open").Bool() || !ap.Get("enable").Bool()) {
			e.add(restoreAPRoam(ctx, d, config.Get("wifi"), a[AskWiFiAP]))
		} else {
			e.add(restoreRoam(ctx, d, config.Get("wifi")))
		}
	}
	e.msg("->r_step:LoginManagerG2")
	if pwd, ok := a[AskLogin]; ok {
		e.add(setAuthG2(ctx, d, pwd))
	} else if !f["Shelly.GetDeviceInfo.json"].Get("auth_en").Bool() {
		e.add(setAuthG2(ctx, d, ""))
	}
}

// restoreCommonConfig: BLE, Cloud, Sys (device, sntp, debug), Matter,
// Zigbee, MQTT — and Ethernet on Pro devices.
func restoreCommonConfig(ctx context.Context, d *Device, config *ojson.Value, a Answers, e *errs) {
	e.add(d.post(ctx, "BLE.SetConfig", ojson.Obj("config", config.Get("ble"))))
	e.add(d.post(ctx, "Cloud.SetConfig", ojson.Obj("config", ojson.Obj("enable", config.Path("cloud", "enable").Bool()))))
	sys := config.Get("sys")
	dev := sys.Get("device").Clone()
	if dev.Kind() != ojson.Object {
		dev = ojson.NewObject()
	}
	for _, k := range []string{"mac", "fw_id", "addon_type", "profile"} {
		dev.Remove(k)
	}
	e.add(d.post(ctx, "Sys.SetConfig", ojson.Obj("config", ojson.Obj("device", dev, "sntp", sys.Get("sntp"), "debug", sys.Get("debug")))))
	if m := config.Get("matter"); m.Exists() {
		e.add(d.post(ctx, "Matter.SetConfig", ojson.Obj("config", m)))
	}
	if z := config.Get("zigbee"); z.Exists() {
		e.add(d.post(ctx, "Zigbee.SetConfig", ojson.Obj("config", z)))
	}
	// Java's condition, with its operator precedence (FEATURE_PARITY O1).
	mqtt := config.Get("mqtt")
	if mqtt.Exists() && a.has(AskMQTT) || !mqtt.Get("enable").Bool() || mqtt.Get("user").Text() == "" {
		m := mqtt.Clone()
		if m.Kind() != ojson.Object {
			m = ojson.NewObject()
		}
		if pwd := a[AskMQTT]; pwd != "" {
			m.Set("pass", ojson.Str(pwd))
		}
		e.add(d.post(ctx, "MQTT.SetConfig", ojson.Obj("config", m)))
	}
	if d.Pro {
		e.add(d.post(ctx, "Eth.SetConfig", ojson.Obj("config", config.Get("eth"))))
	}
}

// restoreStaG2: WIFIManagerG2.restore.
func restoreStaG2(ctx context.Context, d *Device, net string, w *ojson.Value, pwd string) *string {
	if !w.Get("enable").Bool() {
		return d.post(ctx, "WiFi.SetConfig", ojson.Obj("config", ojson.Obj(net, ojson.Obj("enable", false))))
	}
	p := ojson.Obj("ssid", w.Get("ssid").Text(), "pass", pwd, "enable", true)
	if w.Get("ipv4mode").Text() == "static" {
		p.Set("ipv4mode", ojson.Str("static"))
		p.Set("ip", ojson.Str(w.Get("ip").Text()))
		if v := w.Get("netmask").Text(); v != "" {
			p.Set("netmask", ojson.Str(v))
		}
		if v := w.Get("gw").Text(); v != "" {
			p.Set("gw", ojson.Str(v))
		}
		if v := w.Get("nameserver").Text(); v != "" {
			p.Set("nameserver", ojson.Str(v))
		} else {
			p.Set("nameserver", ojson.NullV())
		}
	} else {
		p.Set("ipv4mode", ojson.Str("dhcp"))
	}
	return d.post(ctx, "WiFi.SetConfig", ojson.Obj("config", ojson.Obj(net, p)))
}

// restoreAPRoam: WIFIManagerG2.restoreAP_roam.
func restoreAPRoam(ctx context.Context, d *Device, wifi *ojson.Value, pwd string) *string {
	out := ojson.NewObject()
	if ap := wifi.Get("ap"); ap.Exists() {
		a := ap.Clone()
		if !ap.Get("is_open").Bool() {
			a.Set("pass", ojson.Str(pwd))
		}
		out.Set("ap", a)
	}
	if roam := wifi.Get("roam"); roam.Exists() {
		out.Set("roam", roam.Clone())
	}
	if out.Len() == 0 {
		return nil
	}
	return d.post(ctx, "WiFi.SetConfig", ojson.Obj("config", out))
}

// restoreRoam: WIFIManagerG2.restoreRoam.
func restoreRoam(ctx context.Context, d *Device, wifi *ojson.Value) *string {
	roam := wifi.Get("roam")
	if !roam.Exists() {
		return nil
	}
	return d.post(ctx, "WiFi.SetConfig", ojson.Obj("config", ojson.Obj("roam", roam.Clone())))
}

// setAuthG2: LoginManagerG2.set / disable (realm = the device id).
func setAuthG2(ctx context.Context, d *Device, pwd string) *string {
	realm := d.Hostname // LoginManagerG2(this, true): the realm is the host name
	var ha1 *ojson.Value = ojson.NullV()
	if pwd != "" {
		ha1 = ojson.Str(sha256hex("admin:" + realm + ":" + pwd))
	}
	return d.post(ctx, "Shelly.SetAuth", ojson.Obj("user", "admin", "realm", realm, "ha1", ha1))
}

// restoreScripts: Script.restoreAll — rename on name clashes unless
// overwriting, upload in 1024-character pieces, enable like the backup.
func restoreScripts(ctx context.Context, d *Device, f Files, override, enable bool, e *errs) {
	stored := f["Script.List.json"]
	if !stored.Exists() || stored.Get("scripts").Len() == 0 {
		return
	}
	cur, err := d.get(ctx, "/rpc/Script.List")
	if err != nil {
		e.msg(errorText(err))
		return
	}
	existing := map[string]int{}
	for _, s := range cur.Get("scripts").Items() {
		existing[s.Get("name").Text()] = s.Get("id").Int()
	}
	for _, s := range stored.Get("scripts").Items() {
		name := s.Get("name").Text()
		target := name
		if _, clash := existing[target]; clash && !override {
			target = name + "_restored"
			for i := 1; ; i++ {
				if _, c := existing[target]; !c {
					break
				}
				target = name + "_restored" + itoa(i)
			}
		}
		code := f[name+".mjs.json"].Get("code").Text()
		var id int
		if eid, ok := existing[target]; ok {
			id = eid
		} else {
			res, err := d.call(ctx, "Script.Create", ojson.Obj("name", target))
			if err != nil {
				e.msg(errorText(err))
				return
			}
			id = res.Get("id").Int()
		}
		e.add(putCode(ctx, d, id, code))
		e.add(d.post(ctx, "Script.SetConfig", ojson.Obj("id", id, "config", ojson.Obj("enable", enable && s.Get("enable").Bool()))))
	}
}

// putCode: Script.putCode (pieces of 1024 characters, append after the first).
func putCode(ctx context.Context, d *Device, id int, code string) *string {
	runes := []rune(code)
	for start := 0; start < len(runes); start += 1024 {
		end := start + 1024
		if end > len(runes) {
			end = len(runes)
		}
		seg := crlf.ReplaceAllString(string(runes[start:end]), "\n")
		p := ojson.Obj("id", id)
		if start > 0 {
			p.Set("append", ojson.BoolV(true))
		}
		p.Set("code", ojson.Str(seg))
		if r := d.post(ctx, "Script.PutCode", p); r != nil {
			return r
		}
	}
	return nil
}

// restoreKVS: KVS.restoreKVS — set the items that differ from the device's.
func restoreKVS(ctx context.Context, d *Device, kvs *ojson.Value, e *errs) {
	cur, err := d.paged(ctx, "/rpc/KVS.GetMany", "items")
	if err != nil {
		return
	}
	type item struct{ key, etag, value string }
	have := map[item]bool{}
	curItems := cur.Get("items")
	if curItems.Kind() == ojson.Array {
		for _, it := range curItems.Items() {
			have[item{it.Get("key").Text(), it.Get("etag").Text(), it.Get("value").Text()}] = true
		}
	} else {
		for _, k := range curItems.Keys() {
			v := curItems.Get(k)
			have[item{k, v.Get("etag").Text(), v.Get("value").Text()}] = true
		}
	}
	set := func(it item) {
		if !have[it] {
			e.add(d.post(ctx, "KVS.Set", ojson.Obj("key", it.key, "value", it.value)))
		}
	}
	items := kvs.Get("items")
	if items.Kind() == ojson.Array { // fw >= 1.5.0
		for _, it := range items.Items() {
			set(item{it.Get("key").Text(), it.Get("etag").Text(), it.Get("value").Text()})
		}
	} else {
		for _, k := range items.Keys() {
			v := items.Get(k)
			set(item{k, v.Get("etag").Text(), v.Get("value").Text()})
		}
	}
}

// indexed: RestoreUtil.createIndexedRestoreNode — {"id":N,"config":<type:N minus id>}.
func indexed(config *ojson.Value, typ string, idx int) *ojson.Value {
	data := config.Get(typ + ":" + itoa(idx)).Clone()
	if data.Kind() == ojson.Object {
		data.Remove("id")
	}
	return ojson.Obj("id", idx, "config", data)
}

// sortedKeys is used where Java iterates a HashMap/HashSet (order unspecified).
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
