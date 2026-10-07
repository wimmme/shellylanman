package service

// Profiles and their application to a new device (DECISIONS §29, docs/phase-20-ap-wizards.md §4):
// a profile is made once in ShellyLanMan; when a new Shelly has joined the network, the wizard
// shows what the profile would do and, after confirmation, ApplyProfile does it with the code
// that applies these settings to known devices (the settings dialogs and the checklist).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/store"
)

// ErrNoProfile: there is no such profile.
var ErrNoProfile = errors.New("no such profile")

const (
	secretProfilePfx = "profile:" // + id + ":login" | ":mqtt"
	maxNameLen       = 64
	maxProfiles      = 50
)

// ProfileInput is what the API takes: the profile and its passwords. A password that is
// absent (nil) is kept as it was; "" removes it.
type ProfileInput struct {
	store.Profile
	LoginPassword *string `json:"loginPassword,omitempty"`
	MQTTPassword  *string `json:"mqttPassword,omitempty"`
}

// ProfileView is what the API gives back: passwords are never returned, only whether they are set.
type ProfileView struct {
	store.Profile
	LoginPasswordSet bool `json:"loginPasswordSet"`
	MQTTPasswordSet  bool `json:"mqttPasswordSet"`
}

// PlanStep is one thing a profile would do to a device.
type PlanStep struct {
	Step  string `json:"step"`  // name, eco, ledOff, ap, roaming, autoFW, ntp, cloud, mqtt, login
	Value string `json:"value"` // what it would be set to ("on", "off", a name, a server …)
}

// ProfileStep is the outcome of one step.
type ProfileStep struct {
	Step    string `json:"step"`
	Result  string `json:"result"` // ok, fail, skipped (the device has no such setting)
	Message string `json:"message,omitempty"`
}

// ResultSkipped: the setting does not exist on this kind of device.
const ResultSkipped = "skipped"

func (m *Devices) viewOf(p store.Profile) ProfileView {
	v := ProfileView{Profile: p}
	if _, ok, _ := m.store.Secret(secretProfilePfx + p.ID + ":login"); ok {
		v.LoginPasswordSet = true
	}
	if _, ok, _ := m.store.Secret(secretProfilePfx + p.ID + ":mqtt"); ok {
		v.MQTTPasswordSet = true
	}
	return v
}

// Profiles lists the profiles, by name.
func (m *Devices) Profiles() ([]ProfileView, error) {
	m.profMu.Lock()
	defer m.profMu.Unlock()
	list, err := m.store.LoadProfiles()
	if err != nil {
		return nil, err
	}
	out := make([]ProfileView, 0, len(list))
	for _, p := range list {
		out = append(out, m.viewOf(p))
	}
	return out, nil
}

func (m *Devices) profile(id string) (store.Profile, error) {
	list, err := m.store.LoadProfiles()
	if err != nil {
		return store.Profile{}, err
	}
	for _, p := range list {
		if p.ID == id {
			return p, nil
		}
	}
	return store.Profile{}, fmt.Errorf("%w: %s", ErrNoProfile, id)
}

// validate checks a profile and cleans it (trimmed texts).
func (in *ProfileInput) validate(hadLoginPw, hadMQTTPw bool) error {
	p := &in.Profile
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return invalid("name")
	}
	p.NamePattern = strings.TrimSpace(p.NamePattern)
	if utf8.RuneCountInString(p.NamePattern) > maxNameLen {
		return invalid("namePattern")
	}
	p.WiFiSSID = strings.TrimSpace(p.WiFiSSID)
	p.NTP = strings.TrimSpace(p.NTP)
	switch p.AutoFW {
	case "", "stable", "beta", "none":
	default:
		return invalid("autoFW")
	}
	if l := p.Login; l != nil {
		l.User = strings.TrimSpace(l.User)
		pw := hadLoginPw
		if in.LoginPassword != nil {
			pw = *in.LoginPassword != ""
		}
		if l.Enabled && !pw {
			return invalid("loginPassword")
		}
	}
	if q := p.MQTT; q != nil {
		q.Server, q.User = strings.TrimSpace(q.Server), strings.TrimSpace(q.User)
		if q.Enabled && q.Server == "" {
			return invalid("mqttServer")
		}
		if q.Enabled && !q.NoPassword && q.User != "" {
			pw := hadMQTTPw
			if in.MQTTPassword != nil {
				pw = *in.MQTTPassword != ""
			}
			if !pw {
				return invalid("mqttPassword")
			}
		}
	}
	return nil
}

// SaveProfile creates a profile (empty id) or changes one.
func (m *Devices) SaveProfile(in ProfileInput) (ProfileView, error) {
	m.profMu.Lock()
	defer m.profMu.Unlock()
	list, err := m.store.LoadProfiles()
	if err != nil {
		return ProfileView{}, err
	}
	at := -1
	for i, p := range list {
		if p.ID == in.ID {
			at = i
		}
	}
	if in.ID != "" && at < 0 {
		return ProfileView{}, fmt.Errorf("%w: %s", ErrNoProfile, in.ID)
	}
	if in.ID == "" {
		if len(list) >= maxProfiles {
			return ProfileView{}, invalid("too many profiles")
		}
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			return ProfileView{}, err
		}
		in.ID = hex.EncodeToString(b)
	}
	_, hadL, _ := m.store.Secret(secretProfilePfx + in.ID + ":login")
	_, hadM, _ := m.store.Secret(secretProfilePfx + in.ID + ":mqtt")
	if err := in.validate(hadL, hadM); err != nil {
		return ProfileView{}, err
	}
	// Passwords: a switched-off login or MQTT keeps none; otherwise nil keeps, a value sets.
	set := func(name string, pw *string, wanted bool) error {
		switch {
		case !wanted:
			return m.store.SetSecret(name, "")
		case pw != nil:
			return m.store.SetSecret(name, *pw)
		}
		return nil
	}
	if err := set(secretProfilePfx+in.ID+":login", in.LoginPassword, in.Login != nil && in.Login.Enabled); err != nil {
		return ProfileView{}, err
	}
	if err := set(secretProfilePfx+in.ID+":mqtt", in.MQTTPassword, in.MQTT != nil && in.MQTT.Enabled && !in.MQTT.NoPassword && in.MQTT.User != ""); err != nil {
		return ProfileView{}, err
	}
	if at >= 0 {
		list[at] = in.Profile
	} else {
		list = append(list, in.Profile)
	}
	if err := m.store.SaveProfiles(list); err != nil {
		return ProfileView{}, err
	}
	return m.viewOf(in.Profile), nil
}

// DeleteProfile removes a profile and its passwords.
func (m *Devices) DeleteProfile(id string) error {
	m.profMu.Lock()
	defer m.profMu.Unlock()
	list, err := m.store.LoadProfiles()
	if err != nil {
		return err
	}
	for i, p := range list {
		if p.ID != id {
			continue
		}
		_ = m.store.SetSecret(secretProfilePfx+id+":login", "")
		_ = m.store.SetSecret(secretProfilePfx+id+":mqtt", "")
		return m.store.SaveProfiles(append(list[:i:i], list[i+1:]...))
	}
	return fmt.Errorf("%w: %s", ErrNoProfile, id)
}

// ProfileName is the name a device gets from a pattern: {model}, {mac} (12 digits), {mac4}
// (the last 4), {gen} and {host}, cleaned and cut to 64 characters.
func ProfileName(pattern string, d model.Device) string {
	mac := strings.ToUpper(d.ID)
	last4 := mac
	if len(mac) > 4 {
		last4 = mac[len(mac)-4:]
	}
	s := strings.NewReplacer("{model}", d.TypeName, "{mac}", mac, "{mac4}", last4, "{gen}", d.Gen, "{host}", d.Hostname).Replace(pattern)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxNameLen {
		s = strings.TrimSpace(string(r[:maxNameLen]))
	}
	return s
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// ProfilePlan lists what a profile would do to a device, in the order it does it.
func (m *Devices) ProfilePlan(profileID, deviceID string) ([]PlanStep, error) {
	m.profMu.Lock()
	p, err := m.profile(profileID)
	m.profMu.Unlock()
	if err != nil {
		return nil, err
	}
	e, err := m.entryFor(deviceID)
	if err != nil {
		return nil, err
	}
	d, _ := m.infoOf(e)
	return planOf(p, d), nil
}

func planOf(p store.Profile, d model.Device) []PlanStep {
	var out []PlanStep
	add := func(step, v string) { out = append(out, PlanStep{Step: step, Value: v}) }
	if p.NamePattern != "" {
		add("name", ProfileName(p.NamePattern, d))
	}
	if p.Eco != nil {
		add("eco", onOff(*p.Eco))
	}
	if p.LEDOff != nil {
		add("ledOff", onOff(*p.LEDOff))
	}
	if p.AP != nil {
		add("ap", onOff(*p.AP))
	}
	if p.Roaming != nil {
		add("roaming", onOff(*p.Roaming))
	}
	if p.AutoFW != "" {
		add("autoFW", p.AutoFW)
	}
	if p.NTP != "" {
		add("ntp", p.NTP)
	}
	if p.Cloud != nil {
		add("cloud", onOff(*p.Cloud))
	}
	if q := p.MQTT; q != nil {
		if q.Enabled {
			add("mqtt", q.Server)
		} else {
			add("mqtt", "off")
		}
	}
	if l := p.Login; l != nil {
		if l.Enabled {
			add("login", l.User)
		} else {
			add("login", "off")
		}
	}
	return out
}

// ApplyProfile sets a profile on a device that is on the network (confirmed by the caller). Every
// step is tried; a step the device has no setting for is skipped. The login comes last: once it is
// on, ShellyLanMan keeps the new login for the device (as the settings dialog does) and goes on with it.
func (m *Devices) ApplyProfile(ctx context.Context, profileID, deviceID string) ([]ProfileStep, error) {
	m.profMu.Lock()
	p, err := m.profile(profileID)
	m.profMu.Unlock()
	if err != nil {
		return nil, err
	}
	ts, err := m.targets([]string{deviceID})
	if err != nil {
		return nil, err
	}
	t := ts[0]
	if t.blu {
		return nil, fmt.Errorf("%w: a BLU device has no such settings", ErrBadCommand)
	}
	if !t.usable() {
		return nil, ErrNoConnection
	}
	loginPw, _, _ := m.store.Secret(secretProfilePfx + p.ID + ":login")
	mqttPw, _, _ := m.store.Secret(secretProfilePfx + p.ID + ":mqtt")

	var out []ProfileStep
	record := func(step string, err error) {
		s := ProfileStep{Step: step, Result: ResultOK}
		switch {
		case err == nil:
		case errors.Is(err, ErrBadCommand):
			s.Result = ResultSkipped
		default:
			s.Result, s.Message = ResultFail, msgOf(err)
		}
		out = append(out, s)
	}
	check := func(step, action string, value bool, mode string) {
		record(step, m.checkAction(ctx, t, ChecklistAction{IDs: []string{deviceID}, Action: action, Value: value, Mode: mode}))
	}
	apply := func(step, section string, body any) {
		lines, err := m.ConfigApply(ctx, []string{deviceID}, section, body)
		if err == nil && len(lines) > 0 && lines[0].Result != ResultOK {
			err = errors.New(lines[0].Message)
			if lines[0].Message == "" {
				err = errors.New(lines[0].Result)
			}
		}
		record(step, err)
	}

	if p.NamePattern != "" {
		record("name", m.setName(ctx, t, ProfileName(p.NamePattern, t.d)))
	}
	if p.Eco != nil {
		check("eco", CheckEco, *p.Eco, "")
	}
	if p.LEDOff != nil {
		check("ledOff", CheckLED, *p.LEDOff, "")
	}
	if p.AP != nil {
		check("ap", CheckAP, *p.AP, "")
	}
	if p.Roaming != nil {
		check("roaming", CheckRoaming, *p.Roaming, "")
	}
	if p.AutoFW != "" {
		check("autoFW", CheckAutoFW, false, p.AutoFW)
	}
	if p.NTP != "" {
		apply("ntp", SectionOthers, OthersApply{Part: "ntp", NTP: p.NTP})
	}
	if p.Cloud != nil {
		apply("cloud", SectionOthers, OthersApply{Part: "cloud", Enable: p.Cloud})
	}
	if q := p.MQTT; q != nil {
		apply("mqtt", SectionMQTT, MQTTApply{Enabled: q.Enabled, Server: q.Server, User: q.User, Password: mqttPw, NoPassword: q.NoPassword || q.User == "", DefaultPrefix: true})
	}
	if l := p.Login; l != nil {
		user := l.User
		if user == "" {
			user = "admin"
		}
		apply("login", SectionLogin, LoginApply{Enabled: l.Enabled, User: user, Password: loginPw})
	}
	return out, nil
}

// setName sets the device's own name (the one the Name column shows).
func (m *Devices) setName(ctx context.Context, t cfgTarget, name string) error {
	if name == "" {
		return invalid("name")
	}
	if t.gen1 {
		return g1cmd(ctx, t.e.conn, "/settings?name="+q(name))
	}
	return g2call(ctx, t.e.conn, "Sys.SetConfig", map[string]any{"config": map[string]any{"device": map[string]any{"name": name}}})
}

// ProfileDraft is a profile read from a device (DECISIONS P21-1..): everything the device
// tells that a profile can hold, and which of it differs from a Shelly as it leaves the factory.
// The UI lists the settings with a tick; the ticked ones (Deviating at first) become the profile.
// The name, the name pattern and the Wi-Fi reminder are left empty on purpose; passwords are never
// readable from a device, so the user enters them afterwards.
type ProfileDraft struct {
	Device    DeviceRef     `json:"device"`
	Profile   store.Profile `json:"profile"`
	Deviating []string      `json:"deviating"` // step names, as in PlanStep.Step
}

// factoryNTP is the time server of a Shelly as it leaves the factory.
const factoryNTP = "time.google.com"

// ProfileFromDevice reads a device's settings into a draft profile. Only reads.
func (m *Devices) ProfileFromDevice(ctx context.Context, deviceID string) (ProfileDraft, error) {
	ts, err := m.targets([]string{deviceID})
	if err != nil {
		return ProfileDraft{}, err
	}
	t := ts[0]
	if t.blu {
		return ProfileDraft{}, fmt.Errorf("%w: a BLU device has no such settings", ErrBadCommand)
	}
	if !t.usable() {
		return ProfileDraft{}, ErrNoConnection
	}
	out := ProfileDraft{Device: ref(t), Deviating: []string{}}
	p := &out.Profile
	dev := func(step string, yes bool) {
		if yes {
			out.Deviating = append(out.Deviating, step)
		}
	}

	rows := m.Checklist(ctx, []string{deviceID})
	if len(rows) == 1 {
		r := rows[0]
		if b, ok := r.Eco.(bool); ok {
			p.Eco = &b
			dev("eco", b)
		}
		if b, ok := r.LED.(bool); ok {
			p.LEDOff = &b
			dev("ledOff", b)
		}
		if b, ok := r.AP.(bool); ok {
			p.AP = &b
			dev("ap", !b) // Gen2+: on from the factory
		}
		if s, ok := r.Roaming.(string); ok && s != NAStr { // ✗ or the threshold
			on := s != FalseStr
			p.Roaming = &on
			dev("roaming", on == t.gen1) // Gen1: off from the factory; Gen2+: on
		}
		if s, ok := r.AutoFW.(string); ok && s != NAStr {
			if s == FalseStr {
				s = "none"
			}
			if s == "stable" || s == "beta" || s == "none" {
				p.AutoFW = s
				dev("autoFW", s != "none")
			}
		}
	}
	if ntp, _, err := readOthers(ctx, t); err == nil && ntp != "" {
		p.NTP = ntp
		dev("ntp", ntp != factoryNTP)
	}
	cloud := t.d.CloudEnabled
	p.Cloud = &cloud
	dev("cloud", !cloud)
	if q, err := m.readMQTT(ctx, t); err == nil {
		p.MQTT = &store.ProfileMQTT{Enabled: q.Enabled}
		if q.Enabled {
			p.MQTT.Server, p.MQTT.User, p.MQTT.NoPassword = q.Server, q.User, q.User == ""
		}
		dev("mqtt", q.Enabled)
	}
	if l, err := readLogin(ctx, t); err == nil {
		p.Login = &store.ProfileLogin{Enabled: l.enabled}
		if l.enabled && t.gen1 {
			p.Login.User = l.user
		}
		dev("login", l.enabled)
	}
	return out, nil
}
