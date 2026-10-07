package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/sim"
	"github.com/wimmme/shellylanman/internal/store"
)

const g1Sta = `{"enabled":true,"ssid":"Home","ipv4_method":"dhcp","ip":"192.168.1.9","gw":"192.168.1.1","mask":"255.255.255.0","dns":""}`

func plugS(t *testing.T, m *Devices, ctx context.Context, over map[string]string, id string) *sim.Device {
	t.Helper()
	if over == nil {
		over = map[string]string{}
	}
	if _, ok := over["shelly.json"]; !ok && id != "AABBCC000001" {
		over["shelly.json"] = `{"type":"SHPLG-S","mac":"` + id + `","auth":false,"fw":"20230913-113421/v1.14.0-gcb84623"}`
	}
	over["settings_sta.json"] = g1Sta
	over["settings_login.json"] = `{"enabled":false,"unprotected":false,"username":"admin"}`
	d, addr := startSimDev(t, fixtureDir(t, "gen1/SHPLG-S", over), nil)
	m.handle(ctx, addr, "shellyplug-s-"+strings.ToLower(id), true)
	waitDevice(t, m, id, online)
	return d
}

func plus1(t *testing.T, m *Devices, ctx context.Context, over map[string]string, configure func(*sim.Device)) *sim.Device {
	t.Helper()
	if over == nil {
		over = map[string]string{}
	}
	over["rpc_WiFi.GetConfig.json"] = `{"sta":{"ssid":"Home","enable":true,"ipv4mode":"dhcp","ip":null,"netmask":null,"gw":null,"nameserver":null},"sta1":{"enable":false,"ipv4mode":"dhcp"},"ap":{"enable":false}}`
	over["rpc_MQTT.GetConfig.json"] = `{"enable":true,"server":"broker:1883","user":"","topic_prefix":"shellyplus1-aabbcc000002","enable_control":true,"enable_rpc":true,"rpc_ntf":true,"status_ntf":false}`
	over["shelly.json"] = `{"id":"shellyplus1-aabbcc000002","mac":"AABBCC000002","model":"SNSW-001X16EU","gen":2,"app":"Plus1","auth_en":false}`
	d, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", over), configure)
	m.handle(ctx, addr, "shellyplus1-aabbcc000002", true)
	waitDevice(t, m, "AABBCC000002", online)
	return d
}

func TestWiFiFormAndApply(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	g2 := plus1(t, m, ctx, nil, nil)

	f, err := m.ConfigForm(ctx, []string{"AABBCC000001"}, SectionWiFi1)
	if err != nil || !f.WiFi.Enabled || f.WiFi.SSID != "Home" || *f.WiFi.Static || f.WiFi.IP != "192.168.1.9" {
		t.Fatalf("form %+v %v", f, err)
	}
	both := []string{"AABBCC000001", "AABBCC000002"}
	f, _ = m.ConfigForm(ctx, both, SectionWiFi1)
	if f.WiFi.SSID != "Home" || f.WiFi.IP != "" || f.WiFi.Static == nil || *f.WiFi.Static {
		t.Fatalf("merged form %+v", f.WiFi)
	}

	a := WiFiApply{Enabled: true, SSID: "Net", Password: "pw", Mode: "static", IP: "192.168.1.5", Netmask: "255.255.255.0", Gateway: "192.168.1.1"}
	if _, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionWiFi1, a); !errors.Is(err, ErrConfirm) {
		t.Fatalf("no confirmation: %v", err)
	}
	bad := a
	bad.IP, bad.Confirm = "300.1.1.1", true
	if _, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionWiFi1, bad); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "wrongIP") {
		t.Fatalf("validation: %v", err)
	}
	a.Confirm = true
	res, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionWiFi1, a)
	if err != nil || len(res) != 1 || res[0].Result != ResultOK {
		t.Fatalf("apply %+v %v", res, err)
	}
	if !hasCall(g1, "GET /settings/sta?enabled=true&ssid=Net&key=pw&ipv4_method=static&ip=192.168.1.5&netmask=255.255.255.0&gateway=192.168.1.1") {
		t.Fatalf("G1 calls %v", g1.Calls())
	}
	// Two devices: the IP field is not used, each device keeps its own.
	a.IP = ""
	if _, err := m.ConfigApply(ctx, both, SectionWiFi1, a); err != nil {
		t.Fatal(err)
	}
	if !hasCall(g1, "GET /settings/sta?enabled=true&ssid=Net&key=pw&ipv4_method=static&ip=192.168.1.9&netmask=255.255.255.0&gateway=192.168.1.1") {
		t.Fatalf("G1 keeps its IP: %v", g1.Calls())
	}
	if !hasCall(g2, `RPC WiFi.SetConfig {"config":{"sta":{"enable":true,"gw":"192.168.1.1","ip":"","ipv4mode":"static","nameserver":null,"netmask":"255.255.255.0","pass":"pw","ssid":"Net"}}}`) {
		t.Fatalf("G2 calls %v", g2.Calls())
	}
	dhcp := WiFiApply{Enabled: true, SSID: "Net", Password: "pw", Mode: "dhcp", Confirm: true}
	if _, err := m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionWiFi2, dhcp); err != nil ||
		!hasCall(g2, `RPC WiFi.SetConfig {"config":{"sta1":{"enable":true,"ipv4mode":"dhcp","pass":"pw","ssid":"Net"}}}`) {
		t.Fatalf("G2 dhcp sta1: %v %v", err, g2.Calls())
	}
}

func TestLoginApplyKeepsCredentials(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, nil, nil)
	f, err := m.ConfigForm(ctx, []string{"AABBCC000002"}, SectionLogin)
	if err != nil || f.Login.Enabled || f.Login.User != "admin" || f.Variant != "g2" {
		t.Fatalf("form %+v %v", f, err)
	}
	if _, err := m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionLogin, LoginApply{Enabled: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("password required: %v", err)
	}
	res, err := m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionLogin, LoginApply{Enabled: true, Password: "s3cret"})
	if err != nil || res[0].Result != ResultOK {
		t.Fatalf("apply %+v %v", res, err)
	}
	h := sha256.Sum256([]byte("admin:shellyplus1-aabbcc000002:s3cret"))
	if !hasCall(g2, `RPC Shelly.SetAuth {"ha1":"`+hex.EncodeToString(h[:])+`","realm":"shellyplus1-aabbcc000002","user":"admin"}`) {
		t.Fatalf("calls %v", g2.Calls())
	}
	if c := m.credentialsFor("AABBCC000002"); c == nil || c.Password != "s3cret" {
		t.Fatalf("credentials not kept: %+v", c)
	}
	if _, err := m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionLogin, LoginApply{Enabled: false}); err != nil ||
		!hasCall(g2, `RPC Shelly.SetAuth {"ha1":null,"realm":"shellyplus1-aabbcc000002","user":"admin"}`) {
		t.Fatalf("disable: %v %v", err, g2.Calls())
	}
	if c := m.credentialsFor("AABBCC000002"); c != nil {
		t.Fatalf("credentials kept after disable: %+v", c)
	}
}

func TestMQTTVariants(t *testing.T) {
	m, _, ctx := newService(t, func(s *store.Settings) { s.MQTTSlow = 0 })
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	g2 := plus1(t, m, ctx, nil, nil)

	f, _ := m.ConfigForm(ctx, []string{"AABBCC000001"}, SectionMQTT)
	if f.Variant != "g1" || f.MQTT.KeepAlive == nil || f.MQTT.Control != nil {
		t.Fatalf("G1 form %+v %+v", f, f.MQTT)
	}
	f, _ = m.ConfigForm(ctx, []string{"AABBCC000002"}, SectionMQTT)
	if f.Variant != "g2" || f.MQTT.Control == nil || !*f.MQTT.Control || f.MQTT.KeepAlive != nil || !f.MQTT.NoPwd {
		t.Fatalf("G2 form %+v %+v", f, f.MQTT)
	}
	f, _ = m.ConfigForm(ctx, []string{"AABBCC000001", "AABBCC000002"}, SectionMQTT)
	if f.Variant != "mix" || f.MQTT.Control != nil || f.MQTT.KeepAlive != nil {
		t.Fatalf("mixed form %+v %+v", f, f.MQTT)
	}

	if _, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionMQTT, MQTTApply{Enabled: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("server required: %v", err)
	}
	ka, qos := 60, 1
	_, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionMQTT, MQTTApply{Enabled: true, Server: "b:1883", NoPassword: true, DefaultPrefix: true, KeepAlive: &ka, QoS: &qos})
	if err != nil || !hasCall(g1, "GET /settings?mqtt_enable=true&mqtt_server=b%3A1883&mqtt_user=&mqtt_pass=&mqtt_id=&mqtt_keep_alive=60&mqtt_max_qos=1") {
		t.Fatalf("G1 panel: %v %v", err, g1.Calls())
	}
	on := false
	_, err = m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionMQTT, MQTTApply{Enabled: true, Server: "b:1883", User: "u", Password: "p", Prefix: "house", Control: &on})
	if err != nil || !hasCall(g2, `RPC MQTT.SetConfig {"config":{"enable":true,"enable_control":false,"pass":"p","server":"b:1883","topic_prefix":"house","user":"u"}}`) {
		t.Fatalf("G2 panel: %v %v", err, g2.Calls())
	}
	// Mixed selection: the short form, and with two devices the prefix is not changed.
	_, err = m.ConfigApply(ctx, []string{"AABBCC000001", "AABBCC000002"}, SectionMQTT, MQTTApply{Enabled: false})
	if err != nil || !hasCall(g1, "GET /settings?mqtt_enable=false") || !hasCall(g2, `RPC MQTT.SetConfig {"config":{"enable":false}}`) {
		t.Fatalf("disable both: %v %v %v", err, g1.Calls(), g2.Calls())
	}
}

func TestDeferredTaskRunsWhenBackOnline(t *testing.T) {
	m, st, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, nil, nil)
	m.mu.Lock()
	e := m.devs["AABBCC000002"]
	e.paused = true // keep the poll from bringing it back on line by itself
	m.mu.Unlock()
	m.setStatus(e, model.StatusOffline)

	res, err := m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionMQTT, MQTTApply{Enabled: true, Server: "b:1883", User: "u", Password: "very-secret-pw", DefaultPrefix: true})
	if err != nil || res[0].Result != ResultQueued {
		t.Fatalf("queued: %+v %v", res, err)
	}
	list := m.Deferred()
	if len(list) != 1 || list[0].Status != DefWaiting || list[0].Description != "mqttEnable" || list[0].Sealed != "" {
		t.Fatalf("deferred %+v", list)
	}
	b, _ := os.ReadFile(filepath.Join(st.Dir(), store.DeferredFile))
	if !strings.Contains(string(b), `"WAITING"`) || strings.Contains(string(b), "very-secret-pw") {
		t.Fatalf("deferred.json must exist without the password: %s", b)
	}
	// A second request of the same type replaces the waiting one.
	m.ConfigApply(ctx, []string{"AABBCC000002"}, SectionMQTT, MQTTApply{Enabled: false})
	if l := m.Deferred(); len(l) != 2 || l[0].Status != DefCancelled || l[1].Status != DefWaiting {
		t.Fatalf("replace: %+v", l)
	}
	m.setStatus(e, model.StatusOnline)
	deadline := time.Now().Add(3 * time.Second)
	for m.Deferred()[1].Status != DefSuccess && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if l := m.Deferred(); l[1].Status != DefSuccess || !hasCall(g2, `RPC MQTT.SetConfig {"config":{"enable":false}}`) {
		t.Fatalf("not run: %+v %v", l, g2.Calls())
	}
	if err := m.CancelDeferred(m.Deferred()[1].ID); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("cancel a finished task: %v", err)
	}
}

func TestOthers(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	g2 := plus1(t, m, ctx, nil, nil)
	f, err := m.ConfigForm(ctx, []string{"AABBCC000001"}, SectionOthers)
	if err != nil || f.Others.Cloud == nil || *f.Others.Cloud || f.Others.Reset != nil {
		t.Fatalf("form %+v %v", f.Others, err)
	}
	if _, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionOthers, OthersApply{Part: "ntp"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ntp required: %v", err)
	}
	both := []string{"AABBCC000001", "AABBCC000002"}
	if _, err := m.ConfigApply(ctx, both, SectionOthers, OthersApply{Part: "ntp", NTP: "pool.ntp.org"}); err != nil ||
		!hasCall(g1, "GET /settings?sntp_server=pool.ntp.org") || !hasCall(g2, `RPC Sys.SetConfig {"config":{"sntp":{"server":"pool.ntp.org"}}}`) {
		t.Fatalf("ntp: %v %v %v", err, g1.Calls(), g2.Calls())
	}
	yes := true
	if _, err := m.ConfigApply(ctx, both, SectionOthers, OthersApply{Part: "cloud", Enable: &yes}); err != nil ||
		!hasCall(g1, "GET /settings/cloud?enabled=true") || !hasCall(g2, `RPC Cloud.SetConfig {"config":{"enable":true}}`) {
		t.Fatalf("cloud: %v %v %v", err, g1.Calls(), g2.Calls())
	}
	res, err := m.ConfigApply(ctx, []string{"AABBCC000001"}, SectionOthers, OthersApply{Part: "reset", Enable: &yes})
	if err != nil || res[0].Result != ResultFail || res[0].Message != "notApplicable" {
		t.Fatalf("reset on a plug: %+v %v", res, err)
	}
}

func TestChecklist(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	plus1(t, m, ctx, nil, nil)
	rows := m.Checklist(ctx, []string{"AABBCC000001", "AABBCC000002"})
	if len(rows) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	r := rows[0]
	if r.Eco != false || r.LED != false || r.Logs != false || r.Roaming != "-85" || r.WiFi1 != FalseStr || r.WiFi2 != NAStr || r.AP != NAStr || r.AutoFW != NAStr {
		t.Fatalf("G1 row %+v", r)
	}
	g := rows[1]
	if g.Scripts != "0 / 0" || g.AutoFW != FalseStr || g.LED != NAStr {
		t.Fatalf("G2 row %+v", g)
	}
	lines, rows, err := m.ChecklistApply(ctx, ChecklistAction{IDs: []string{"AABBCC000001"}, Action: CheckEco, Value: true})
	if err != nil || len(lines) != 0 || !hasCall(g1, "GET /settings?eco_mode_enabled=true") || len(rows) != 1 {
		t.Fatalf("eco: %v %+v %v", err, lines, g1.Calls())
	}
	if d, _ := m.Get("AABBCC000001"); !d.RebootRequired {
		t.Fatal("Gen1 eco mode change must mark the device reboot required")
	}
	// Gen1: no (the original shows "-"; switching the AP on took a Gen1 plug off the LAN, DECISIONS P20-10).
	lines, _, _ = m.ChecklistApply(ctx, ChecklistAction{IDs: []string{"AABBCC000001"}, Action: CheckAP, Value: true})
	if len(lines) != 1 || lines[0].Result != ResultFail || hasCall(g1, "GET /settings/ap?enabled=true") {
		t.Fatalf("AP on Gen1: %+v %v", lines, g1.Calls())
	}
}

func TestAutoFirmwareUpdateSchedule(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, map[string]string{
		"rpc_Schedule.List.json": `{"jobs":[{"id":3,"enable":true,"timespec":"0 0 0 * * *","calls":[{"method":"Shelly.Update","params":{"stage":"beta"}}]}]}`,
	}, nil)
	rows := m.Checklist(ctx, []string{"AABBCC000002"})
	if rows[0].AutoFW != "beta" {
		t.Fatalf("auto FW %+v", rows[0].AutoFW)
	}
	if lines, _, err := m.ChecklistApply(ctx, ChecklistAction{IDs: []string{"AABBCC000002"}, Action: CheckAutoFW, Mode: "stable"}); err != nil || len(lines) != 0 {
		t.Fatalf("stable: %v %+v", err, lines)
	}
	if !hasCall(g2, `RPC Schedule.Delete {"id":3}`) ||
		!hasCall(g2, `RPC Schedule.Create {"calls":[{"method":"Shelly.Update","origin":"shelly_service","params":{"stage":"stable"}}],"enable":true,"timespec":"0 0 0 * * 0,1,2,3,4,5,6"}`) {
		t.Fatalf("calls %v", g2.Calls())
	}
	_ = shelly.DigestUser
}
