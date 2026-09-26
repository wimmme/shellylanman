// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// the "Devices settings" dialog (view/devsettings/DialogDeviceSettings,
// PanelWIFI, PanelResLogin, PanelMQTTG1, PanelMQTTG2, PanelMQTTMix,
// PanelOthers) and the managers it uses (g1/modules and g2/modules
// WIFIManager*, LoginManager*, MQTTManager*, TimeAndLocationManager*,
// InputResetManager*, setCloudEnabled).

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Settings dialog sections (tabs).
const (
	SectionWiFi1  = "wifi1"
	SectionWiFi2  = "wifi2"
	SectionLogin  = "login"
	SectionMQTT   = "mqtt"
	SectionOthers = "others"
)

// Per-device results of an apply (dlgSetMultiMsg*).
const (
	ResultOK       = "ok"
	ResultFail     = "fail"
	ResultQueued   = "queued"
	ResultExcluded = "excluded"
)

var (
	// ErrAllExcluded: none of the selected devices supports the section.
	ErrAllExcluded = errors.New("all devices excluded")
	// ErrInvalid wraps a validation message of a form.
	ErrInvalid = errors.New("invalid")
)

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalid, msg) }

// DeviceRef names a device in results and exclusion lists.
type DeviceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"` // host name, as the original's result lines
}

// ResultLine is the outcome for one device.
type ResultLine struct {
	DeviceRef
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

// ConfigForm is what a tab shows: the values common to all selected devices
// ("" / null where they differ) and the devices that were excluded.
type ConfigForm struct {
	Section  string      `json:"section"`
	Variant  string      `json:"variant,omitempty"` // login/mqtt: "g1", "g2" or "mix"
	Devices  int         `json:"devices"`
	Excluded []DeviceRef `json:"excluded"`
	WiFi     *WiFiForm   `json:"wifi,omitempty"`
	Login    *LoginForm  `json:"login,omitempty"`
	MQTT     *MQTTForm   `json:"mqtt,omitempty"`
	Others   *OthersForm `json:"others,omitempty"`
}

// ---- selection ------------------------------------------------------------------

type cfgTarget struct {
	e       *entry
	d       model.Device
	gen1    bool
	blu     bool
	offline bool // off line or archived: the original queues a deferred task
}

// targets resolves the selection like DialogDeviceSettings: BTHome devices
// (sensors, buttons) are dropped; the BLU TRV stays but no tab supports it.
func (m *Devices) targets(ids []string) ([]cfgTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []cfgTarget
	for _, id := range ids {
		e, ok := m.devs[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		if e.dev.Gen == model.GenBTHome {
			continue
		}
		t := cfgTarget{e: e, d: e.dev, gen1: e.dev.Gen == "1", blu: e.dev.Gen == model.GenBLU}
		t.offline = e.dev.Status == model.StatusGhost || e.dev.Status == model.StatusOffline
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, ErrAllExcluded
	}
	return out, nil
}

func ref(t cfgTarget) DeviceRef {
	name := t.d.Hostname
	if name == "" {
		name = t.d.Address()
	}
	return DeviceRef{ID: t.d.ID, Name: name}
}

// usable: a device that can be read and written now.
func (t cfgTarget) usable() bool {
	return !t.blu && !t.offline && t.e.conn != nil && t.d.Managed && (t.d.Gen == "1" || t.d.Gen == "2" || t.d.Gen == "3" || t.d.Gen == "4")
}

func getNode(ctx context.Context, c *shelly.Conn, path string) (parse.Node, error) {
	b, err := c.Get(ctx, path)
	if err != nil {
		return parse.Node{}, err
	}
	return parse.Decode(b), nil
}

// g1cmd runs a Gen1 settings GET; nil on success, the error otherwise
// (AbstractG1Device.sendCommand).
func g1cmd(ctx context.Context, c *shelly.Conn, path string) error {
	_, err := c.Get(ctx, path)
	return err
}

// g2call runs a Gen2+ RPC with POST (AbstractG2Device.postCommand).
func g2call(ctx context.Context, c *shelly.Conn, method string, params any) error {
	_, err := c.Call(ctx, method, params)
	return err
}

func msgOf(err error) string {
	if err == nil {
		return ""
	}
	var api *shelly.APIError
	if errors.As(err, &api) && api.Message != "" {
		return api.Message
	}
	if shelly.IsOffline(err) {
		return "Status-OFFLINE"
	}
	if errors.Is(err, shelly.ErrUnauthorized) {
		return "Status-PROTECTED"
	}
	return err.Error()
}

func lineOf(t cfgTarget, err error) ResultLine {
	if err != nil {
		return ResultLine{DeviceRef: ref(t), Result: ResultFail, Message: msgOf(err)}
	}
	return ResultLine{DeviceRef: ref(t), Result: ResultOK}
}

func q(s string) string { return url.QueryEscape(s) }

// ---- Wi-Fi ------------------------------------------------------------------------

// WiFiForm: PanelWIFI. Static is null when the devices differ ("Keep").
type WiFiForm struct {
	Enabled bool   `json:"enabled"`
	SSID    string `json:"ssid"`
	Static  *bool  `json:"static"`
	IP      string `json:"ip"`
	Netmask string `json:"netmask"`
	Gateway string `json:"gateway"`
	DNS     string `json:"dns"`
}

// WiFiApply is a Wi-Fi tab submission. Mode is "dhcp", "static" or "keep"
// (keep each device's mode). With several devices the IP is never changed.
type WiFiApply struct {
	Enabled  bool   `json:"enabled"`
	SSID     string `json:"ssid"`
	Password string `json:"password"`
	Mode     string `json:"mode"`
	IP       string `json:"ip"`
	Netmask  string `json:"netmask"`
	Gateway  string `json:"gateway"`
	DNS      string `json:"dns"`
	// Confirm: "Wrong parameters may cause devices to disconnect from the network".
	Confirm bool `json:"confirm"`
}

type wifiState struct {
	enabled, static         bool
	ssid, ip, mask, gw, dns string
}

var ipv4Re = regexp.MustCompile(`^((0|1\d?\d?|2[0-4]?\d?|25[0-5]?|[3-9]\d?)\.){3}(0|1\d?\d?|2[0-4]?\d?|25[0-5]?|[3-9]\d?)$`)

func wifiNet(section string) string {
	if section == SectionWiFi2 {
		return "sta1"
	}
	return "sta"
}

// readWiFi: WIFIManagerG1/G2.init.
func readWiFi(ctx context.Context, t cfgTarget, net string) (wifiState, error) {
	if t.gen1 {
		n, err := getNode(ctx, t.e.conn, "/settings/"+net)
		if err != nil {
			return wifiState{}, err
		}
		if !n.Get("enabled").Exists() {
			return wifiState{}, errors.New("no " + net)
		}
		return wifiState{enabled: n.Get("enabled").Bool(), ssid: n.Get("ssid").Str(""), static: n.Get("ipv4_method").Str("") == "static",
			ip: n.Get("ip").Str(""), mask: n.Get("mask").Str(""), gw: n.Get("gw").Str(""), dns: n.Get("dns").Str("")}, nil
	}
	n, err := getNode(ctx, t.e.conn, "/rpc/WiFi.GetConfig")
	if err != nil {
		return wifiState{}, err
	}
	w := n.Get(net)
	if !w.Exists() {
		return wifiState{}, errors.New("no " + net)
	}
	return wifiState{enabled: w.Get("enable").Bool(), ssid: w.Get("ssid").Str(""), static: w.Get("ipv4mode").Str("") == "static",
		ip: w.Get("ip").Str(""), mask: w.Get("netmask").Str(""), gw: w.Get("gw").Str(""), dns: w.Get("nameserver").Str("")}, nil
}

func (m *Devices) wifiForm(ctx context.Context, ts []cfgTarget, section string) *ConfigForm {
	f := &ConfigForm{Section: section, Devices: len(ts), Excluded: []DeviceRef{}, WiFi: &WiFiForm{}}
	first := true
	var static *bool
	for _, t := range ts {
		var w wifiState
		var err error = errors.New("excluded")
		if t.usable() {
			w, err = readWiFi(ctx, t, wifiNet(section))
		}
		if err != nil {
			f.Excluded = append(f.Excluded, ref(t))
			continue
		}
		g := f.WiFi
		if first {
			*g = WiFiForm{Enabled: w.enabled, SSID: w.ssid, IP: w.ip, Netmask: w.mask, Gateway: w.gw, DNS: w.dns}
			s := w.static
			static = &s
			first = false
			continue
		}
		if w.enabled != g.Enabled {
			g.Enabled = false
		}
		if w.ssid != g.SSID {
			g.SSID = ""
		}
		if static != nil && w.static != *static {
			static = nil
		}
		if w.ip != g.IP {
			g.IP = ""
		}
		if w.mask != g.Netmask {
			g.Netmask = ""
		}
		if w.gw != g.Gateway {
			g.Gateway = ""
		}
		if w.dns != g.DNS {
			g.DNS = ""
		}
	}
	if first {
		static = ptrBool(false)
	}
	f.WiFi.Static = static
	return f
}

func ptrBool(b bool) *bool { return &b }

func validateWiFi(a WiFiApply, single bool) error {
	if !a.Enabled {
		return nil
	}
	for _, c := range []struct{ v, msg string }{{a.IP, "wrongIP"}, {a.Gateway, "wrongGW"}, {a.Netmask, "wrongMask"}, {a.DNS, "wrongDNS"}} {
		if c.v != "" && !ipv4Re.MatchString(c.v) {
			return invalid(c.msg)
		}
	}
	if a.Mode == "static" && single && a.IP == "" {
		return invalid("staticIPRequired")
	}
	if (a.Mode == "static" || a.Mode == "keep") && (a.Netmask == "" || a.Gateway == "") {
		return invalid("staticMaskGWRequired")
	}
	if a.SSID == "" || a.Password == "" {
		return invalid("ssidRequired")
	}
	return nil
}

func (m *Devices) applyWiFi(ctx context.Context, ts []cfgTarget, section string, a WiFiApply) ([]ResultLine, error) {
	a.SSID, a.Password = strings.TrimSpace(a.SSID), strings.TrimSpace(a.Password)
	a.IP, a.Netmask, a.Gateway, a.DNS = strings.TrimSpace(a.IP), strings.TrimSpace(a.Netmask), strings.TrimSpace(a.Gateway), strings.TrimSpace(a.DNS)
	single := len(ts) == 1
	if err := validateWiFi(a, single); err != nil {
		return nil, err
	}
	if !a.Confirm {
		return nil, ErrConfirm
	}
	net := wifiNet(section)
	var out []ResultLine
	for _, t := range ts {
		var cur wifiState
		var err error = errors.New("excluded")
		if t.usable() {
			cur, err = readWiFi(ctx, t, net)
		}
		if err != nil {
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultExcluded})
			continue
		}
		out = append(out, lineOf(t, setWiFi(ctx, t, net, a, cur, single)))
	}
	return out, nil
}

// setWiFi: WIFIManager.disable / set(ssid,pwd) / set(ssid,pwd,ip,netmask,gw,dns).
func setWiFi(ctx context.Context, t cfgTarget, net string, a WiFiApply, cur wifiState, single bool) error {
	c := t.e.conn
	if !a.Enabled {
		if t.gen1 {
			return g1cmd(ctx, c, "/settings/"+net+"?enabled=false")
		}
		return g2call(ctx, c, "WiFi.SetConfig", map[string]any{"config": map[string]any{net: map[string]any{"enable": false}}})
	}
	dhcp := a.Mode == "dhcp"
	if a.Mode == "keep" {
		dhcp = !cur.static
	}
	ip := a.IP
	if !single { // the IP field is disabled with several devices: keep each device's IP
		ip = cur.ip
	}
	if t.gen1 {
		cmd := "/settings/" + net + "?enabled=true&ssid=" + q(a.SSID) + "&key=" + q(a.Password)
		if dhcp {
			return g1cmd(ctx, c, cmd+"&ipv4_method=dhcp")
		}
		cmd += "&ipv4_method=static&ip=" + ip + "&netmask=" + a.Netmask + "&gateway=" + a.Gateway
		if a.DNS != "" {
			cmd += "&dns=" + a.DNS
		}
		return g1cmd(ctx, c, cmd)
	}
	p := map[string]any{"ssid": a.SSID, "pass": a.Password, "enable": true}
	if dhcp {
		p["ipv4mode"] = "dhcp"
	} else {
		p["ipv4mode"] = "static"
		p["ip"] = ip
		if a.Netmask != "" {
			p["netmask"] = a.Netmask
		}
		if a.Gateway != "" {
			p["gw"] = a.Gateway
		}
		if a.DNS != "" {
			p["nameserver"] = a.DNS
		} else {
			p["nameserver"] = nil
		}
	}
	return g2call(ctx, c, "WiFi.SetConfig", map[string]any{"config": map[string]any{net: p}})
}

// ---- restricted login ----------------------------------------------------------------

// LoginForm: PanelResLogin. User is fixed to "admin" unless all devices are Gen1.
type LoginForm struct {
	Enabled bool   `json:"enabled"`
	User    string `json:"user"`
}

// LoginApply is a login tab submission.
type LoginApply struct {
	Enabled  bool   `json:"enabled"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type loginState struct {
	enabled     bool
	user, realm string
}

// readLogin: LoginManagerG1.init (/settings/login) / LoginManagerG2.init (/shelly).
func readLogin(ctx context.Context, t cfgTarget) (loginState, error) {
	if t.gen1 {
		n, err := getNode(ctx, t.e.conn, "/settings/login")
		if err != nil {
			return loginState{}, err
		}
		return loginState{enabled: n.Get("enabled").Bool(), user: n.Get("username").Str("")}, nil
	}
	n, err := getNode(ctx, t.e.conn, "/shelly")
	if err != nil {
		return loginState{}, err
	}
	return loginState{enabled: n.Get("auth_en").Bool(), user: shelly.DigestUser, realm: n.Get("id").Str("")}, nil
}

// variant: "g1" or "g2" when all devices are of one kind (an archived
// device makes it "mix", as DialogDeviceSettings.getTypes).
func variant(ts []cfgTarget) string {
	v := ""
	for _, t := range ts {
		k := "g2"
		switch {
		case t.d.Status == model.StatusGhost:
			return "mix"
		case t.gen1:
			k = "g1"
		}
		if v == "" {
			v = k
		} else if v != k {
			return "mix"
		}
	}
	return v
}

func (m *Devices) loginForm(ctx context.Context, ts []cfgTarget) *ConfigForm {
	f := &ConfigForm{Section: SectionLogin, Variant: variant(ts), Devices: len(ts), Excluded: []DeviceRef{}, Login: &LoginForm{}}
	first := true
	for _, t := range ts {
		if t.blu {
			f.Excluded = append(f.Excluded, ref(t))
			continue
		}
		if !t.usable() {
			continue // queued at apply when off line; otherwise reported then
		}
		l, err := readLogin(ctx, t)
		if err != nil {
			continue
		}
		if first {
			f.Login.Enabled, f.Login.User = l.enabled, l.user
			first = false
			continue
		}
		if l.enabled != f.Login.Enabled {
			f.Login.Enabled = false
		}
		if l.user != f.Login.User {
			f.Login.User = ""
		}
	}
	if f.Variant != "g1" {
		f.Login.User = shelly.DigestUser
	}
	return f
}

func (m *Devices) applyLogin(ctx context.Context, ts []cfgTarget, a LoginApply) ([]ResultLine, error) {
	a.User = strings.TrimSpace(a.User)
	if variant(ts) != "g1" {
		a.User = shelly.DigestUser
	}
	if a.Enabled && (a.User == "" || a.Password == "") {
		return nil, invalid("userRequired")
	}
	var out []ResultLine
	for _, t := range ts {
		switch {
		case t.blu:
		case t.usable():
			out = append(out, lineOf(t, m.setLogin(ctx, t, a)))
		case t.offline:
			m.defer_(t, TaskLogin, a)
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultQueued})
		default:
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultExcluded})
		}
	}
	return out, nil
}

// setLogin: LoginManagerG1.set/disable, LoginManagerG2.set/disable
// (Shelly.SetAuth with ha1 = sha256("admin:<realm>:<password>")). On success
// the new credentials are kept for the device, as the original does.
func (m *Devices) setLogin(ctx context.Context, t cfgTarget, a LoginApply) error {
	c := t.e.conn
	var err error
	if t.gen1 {
		if a.Enabled {
			err = g1cmd(ctx, c, "/settings/login?enabled=true&username="+q(a.User)+"&password="+q(a.Password))
		} else {
			err = g1cmd(ctx, c, "/settings/login?enabled=false")
		}
	} else {
		l, rerr := readLogin(ctx, t)
		if rerr != nil {
			return rerr
		}
		var ha1 any
		if a.Enabled {
			h := sha256.Sum256([]byte(shelly.DigestUser + ":" + l.realm + ":" + a.Password))
			ha1 = hex.EncodeToString(h[:])
		}
		err = g2call(ctx, c, "Shelly.SetAuth", map[string]any{"user": shelly.DigestUser, "realm": l.realm, "ha1": ha1})
	}
	if err != nil {
		return err
	}
	if a.Enabled {
		cred := shelly.Credentials{User: a.User, Password: a.Password}
		_ = m.writeCred(secretDevicePfx+t.d.ID, cred)
		c.SetCredentials(&cred)
	} else {
		_ = m.writeCred(secretDevicePfx+t.d.ID, shelly.Credentials{})
		c.SetCredentials(nil)
	}
	return nil
}

// ---- MQTT ----------------------------------------------------------------------------

// MQTTForm: PanelMQTTG1 / PanelMQTTG2 / PanelMQTTMix. Pointers are null when
// the devices differ ("Keep" / empty field).
type MQTTForm struct {
	Enabled bool   `json:"enabled"`
	Server  string `json:"server"`
	User    string `json:"user"`
	Prefix  string `json:"prefix"`
	NoPwd   bool   `json:"noPassword"`
	// Gen1 only
	ReconnectMax *int  `json:"reconnectMax,omitempty"`
	ReconnectMin *int  `json:"reconnectMin,omitempty"`
	CleanSession *bool `json:"cleanSession,omitempty"`
	KeepAlive    *int  `json:"keepAlive,omitempty"`
	QoS          *int  `json:"qos,omitempty"`
	Retain       *bool `json:"retain,omitempty"`
	UpdatePeriod *int  `json:"updatePeriod,omitempty"`
	// Gen2+ only
	Control   *bool `json:"control,omitempty"`
	RPC       *bool `json:"rpc,omitempty"`
	RPCNtf    *bool `json:"rpcNtf,omitempty"`
	StatusNtf *bool `json:"statusNtf,omitempty"`
}

// MQTTApply is an MQTT tab submission. Null pointers leave a value unchanged.
type MQTTApply struct {
	Enabled       bool   `json:"enabled"`
	Server        string `json:"server"`
	User          string `json:"user"`
	Password      string `json:"password"`
	NoPassword    bool   `json:"noPassword"`
	DefaultPrefix bool   `json:"defaultPrefix"`
	Prefix        string `json:"prefix"` // only with one device
	ReconnectMax  *int   `json:"reconnectMax,omitempty"`
	ReconnectMin  *int   `json:"reconnectMin,omitempty"`
	CleanSession  *bool  `json:"cleanSession,omitempty"`
	KeepAlive     *int   `json:"keepAlive,omitempty"`
	QoS           *int   `json:"qos,omitempty"`
	Retain        *bool  `json:"retain,omitempty"`
	UpdatePeriod  *int   `json:"updatePeriod,omitempty"`
	Control       *bool  `json:"control,omitempty"`
	RPC           *bool  `json:"rpc,omitempty"`
	RPCNtf        *bool  `json:"rpcNtf,omitempty"`
	StatusNtf     *bool  `json:"statusNtf,omitempty"`

	// Set by the service: which panel the original would show.
	Variant string `json:"variant,omitempty"`
	// Multi: more than one device selected (prefix "" = do not alter).
	Multi bool `json:"multi,omitempty"`
}

// mqttVariant: PanelMQTTG1 for Gen1 only, PanelMQTTG2 for Gen2+ only, and
// PanelMQTTMix when mixed or when a device is off line (deferred tasks are
// only offered there).
func mqttVariant(ts []cfgTarget) string {
	v := variant(ts)
	for _, t := range ts {
		if t.offline {
			return "mix"
		}
	}
	return v
}

func (m *Devices) readMQTT(ctx context.Context, t cfgTarget) (MQTTForm, error) {
	if t.gen1 {
		n, err := getNode(ctx, t.e.conn, "/settings")
		if err != nil {
			return MQTTForm{}, err
		}
		s := n.Get("mqtt")
		ip := func(k string) *int { v := s.Get(k).Int(); return &v }
		bp := func(k string) *bool { v := s.Get(k).Bool(); return &v }
		return MQTTForm{Enabled: s.Get("enable").Bool(), Server: s.Get("server").Str(""), User: s.Get("user").Str(""), Prefix: s.Get("id").Str(""),
			ReconnectMax: ip("reconnect_timeout_max"), ReconnectMin: ip("reconnect_timeout_min"), CleanSession: bp("clean_session"),
			KeepAlive: ip("keep_alive"), QoS: ip("max_qos"), Retain: bp("retain"), UpdatePeriod: ip("update_period")}, nil
	}
	s, err := getNode(ctx, t.e.conn, "/rpc/MQTT.GetConfig")
	if err != nil {
		return MQTTForm{}, err
	}
	bp := func(k string) *bool { v := s.Get(k).Bool(); return &v }
	return MQTTForm{Enabled: s.Get("enable").Bool(), Server: s.Get("server").Str(""), User: s.Get("user").Str(""), Prefix: s.Get("topic_prefix").Str(""),
		Control: bp("enable_control"), RPC: bp("enable_rpc"), RPCNtf: bp("rpc_ntf"), StatusNtf: bp("status_ntf")}, nil
}

func sameInt(a, b *int) *int {
	if a == nil || b == nil || *a != *b {
		return nil
	}
	return a
}

func sameBool(a, b *bool) *bool {
	if a == nil || b == nil || *a != *b {
		return nil
	}
	return a
}

func (m *Devices) mqttForm(ctx context.Context, ts []cfgTarget) *ConfigForm {
	v := mqttVariant(ts)
	f := &ConfigForm{Section: SectionMQTT, Variant: v, Devices: len(ts), Excluded: []DeviceRef{}}
	var g *MQTTForm
	for _, t := range ts {
		if !t.usable() {
			if t.blu || !t.offline {
				f.Excluded = append(f.Excluded, ref(t))
			}
			continue
		}
		cur, err := m.readMQTT(ctx, t)
		if err != nil {
			f.Excluded = append(f.Excluded, ref(t))
			continue
		}
		if g == nil {
			c := cur
			c.NoPwd = cur.User == ""
			g = &c
			continue
		}
		if cur.Enabled != g.Enabled {
			g.Enabled = false
		}
		if cur.Server != g.Server {
			g.Server = ""
		}
		if cur.User != g.User {
			g.User = ""
		}
		if cur.Prefix != g.Prefix {
			g.Prefix = ""
		}
		g.NoPwd = g.NoPwd && cur.User == ""
		g.ReconnectMax, g.ReconnectMin, g.KeepAlive = sameInt(g.ReconnectMax, cur.ReconnectMax), sameInt(g.ReconnectMin, cur.ReconnectMin), sameInt(g.KeepAlive, cur.KeepAlive)
		g.QoS, g.UpdatePeriod = sameInt(g.QoS, cur.QoS), sameInt(g.UpdatePeriod, cur.UpdatePeriod)
		g.CleanSession, g.Retain = sameBool(g.CleanSession, cur.CleanSession), sameBool(g.Retain, cur.Retain)
		g.Control, g.RPC, g.RPCNtf, g.StatusNtf = sameBool(g.Control, cur.Control), sameBool(g.RPC, cur.RPC), sameBool(g.RPCNtf, cur.RPCNtf), sameBool(g.StatusNtf, cur.StatusNtf)
	}
	if g == nil {
		g = &MQTTForm{}
	}
	if v != "g1" { // only PanelMQTTG1 shows these
		g.ReconnectMax, g.ReconnectMin, g.CleanSession, g.KeepAlive, g.QoS, g.Retain, g.UpdatePeriod = nil, nil, nil, nil, nil, nil, nil
	}
	if v != "g2" {
		g.Control, g.RPC, g.RPCNtf, g.StatusNtf = nil, nil, nil, nil
	}
	f.MQTT = g
	return f
}

func (m *Devices) applyMQTT(ctx context.Context, ts []cfgTarget, a MQTTApply) ([]ResultLine, error) {
	a.Server, a.User, a.Password = strings.TrimSpace(a.Server), strings.TrimSpace(a.User), strings.TrimSpace(a.Password)
	if a.Enabled {
		if a.Server == "" {
			return nil, invalid("mqttServerRequired")
		}
		if !a.NoPassword && (a.User == "" || a.Password == "") {
			return nil, invalid("mqttUserRequired")
		}
	}
	for _, p := range []*int{a.ReconnectMax, a.ReconnectMin, a.KeepAlive, a.UpdatePeriod} {
		if p != nil && (*p < 0 || *p > 65535) {
			return nil, invalid("mqttRange")
		}
	}
	if a.QoS != nil && (*a.QoS < 0 || *a.QoS > 2) {
		return nil, invalid("mqttRange")
	}
	a.Variant, a.Multi = mqttVariant(ts), len(ts) > 1
	slow := time.Duration(m.store.Settings().MQTTSlow) * 100 * time.Millisecond
	var out []ResultLine
	for _, t := range ts {
		switch {
		case t.usable():
			out = append(out, lineOf(t, setMQTT(ctx, t, a)))
			if slow > 0 {
				time.Sleep(slow)
			}
		case t.offline && !t.blu && a.Variant != "g1": // PanelMQTTG1 never queues
			m.defer_(t, TaskMQTT, a)
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultQueued})
		case t.blu:
		default:
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultExcluded})
		}
	}
	return out, nil
}

// setMQTT: MQTTManagerG1.set (short form in the mixed panel, long form with
// the Gen1 extras) and MQTTManagerG2.set (with the Gen2+ extras in the G2 panel).
func setMQTT(ctx context.Context, t cfgTarget, a MQTTApply) error {
	c := t.e.conn
	var prefix *string // nil: use the device default
	if !a.DefaultPrefix {
		p := a.Prefix
		if a.Multi {
			p = "" // do not alter
		}
		prefix = &p
	}
	if !a.Enabled {
		if t.gen1 {
			return g1cmd(ctx, c, "/settings?mqtt_enable=false")
		}
		return g2call(ctx, c, "MQTT.SetConfig", map[string]any{"config": map[string]any{"enable": false}})
	}
	if t.gen1 {
		user, pwd := a.User, a.Password
		if a.NoPassword {
			user, pwd = "", ""
		}
		cmd := "/settings?mqtt_enable=true&mqtt_server=" + q(a.Server) + "&mqtt_user=" + q(user) + "&mqtt_pass=" + q(pwd)
		if a.Variant != "g1" { // MQTTManagerG1.set(server, user, pwd, prefix)
			if prefix != nil && *prefix != "" {
				cmd += "&mqtt_id=" + q(*prefix)
			}
			return g1cmd(ctx, c, cmd)
		}
		if prefix == nil {
			cmd += "&mqtt_id=" // default value
		} else if *prefix != "" {
			cmd += "&mqtt_id=" + q(*prefix)
		}
		addInt := func(k string, v *int) {
			if v != nil {
				cmd += "&" + k + "=" + strconv.Itoa(*v)
			}
		}
		addBool := func(k string, v *bool) {
			if v != nil {
				cmd += "&" + k + "=" + strconv.FormatBool(*v)
			}
		}
		addInt("mqtt_reconnect_timeout_max", a.ReconnectMax)
		addInt("mqtt_reconnect_timeout_min", a.ReconnectMin)
		addBool("mqtt_clean_session", a.CleanSession)
		addInt("mqtt_keep_alive", a.KeepAlive)
		addInt("mqtt_max_qos", a.QoS)
		addBool("mqtt_retain", a.Retain)
		addInt("mqtt_update_period", a.UpdatePeriod)
		return g1cmd(ctx, c, cmd)
	}
	p := map[string]any{"enable": true, "server": a.Server}
	if a.NoPassword {
		p["user"], p["pass"] = nil, nil
	} else {
		p["user"], p["pass"] = a.User, a.Password
	}
	if prefix == nil {
		p["topic_prefix"] = nil // device id is the default prefix
	} else if *prefix != "" {
		p["topic_prefix"] = *prefix
	}
	if a.Variant == "g2" {
		for k, v := range map[string]*bool{"enable_control": a.Control, "enable_rpc": a.RPC, "rpc_ntf": a.RPCNtf, "status_ntf": a.StatusNtf} {
			if v != nil {
				p[k] = *v
			}
		}
	}
	return g2call(ctx, c, "MQTT.SetConfig", map[string]any{"config": p})
}

// ---- others: NTP, cloud, reset from input -----------------------------------------------

// OthersForm: PanelOthers. Cloud and Reset are null when the devices differ
// (or, for Reset, when it does not apply).
type OthersForm struct {
	NTP   string `json:"ntp"`
	Cloud *bool  `json:"cloud"`
	Reset *bool  `json:"reset"`
}

// OthersApply applies one part: "ntp" (NTP), "cloud" or "reset" (Enable).
type OthersApply struct {
	Part   string `json:"part"`
	NTP    string `json:"ntp"`
	Enable *bool  `json:"enable"`
}

// resetState: InputResetManagerG1/G2. mode: "true", "false", "na", "mix".
type resetState struct {
	mode string
	ids  []int // Gen2+ inputs that have factory_reset
}

func (r resetState) asBool() *bool {
	switch r.mode {
	case "true":
		return ptrBool(true)
	case "false":
		return ptrBool(false)
	}
	return nil
}

func readOthers(ctx context.Context, t cfgTarget) (ntp string, reset resetState, err error) {
	if t.gen1 {
		n, err := getNode(ctx, t.e.conn, "/settings")
		if err != nil {
			return "", resetState{}, err
		}
		r := n.Get("factory_reset_from_switch")
		switch {
		case !r.Exists():
			reset.mode = "na"
		case r.Bool():
			reset.mode = "true"
		default:
			reset.mode = "false"
		}
		return n.Path("sntp", "server").Str(""), reset, nil
	}
	n, err := getNode(ctx, t.e.conn, "/rpc/Shelly.GetConfig")
	if err != nil {
		return "", resetState{}, err
	}
	reset.mode = "na"
	for _, k := range n.Keys() {
		var id int
		if _, err := fmt.Sscanf(k, "input:%d", &id); err != nil || id >= 100 {
			continue
		}
		r := n.Path(k, "factory_reset")
		if !r.Exists() {
			continue
		}
		reset.ids = append(reset.ids, id)
		cur := map[bool]string{true: "true", false: "false"}[r.Bool()]
		if (reset.mode == "true" || reset.mode == "false") && reset.mode != cur {
			reset.mode = "mix"
		} else if reset.mode != "mix" {
			reset.mode = cur
		}
	}
	return n.Path("sys", "sntp", "server").Str(""), reset, nil
}

func (m *Devices) othersForm(ctx context.Context, ts []cfgTarget) *ConfigForm {
	f := &ConfigForm{Section: SectionOthers, Devices: len(ts), Excluded: []DeviceRef{}, Others: &OthersForm{}}
	first := true
	for _, t := range ts {
		if t.blu || !t.d.Managed {
			f.Excluded = append(f.Excluded, ref(t))
			continue
		}
		if !t.usable() {
			continue
		}
		ntp, reset, err := readOthers(ctx, t)
		if err != nil {
			continue
		}
		cloud := t.d.CloudEnabled
		g := f.Others
		if first {
			g.NTP, g.Cloud, g.Reset = ntp, &cloud, reset.asBool()
			first = false
			continue
		}
		if ntp != g.NTP {
			g.NTP = ""
		}
		g.Cloud = sameBool(g.Cloud, &cloud)
		g.Reset = sameBool(g.Reset, reset.asBool())
	}
	return f
}

func (m *Devices) applyOthers(ctx context.Context, ts []cfgTarget, a OthersApply) ([]ResultLine, error) {
	a.NTP = strings.TrimSpace(a.NTP)
	var task string
	switch a.Part {
	case "ntp":
		if a.NTP == "" {
			return nil, invalid("ntpRequired")
		}
		task = TaskNTP
	case "cloud":
		if a.Enable == nil {
			return nil, invalid("cloudRequired")
		}
		task = TaskCloud
	case "reset":
		if a.Enable == nil {
			return nil, invalid("resetRequired")
		}
		task = TaskInputReset
	default:
		return nil, invalid("part")
	}
	var out []ResultLine
	for _, t := range ts {
		switch {
		case t.blu || !t.d.Managed:
		case t.offline:
			m.defer_(t, task, a)
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultQueued})
		case t.usable():
			out = append(out, lineOf(t, setOthers(ctx, t, a)))
		default:
			out = append(out, ResultLine{DeviceRef: ref(t), Result: ResultFail, Message: "Status-" + strings.ToUpper(string(t.d.Status))})
		}
	}
	return out, nil
}

// setOthers: TimeAndLocationManager.setSNTPServer, setCloudEnabled,
// InputResetManager.enableReset.
func setOthers(ctx context.Context, t cfgTarget, a OthersApply) error {
	c := t.e.conn
	switch a.Part {
	case "ntp":
		if t.gen1 {
			return g1cmd(ctx, c, "/settings?sntp_server="+q(a.NTP))
		}
		return g2call(ctx, c, "Sys.SetConfig", map[string]any{"config": map[string]any{"sntp": map[string]any{"server": a.NTP}}})
	case "cloud":
		if t.gen1 {
			return g1cmd(ctx, c, "/settings/cloud?enabled="+strconv.FormatBool(*a.Enable))
		}
		return g2call(ctx, c, "Cloud.SetConfig", map[string]any{"config": map[string]any{"enable": *a.Enable}})
	case "reset":
		_, r, err := readOthers(ctx, t)
		if err != nil {
			return err
		}
		if r.mode == "na" {
			return errors.New("notApplicable")
		}
		if t.gen1 {
			return g1cmd(ctx, c, "/settings?factory_reset_from_switch="+strconv.FormatBool(*a.Enable))
		}
		var last error
		for _, id := range r.ids { // every input with factory_reset (FEATURE_PARITY O21)
			if err := g2call(ctx, c, "Input.SetConfig", map[string]any{"id": id, "config": map[string]any{"factory_reset": *a.Enable}}); err != nil {
				last = err
			}
		}
		return last
	}
	return invalid("part")
}

// ---- entry points ------------------------------------------------------------------------

// ConfigForm reads the current values of a section for the selected devices.
func (m *Devices) ConfigForm(ctx context.Context, ids []string, section string) (*ConfigForm, error) {
	ts, err := m.targets(ids)
	if err != nil {
		return nil, err
	}
	var f *ConfigForm
	switch section {
	case SectionWiFi1, SectionWiFi2:
		f = m.wifiForm(ctx, ts, section)
	case SectionLogin:
		f = m.loginForm(ctx, ts)
	case SectionMQTT:
		f = m.mqttForm(ctx, ts)
	case SectionOthers:
		f = m.othersForm(ctx, ts)
	default:
		return nil, invalid("section")
	}
	if len(f.Excluded) == len(ts) {
		return nil, ErrAllExcluded
	}
	return f, nil
}

// ConfigApply applies one section to the selected devices. body is the
// section's *Apply struct.
func (m *Devices) ConfigApply(ctx context.Context, ids []string, section string, body any) ([]ResultLine, error) {
	ts, err := m.targets(ids)
	if err != nil {
		return nil, err
	}
	var out []ResultLine
	switch a := body.(type) {
	case WiFiApply:
		if section != SectionWiFi1 && section != SectionWiFi2 {
			return nil, invalid("section")
		}
		out, err = m.applyWiFi(ctx, ts, section, a)
	case LoginApply:
		out, err = m.applyLogin(ctx, ts, a)
	case MQTTApply:
		out, err = m.applyMQTT(ctx, ts, a)
	case OthersApply:
		out, err = m.applyOthers(ctx, ts, a)
	default:
		return nil, invalid("section")
	}
	if err == nil {
		for _, t := range ts {
			if t.usable() {
				m.poke(t.e) // the table shows the new cloud/MQTT state at once
			}
		}
	}
	return out, err
}
