package sbk

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/sim"
)

func simDir(t *testing.T, rel string, over map[string]string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", rel)
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(src, e.Name()))
		os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644)
	}
	for n, c := range over {
		os.WriteFile(filepath.Join(dst, n), []byte(c), 0o644)
	}
	return dst
}

func startSim(t *testing.T, dir string) (*sim.Device, string) {
	t.Helper()
	d, err := sim.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	return d, strings.TrimPrefix(srv.URL, "http://")
}

func calls(d *sim.Device, prefix string) []string {
	var out []string
	for _, c := range d.Calls() {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func has(list []string, want string) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

func g1Plug(t *testing.T) (*sim.Device, *Device) {
	s, addr := startSim(t, simDir(t, "gen1/SHPLG-S", nil))
	return s, &Device{Conn: shelly.NewClient().Conn(addr, true), Gen: "1", TypeID: "SHPLG-S", Hostname: "shellyplug-s-AABBCC000001", MAC: "AABBCC000001", SSID: "REDACTED"}
}

func TestGen1BackupAndRestore(t *testing.T) {
	ctx := context.Background()
	s, d := g1Plug(t)
	data, stored, err := Backup(ctx, d)
	if err != nil || stored {
		t.Fatal(err, stored)
	}
	f, err := Read(data)
	if err != nil || !f["settings.json"].Exists() || !f["actions.json"].Exists() {
		t.Fatalf("backup content %v %v", err, f)
	}
	chk := CheckRestore(ctx, d, f)
	if len(chk.Items()) != 0 {
		t.Fatalf("same device, no login: nothing to ask, got %+v", chk.Items())
	}
	res := Restore(ctx, d, f, Answers{})
	if p := Problems(res); len(p) != 0 {
		t.Fatalf("problems %v", p)
	}
	gets := calls(s, "GET /settings")
	if !has(gets, "GET /settings?led_status_disable=false&led_power_disable=false&wifirecovery_reboot_enabled=") {
		t.Fatalf("model settings not sent: %v", gets)
	}
	var relay, cloud, common, login, mqtt, roam string
	for _, c := range gets {
		switch {
		case strings.HasPrefix(c, "GET /settings/relay/0?"):
			relay = c
		case strings.HasPrefix(c, "GET /settings/cloud?"):
			cloud = c
		case strings.HasPrefix(c, "GET /settings?name="):
			common = c
		case strings.HasPrefix(c, "GET /settings/login?"):
			login = c
		case strings.HasPrefix(c, "GET /settings?mqtt_enable="):
			mqtt = c
		case strings.HasPrefix(c, "GET /settings?ap_roaming_enabled="):
			roam = c
		}
	}
	if strings.Contains(relay, "ison=") || !strings.Contains(relay, "default_state=") {
		t.Fatalf("relay restore without state fields, in backup order: %q", relay)
	}
	if cloud != "GET /settings/cloud?enabled=false" || !strings.Contains(common, "&tz_dst_auto=") || !strings.Contains(common, "&coiot_enable=") || !strings.Contains(common, "&sntp_server=") {
		t.Fatalf("commons: %q %q", cloud, common)
	}
	if login != "GET /settings/login?enabled=false" || !strings.HasPrefix(mqtt, "GET /settings?mqtt_enable=true&mqtt_server=") || roam != "GET /settings?ap_roaming_enabled=true&ap_roaming_threshold=-85" {
		t.Fatalf("login %q mqtt %q roam %q", login, mqtt, roam)
	}
	for _, c := range gets {
		if strings.HasPrefix(c, "GET /settings/sta?") {
			t.Fatalf("the network in use must not be touched without a password: %q", c)
		}
	}
	if !has(gets, "GET /settings/sta1?enabled=false") { // disabled in the backup: disabled again
		t.Fatalf("Wi-Fi 2: %v", gets)
	}
	if !has(gets, "GET /settings/actions?name=btn_on_url&enabled=false&index=0&urls[]=") && len(calls(s, "GET /settings/actions?name=")) == 0 {
		t.Fatalf("actions not restored: %v", gets)
	}
}

func TestGen1CheckMessages(t *testing.T) {
	ctx := context.Background()
	_, d := g1Plug(t)
	data, _, _ := Backup(ctx, d)
	f, _ := Read(data)
	s := f["settings.json"]
	s.Get("device").Set("hostname", ojson.Str("shellyplug-s-OTHER"))
	s.Get("login").Set("enabled", ojson.BoolV(true)).Set("username", ojson.Str("admin"))
	s.Get("mqtt").Set("user", ojson.Str("broker-user"))
	chk := CheckRestore(ctx, d, f)
	var keys []string
	for _, it := range chk.Items() {
		keys = append(keys, it.Key+"="+it.Value)
	}
	if strings.Join(keys, " ") != "PRE_QUESTION_RESTORE_HOST=shellyplug-s-OTHER RESTORE_LOGIN=admin RESTORE_MQTT=broker-user" {
		t.Fatalf("check %v", keys)
	}
	s.Get("device").Set("type", ojson.Str("SHSW-1"))
	if chk := CheckRestore(ctx, d, f); chk.Items()[0].Key != ErrModel {
		t.Fatalf("other model: %+v", chk.Items())
	}
}

func plus1(t *testing.T, over map[string]string) (*sim.Device, *Device) {
	if over == nil {
		over = map[string]string{}
	}
	if _, ok := over["rpc_Shelly.GetDeviceInfo.json"]; !ok {
		over["rpc_Shelly.GetDeviceInfo.json"] = `{"name":"Test","id":"shellyplus1-aabbcc000001","mac":"AABBCC000001","model":"SNSW-001X16EU","gen":2,"app":"Plus1","ver":"1.7.5","auth_en":false,"profile":null}`
	}
	s, addr := startSim(t, simDir(t, "gen2/Plus1", over))
	return s, &Device{Conn: shelly.NewClient().Conn(addr, false), Gen: "2", TypeID: "Plus1", App: "Plus1", Model: "SNSW-001X16EU",
		Hostname: "shellyplus1-aabbcc000001", MAC: "AABBCC000001", SSID: "REDACTED"}
}

func TestGen2BackupAndRestore(t *testing.T) {
	ctx := context.Background()
	s, d := plus1(t, map[string]string{
		"rpc_Script.List.json":    `{"scripts":[{"id":1,"name":"blink","enable":true}]}`,
		"rpc_Script.GetCode.json": `{"data":"let a = 1;\r\nprint(a);"}`,
		"rpc_Schedule.List.json":  `{"jobs":[{"id":4,"enable":true,"timespec":"0 0 7 * * *","calls":[{"method":"Switch.Set","params":{"id":0,"on":true}}]}]}`,
		"rpc_KVS.GetMany.json":    `{"items":[{"key":"k","etag":"e","value":"v"}],"offset":0,"total":1}`,
		"rpc_Webhook.List.json":   `{"hooks":[{"id":7,"cid":0,"enable":true,"event":"input.button_push","name":"push","urls":["http://x"]}]}`,
	})
	data, _, err := Backup(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Shelly.GetDeviceInfo.json", "Shelly.GetConfig.json", "Schedule.List.json", "Webhook.List.json", "KVS.GetMany.json", "Script.List.json", "blink.mjs.json"} {
		if !f[n].Exists() {
			t.Fatalf("missing %s in %v", n, f)
		}
	}
	if f["blink.mjs.json"].Get("code").Text() != "let a = 1;\nprint(a);" {
		t.Fatalf("script code %q", f["blink.mjs.json"].Get("code").Text())
	}
	chk := CheckRestore(ctx, d, f)
	var keys []string
	for _, it := range chk.Items() {
		keys = append(keys, it.Key)
	}
	// Same device; the script "blink" exists on the device (the simulator lists it) and was enabled.
	if strings.Join(keys, " ") != QScriptsOverride+" "+QScriptsEnable {
		t.Fatalf("check %v", keys)
	}
	res := Restore(ctx, d, f, Answers{QScriptsOverride: "true", QScriptsEnable: "true"})
	if p := Problems(res); len(p) != 0 {
		t.Fatalf("problems %v", p)
	}
	rpc := calls(s, "RPC ")
	for _, want := range []string{
		`RPC Input.SetConfig {"id":0,"config":`,
		`RPC Switch.SetConfig {"id":0,"config":`,
		`RPC BLE.SetConfig {"config":`,
		`RPC Cloud.SetConfig {"config":{"enable":`,
		`RPC Sys.SetConfig {"config":{"device":`,
		`RPC Schedule.DeleteAll {}`,
		`RPC Schedule.Create {"enable":true,"timespec":"0 0 7 * * *","calls":[{"method":"Switch.Set","params":{"id":0,"on":true}}]}`,
		`RPC Script.PutCode {"id":1,"code":"let a = 1;\nprint(a);"}`,
		`RPC Script.SetConfig {"id":1,"config":{"enable":true}}`,
		`RPC Webhook.DeleteAll {}`,
		`RPC Webhook.Create {"cid":0,"enable":true,"event":"input.button_push","name":"push","urls":["http://x"]}`,
		`RPC Shelly.SetAuth {"user":"admin","realm":"shellyplus1-aabbcc000001","ha1":null}`,
	} {
		found := false
		for _, c := range rpc {
			if strings.HasPrefix(c, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s in\n%s", want, strings.Join(rpc, "\n"))
		}
	}
	for _, c := range rpc {
		if strings.HasPrefix(c, "RPC KVS.Set") {
			t.Fatalf("KVS item equal to the device's must not be set: %s", c)
		}
		if strings.Contains(c, `"mac"`) || strings.Contains(c, `"fw_id"`) {
			t.Fatalf("device identity must not be restored: %s", c)
		}
	}
}

func TestGen2ProfileAndModelChecks(t *testing.T) {
	ctx := context.Background()
	_, d := plus1(t, nil)
	data, _, _ := Backup(ctx, d)
	f, _ := Read(data)
	other := &Device{Conn: d.Conn, Gen: "2", TypeID: "Plus1PM", App: "Plus1PM", Model: "SNSW-001P16EU", Hostname: d.Hostname, MAC: d.MAC}
	if c := CheckRestore(ctx, other, f); c.Items()[0].Key != ErrModel {
		t.Fatalf("different model: %+v", c.Items())
	}
	// Compatibility groups: a Shelly 1 G3 backup fits a Shelly 1 G4.
	f["Shelly.GetDeviceInfo.json"].Set("app", ojson.Str("S1G3")).Set("model", ojson.Str("S3SW-001X16EU"))
	g4 := &Device{Conn: d.Conn, Gen: "4", TypeID: "S4SW-001X16EU", App: "S1G4", Model: "S4SW-001X16EU", Hostname: d.Hostname, MAC: d.MAC}
	if c := CheckRestore(ctx, g4, f); len(c.Items()) > 0 && c.Items()[0].Key == ErrModel {
		t.Fatalf("G3 → G4 must be compatible: %+v", c.Items())
	}
	// 2PM: switch backup onto a cover device.
	s2, addr := startSim(t, simDir(t, "gen2/Plus1", map[string]string{
		"rpc_Shelly.GetConfig.json": `{"sys":{"device":{"profile":"cover"}},"cover:0":{"id":0}}`,
	}))
	_ = s2
	two := &Device{Conn: shelly.NewClient().Conn(addr, false), Gen: "2", TypeID: "Plus2PM", App: "Plus2PM", Hostname: "x", MAC: "AABBCC000009"}
	back := Files{"Shelly.GetDeviceInfo.json": ojson.MustParse(`{"app":"Plus2PM","mac":"AABBCC000009","id":"x","profile":"switch"}`),
		"Shelly.GetConfig.json": ojson.MustParse(`{"sys":{"device":{"profile":"switch"}},"wifi":{}}`)}
	c := CheckRestore(ctx, two, back)
	if _, ok := c[ErrModeCover]; !ok {
		t.Fatalf("mode check: %+v", c.Items())
	}
	res := Restore(ctx, two, back, Answers{})
	if p := Problems(res); len(p) == 0 || p[0] != ErrModeCover {
		t.Fatalf("restore must report the mode error first: %v", p)
	}
}

func TestReadScriptsAndFileName(t *testing.T) {
	z := newZip()
	z.addJSON("Shelly.GetConfig.json", ojson.MustParse(`{"a":1}`))
	z.add("my script.mjs", []byte("print(1)"))
	b, _ := z.bytes()
	f, err := Read(b)
	if err != nil || f["my script.mjs.json"].Get("code").Text() != "print(1)" || f["Shelly.GetConfig.json"].Get("a").Int() != 1 {
		t.Fatalf("read %v %v", err, f)
	}
	if _, err := Read([]byte("not a zip")); err != ErrNotBackup {
		t.Fatalf("not a zip: %v", err)
	}
	if FileName("shelly plug/1") != "shelly_plug_1.sbk" {
		t.Fatal(FileName("shelly plug/1"))
	}
	if p := Problems([]string{"", "->r_step:x", "a", "a", "b"}); strings.Join(p, ",") != "a,b" {
		t.Fatal(p)
	}
}

func TestCheckStored(t *testing.T) {
	keys := func(c Check) string {
		var s []string
		for _, it := range c.Items() {
			s = append(s, it.Key+"="+it.Value)
		}
		return strings.Join(s, ",")
	}
	g1 := Files{"settings.json": ojson.MustParse(`{"device":{"type":"SHPLG-S","hostname":"old"},"login":{"enabled":true,"username":"u"},"mqtt":{"enable":true,"user":"m"}}`)}
	d := &Device{Gen: "1", TypeID: "SHPLG-S", Hostname: "new"}
	if got := keys(CheckStored(d, g1)); got != "PRE_QUESTION_RESTORE_HOST=old,RESTORE_LOGIN=u,RESTORE_MQTT=m" {
		t.Fatalf("g1: %s", got)
	}
	g2 := Files{"Shelly.GetDeviceInfo.json": ojson.MustParse(`{"id":"h","app":"Plus1","auth_en":true}`), "Shelly.GetConfig.json": ojson.MustParse(`{"mqtt":{"enable":false}}`)}
	if got := keys(CheckStored(&Device{TypeID: "Plus1", Hostname: "h"}, g2)); got != "RESTORE_LOGIN=admin" {
		t.Fatalf("g2: %s", got)
	}
	if got := keys(CheckStored(&Device{TypeID: "Pro1"}, g2)); got != "ERR_RESTORE_MODEL=" {
		t.Fatalf("other model: %s", got)
	}
	blu := Files{bluFile: ojson.MustParse(`{"index":"200","type":"SBBT-002C","mac":"x"}`),
		"Shelly.GetComponents.json": ojson.MustParse(`{"components":[{"key":"bthomedevice:200","config":{"addr":"aa:bb"}}]}`)}
	if got := keys(CheckStored(&Device{TypeID: "SBBT-002C", MAC: "cc:dd"}, blu)); got != "PRE_QUESTION_RESTORE_HOST=SBBT-002C-aa:bb" {
		t.Fatalf("blu: %s", got)
	}
	if got := keys(CheckStored(d, Files{})); got != "ERR_RESTORE_MODEL=" {
		t.Fatalf("empty: %s", got)
	}
}
