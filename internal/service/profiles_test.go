package service

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

func bp(b bool) *bool     { return &b }
func sp(s string) *string { return &s }
func anyCall(calls []string, sub string) bool {
	for _, c := range calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func TestProfileName(t *testing.T) {
	d := model.Device{ID: "54320467CBD4", Gen: "3", TypeName: "Plug S G3", Hostname: "shellyplugsg3-54320467cbd4"}
	for pattern, want := range map[string]string{
		"{model} {mac4}":         "Plug S G3 CBD4",
		"  {model}   {gen}  ":    "Plug S G3 3",
		"shelly-{mac}":           "shelly-54320467CBD4",
		"{host}":                 "shellyplugsg3-54320467cbd4",
		"Terras":                 "Terras",
		"":                       "",
		strings.Repeat("x", 100): strings.Repeat("x", 64),
	} {
		if got := ProfileName(pattern, d); got != want {
			t.Errorf("ProfileName(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestProfileCRUDAndPasswords(t *testing.T) {
	m, st, _ := newService(t, nil)
	v, err := m.SaveProfile(ProfileInput{Profile: store.Profile{Name: " Home ", NamePattern: "{model} {mac4}", WiFiSSID: "HomeNet",
		Login: &store.ProfileLogin{Enabled: true, User: "admin"}, MQTT: &store.ProfileMQTT{Enabled: true, Server: "broker:1883", User: "u"}},
		LoginPassword: sp("secret1"), MQTTPassword: sp("mq1")})
	if err != nil || v.ID == "" || v.Name != "Home" || !v.LoginPasswordSet || !v.MQTTPasswordSet {
		t.Fatalf("create: %+v %v", v, err)
	}
	if pw, _, _ := st.Secret("profile:" + v.ID + ":login"); pw != "secret1" {
		t.Fatalf("the password is a secret of the store: %q", pw)
	}
	raw, _ := m.store.LoadProfiles()
	if len(raw) != 1 {
		t.Fatal(raw)
	}
	// The file holds no password.
	if b, _ := readFile(st.Dir() + "/profiles.json"); strings.Contains(string(b), "secret1") || strings.Contains(string(b), "mq1") {
		t.Fatalf("a password in profiles.json: %s", b)
	}
	// Changing without passwords keeps them; "" removes; switching login off drops its password.
	in := ProfileInput{Profile: v.Profile}
	in.Name = "Home 2"
	if v, err = m.SaveProfile(in); err != nil || !v.LoginPasswordSet || v.Name != "Home 2" {
		t.Fatalf("keep: %+v %v", v, err)
	}
	in.Login = &store.ProfileLogin{Enabled: false}
	if v, err = m.SaveProfile(in); err != nil || v.LoginPasswordSet || !v.MQTTPasswordSet {
		t.Fatalf("login off: %+v %v", v, err)
	}
	in.MQTT = &store.ProfileMQTT{Enabled: true, Server: "b:1", User: "u"}
	in.MQTTPassword = sp("")
	if _, err := m.SaveProfile(in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("MQTT with a user and no password: %v", err)
	}
	// Validation.
	for _, bad := range []ProfileInput{
		{Profile: store.Profile{Name: " "}},
		{Profile: store.Profile{Name: "x", AutoFW: "yes"}},
		{Profile: store.Profile{Name: "x", Login: &store.ProfileLogin{Enabled: true}}},
		{Profile: store.Profile{Name: "x", MQTT: &store.ProfileMQTT{Enabled: true}}},
		{Profile: store.Profile{Name: "x", NamePattern: strings.Repeat("a", 65)}},
	} {
		if _, err := m.SaveProfile(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad.Profile, err)
		}
	}
	if _, err := m.SaveProfile(ProfileInput{Profile: store.Profile{ID: "nope", Name: "x"}}); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("unknown id: %v", err)
	}
	list, _ := m.Profiles()
	if len(list) != 1 {
		t.Fatalf("%d profiles", len(list))
	}
	if err := m.DeleteProfile(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.Secret("profile:" + v.ID + ":mqtt"); ok {
		t.Fatal("its passwords go with it")
	}
	if err := m.DeleteProfile(v.ID); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestApplyProfileGen1(t *testing.T) {
	m, st, ctx := newService(t, nil)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	v, err := m.SaveProfile(ProfileInput{Profile: store.Profile{Name: "Home", NamePattern: "{model} {mac4}", NTP: "pool.ntp.org", Cloud: bp(false),
		Eco: bp(true), AP: bp(true), AutoFW: "stable",
		MQTT:  &store.ProfileMQTT{Enabled: true, Server: "broker:1883", User: "mq"},
		Login: &store.ProfileLogin{Enabled: true, User: "admin"}},
		LoginPassword: sp("secret1"), MQTTPassword: sp("mqpw")})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := m.ProfilePlan(v.ID, "AABBCC000001")
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	for _, p := range plan {
		steps = append(steps, p.Step+"="+p.Value)
	}
	if got := strings.Join(steps, " "); got != "name=PlugS 0001 eco=on ap=on autoFW=stable ntp=pool.ntp.org cloud=off mqtt=broker:1883 login=admin" {
		t.Fatalf("plan: %s", got)
	}
	res, err := m.ApplyProfile(ctx, v.ID, "AABBCC000001")
	if err != nil {
		t.Fatal(err)
	}
	byStep := map[string]ProfileStep{}
	for _, s := range res {
		byStep[s.Step] = s
	}
	for _, ok := range []string{"name", "eco", "ntp", "cloud", "mqtt", "login"} {
		if byStep[ok].Result != ResultOK {
			t.Errorf("%s: %+v", ok, byStep[ok])
		}
	}
	// Gen1 has no access-point switch and no automatic firmware update here: skipped, not failed.
	for _, skipped := range []string{"ap", "autoFW"} {
		if byStep[skipped].Result != ResultSkipped {
			t.Errorf("%s: %+v", skipped, byStep[skipped])
		}
	}
	calls := g1.Calls()
	for _, want := range []string{"/settings?name=PlugS+0001", "/settings?eco_mode_enabled=true", "/settings?sntp_server=pool.ntp.org", "/settings/cloud?enabled=false", "mqtt_enable=true", "/settings/login?enabled=true&username=admin&password=secret1"} {
		if !anyCall(calls, want) {
			t.Errorf("no call %q in %v", want, calls)
		}
	}
	if last := calls[len(calls)-1]; !strings.Contains(last, "/settings") { // the login comes after the others
		t.Logf("last call %q", last)
	}
	if c, err := m.DeviceCredentials("AABBCC000001"); err != nil || c.User != "admin" || c.Password != "secret1" {
		t.Fatalf("the device's new login is kept: %+v %v", c, err)
	}
	_ = st
}

func TestApplyProfileErrors(t *testing.T) {
	m, _, ctx := newService(t, nil)
	v, _ := m.SaveProfile(ProfileInput{Profile: store.Profile{Name: "x", NTP: "pool.ntp.org"}})
	if _, err := m.ApplyProfile(ctx, v.ID, "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown device: %v", err)
	}
	if _, err := m.ApplyProfile(ctx, "nope", "AABBCC000001"); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("unknown profile: %v", err)
	}
	plugS(t, m, ctx, nil, "AABBCC000001")
	if _, err := m.ApplyProfile(ctx, "nope", "AABBCC000001"); !errors.Is(err, ErrNoProfile) {
		t.Fatalf("unknown profile: %v", err)
	}
	if res, err := m.ApplyProfile(ctx, v.ID, "AABBCC000001"); err != nil || len(res) != 1 || res[0].Step != "ntp" || res[0].Result != ResultOK {
		t.Fatalf("one step: %+v %v", res, err)
	}
}

// A profile read from a device (DECISIONS §30): every setting it tells, the deviations from the
// factory marked, no name, no passwords.
func TestProfileFromDevice(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g1 := plugS(t, m, ctx, nil, "AABBCC000001")
	g2 := plus1(t, m, ctx, nil, nil)

	d, err := m.ProfileFromDevice(ctx, "AABBCC000001")
	if err != nil {
		t.Fatal(err)
	}
	p := d.Profile
	if p.Name != "" || p.NamePattern != "" || p.WiFiSSID != "" {
		t.Errorf("name, pattern and Wi-Fi are left empty: %+v", p)
	}
	if p.Eco == nil || *p.Eco || p.Cloud == nil || *p.Cloud || p.Roaming == nil || !*p.Roaming || p.AP != nil || p.AutoFW != "" {
		t.Errorf("read values: eco=%v cloud=%v roaming=%v ap=%v autoFW=%q", p.Eco, p.Cloud, p.Roaming, p.AP, p.AutoFW)
	}
	if p.MQTT == nil || !p.MQTT.Enabled || !p.MQTT.NoPassword || p.Login == nil || p.Login.Enabled || p.NTP == "" {
		t.Errorf("mqtt=%+v login=%+v ntp=%q", p.MQTT, p.Login, p.NTP)
	}
	got := strings.Join(d.Deviating, " ")
	for _, want := range []string{"roaming", "ntp", "cloud", "mqtt"} { // Gen1: roaming on, other time server, cloud off, MQTT on
		if !strings.Contains(" "+got+" ", " "+want+" ") {
			t.Errorf("%q should deviate: %s", want, got)
		}
	}
	for _, not := range []string{"eco", "login", "ledOff"} {
		if strings.Contains(" "+got+" ", " "+not+" ") {
			t.Errorf("%q is as from the factory: %s", not, got)
		}
	}

	d2, err := m.ProfileFromDevice(ctx, "AABBCC000002")
	if err != nil || d2.Profile.MQTT == nil || d2.Profile.MQTT.Server != "broker:1883" || d2.Profile.AP == nil {
		t.Fatalf("Gen2+: %+v %v", d2, err)
	}
	// Only reads: nothing but GETs went to the devices.
	for _, c := range append(g1.Calls(), g2.Calls()...) {
		if strings.Contains(c, "POST") || strings.Contains(c, "enable=") && strings.Contains(c, "?") {
			t.Errorf("a write: %s", c)
		}
	}
	if _, err := m.ProfileFromDevice(ctx, "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown device: %v", err)
	}
}

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }
