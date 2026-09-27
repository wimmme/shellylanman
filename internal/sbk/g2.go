// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// backup/restoreCheck/restore of the classes in model/device/g2, g3 and g4;
// g2/modules Input, Relay, Roller, LightWhite, LightRGB(W), LightCCT,
// ThermostatG2, CBreakerPro, SensorAddOn, SensorAddOnPro, LoRaAddOn,
// DynamicComponents, ScheduleManagerThermWD; g3/modules LightRGBCCT,
// Camera; g4/modules PresenceZoneG4.

package sbk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/ojson"
)

const (
	addonSensor     = "sensor"
	addonLoRa       = "LoRa"
	peripheralsFile = "SensorAddon.GetPeripherals.json"
	xt1ST1820       = "linkedgo-st1820-floor-thermostat"
	xt1ST802        = "linkedgo-st-802-hvac"
)

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// state is the device's current configuration, read once per operation
// (the Java objects hold it from their last refresh).
type state struct {
	d      *Device
	ctx    context.Context
	cfg    *ojson.Value
	periph *ojson.Value
}

func loadState(ctx context.Context, d *Device) *state {
	st := &state{d: d, ctx: ctx, cfg: ojson.NewObject()}
	if c, err := d.get(ctx, "/rpc/Shelly.GetConfig"); err == nil {
		st.cfg = c
	}
	return st
}

func (s *state) profile() string   { return s.cfg.Path("sys", "device", "profile").Text() }
func (s *state) addonType() string { return s.cfg.Path("sys", "device", "addon_type").Str("") }
func (s *state) hasAddon() bool    { return s.addonType() == addonSensor }
func (s *state) hasLoRa() bool     { return s.addonType() == addonLoRa }

// peripherals: SensorAddon.GetPeripherals of the device (nil without add-on).
func (s *state) peripherals() *ojson.Value {
	if s.periph == nil && s.hasAddon() {
		if p, err := s.d.get(s.ctx, "/rpc/SensorAddon.GetPeripherals"); err == nil {
			s.periph = p
		} else {
			s.periph = ojson.NewObject()
		}
	}
	return s.periph
}

// addonSensors: the number of sensors configured on the add-on (the
// measurement types of SensorAddOn / SensorAddOnPro).
func (s *state) addonSensors() int {
	n := 0
	p := s.peripherals()
	for _, g := range []string{"ds18b20", "dht22", "digital_in", "analog_in", "voltmeter"} {
		n += p.Get(g).Len()
	}
	return n
}

type g2Model struct {
	check   func(ctx context.Context, d *Device, st *state, f Files, res Check)
	restore func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs)
}

// g2Models picks the class of a Gen2+ device; some apps are shared by two
// classes told apart by model.
func g2Models(d *Device) *g2Model {
	switch d.TypeID {
	case "Pro4PM":
		if d.Model == "SPSH-002PE16EU" {
			return &proDualCover
		}
	case "ProDimmerx", "ProDimmerxProAddon":
		if d.Model == "SPDM-002PE01EU" {
			return &proDimmer2
		}
		return &proDimmer1
	}
	if m, ok := g2Table[d.TypeID]; ok {
		return &m
	}
	return nil
}

// ---- building blocks ------------------------------------------------------------------

type steps []func(ctx context.Context, d *Device, cfg *ojson.Value, e *errs)

func (s steps) run(ctx context.Context, d *Device, cfg *ojson.Value, e *errs) {
	for _, f := range s {
		f(ctx, d, cfg, e)
	}
}

func setIndexed(method, typ string, i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return func(ctx context.Context, d *Device, cfg *ojson.Value, e *errs) {
		e.add(d.post(ctx, method, indexed(cfg, typ, i)))
	}
}

func input(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("Input.SetConfig", "input", i)
}
func sw(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("Switch.SetConfig", "switch", i)
}
func cover(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("Cover.SetConfig", "cover", i)
}
func light(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("Light.SetConfig", "light", i)
}
func rgb(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("RGB.SetConfig", "rgb", i)
}
func rgbw(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("RGBW.SetConfig", "rgbw", i)
}
func cct(i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return setIndexed("CCT.SetConfig", "cct", i)
}
func inputs(n int) steps {
	var s steps
	for i := 0; i < n; i++ {
		s = append(s, input(i))
	}
	return s
}

// ui: "<METHOD>.SetConfig" {"config": cfg[key]} (optional: skipped when absent).
func ui(method, key string, optional bool) func(context.Context, *Device, *ojson.Value, *errs) {
	return func(ctx context.Context, d *Device, cfg *ojson.Value, e *errs) {
		v := cfg.Get(key)
		if optional && !v.Exists() {
			return
		}
		e.add(d.post(ctx, method, ojson.Obj("config", v.Clone())))
	}
}

// emNoCT: EM/EM1.SetConfig without ct_type.
func emNoCT(method, typ string, i int) func(context.Context, *Device, *ojson.Value, *errs) {
	return func(ctx context.Context, d *Device, cfg *ojson.Value, e *errs) {
		c := indexed(cfg, typ, i)
		c.Get("config").Remove("ct_type")
		e.add(d.post(ctx, method, c))
	}
}

// ---- add-ons ------------------------------------------------------------------------

// checkAddon: SensorAddOn.restoreCheck / SensorAddOnPro.restoreCheck.
func checkAddon(ctx context.Context, d *Device, st *state, f Files, res Check) {
	back := f[peripheralsFile]
	if !back.Exists() {
		return
	}
	n := 0
	for _, k := range back.Keys() {
		if back.Get(k).Len() > 0 {
			n++
		}
	}
	switch {
	case !st.hasAddon() && n > 0:
		res.put(WarnAddonEnable, "")
	case st.hasAddon() && st.addonSensors() > 0 && n > 0:
		if !ojson.Equal(st.peripherals(), back) {
			res.put(WarnAddonCantInstall, "")
		}
	case st.hasAddon() && st.addonSensors() == 0 && n > 0:
		res.put(WarnAddonInstall, "")
	}
}

// restoreAddon: SensorAddOn.restore; pro adds the "io" attribute and the
// digital output (switch) configuration.
func restoreAddon(ctx context.Context, d *Device, st *state, f Files, e *errs, pro bool) {
	back := f[peripheralsFile]
	enable := func(on bool) *string {
		var t *ojson.Value = ojson.NullV()
		if on {
			t = ojson.Str(addonSensor)
		}
		return d.post(ctx, "Sys.SetConfig", ojson.Obj("config", ojson.Obj("device", ojson.Obj("addon_type", t))))
	}
	switch {
	case !back.Exists() && st.hasAddon():
		e.add(enable(false))
	case back.Exists() && !st.hasAddon():
		e.add(enable(true))
	case back.Exists() && st.addonSensors() == 0:
		for _, sensor := range back.Keys() {
			group := back.Get(sensor)
			prev := ""
			for _, key := range group.Keys() {
				idx := key[strings.Index(key, ":")+1:]
				if idx == prev { // dht22 has two entries but is added once
					continue
				}
				prev = idx
				attrs := group.Get(key)
				cid, _ := strconv.Atoi(idx)
				a := ojson.Obj("cid", cid)
				if pro {
					a.Set("io", ojson.Int(attrs.Get("io").Int()))
				}
				if attrs.Get("addr").Exists() {
					a.Set("addr", ojson.Str(attrs.Get("addr").Text()))
				}
				e.add(d.post(ctx, "SensorAddon.AddPeripheral", ojson.Obj("type", sensor, "attrs", a)))
			}
		}
	case back.Exists():
		backCfg := f["Shelly.GetConfig.json"]
		cur, err := d.get(ctx, "/rpc/Shelly.GetConfig")
		if err != nil {
			return
		}
		methods := map[string]string{"temperature": "Temperature.SetConfig", "humidity": "Humidity.SetConfig", "input": "Input.SetConfig", "voltmeter": "Voltmeter.SetConfig"}
		if pro {
			methods["switch"] = "Switch.SetConfig"
		}
		for _, sensor := range back.Keys() {
			for _, key := range back.Get(sensor).Keys() {
				if !cur.Get(key).Exists() {
					continue
				}
				typIdx := strings.SplitN(key, ":", 2)
				if m, ok := methods[typIdx[0]]; ok && len(typIdx) == 2 {
					i, _ := strconv.Atoi(typIdx[1])
					e.add(d.post(ctx, m, indexed(backCfg, typIdx[0], i)))
				}
			}
		}
	}
}

func checkLoRa(st *state, f Files, res Check) {
	if f["Shelly.GetConfig.json"].Get("lora:100").NonNull() && !st.hasLoRa() {
		res.put(WarnLoRaEnable, "")
	}
}

func restoreLoRa(ctx context.Context, d *Device, st *state, cfg *ojson.Value, e *errs) {
	if !cfg.Get("lora:100").NonNull() {
		return
	}
	if !st.hasLoRa() {
		e.add(d.post(ctx, "Sys.SetConfig", ojson.Obj("config", ojson.Obj("device", ojson.Obj("addon_type", addonLoRa)))))
	} else {
		e.add(d.post(ctx, "LoRa.SetConfig", indexed(cfg, "lora", 100)))
	}
}

// ---- model helpers ---------------------------------------------------------------------

func withAddon(s steps) g2Model {
	return g2Model{
		check: checkAddon,
		restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
			s.run(ctx, d, cfg, e)
			restoreAddon(ctx, d, st, f, e, false)
		},
	}
}

// g3Addon: SensorAddOn + LoRa (Gen3/Gen4 classes).
func g3Addon(s steps) g2Model {
	return g2Model{
		check: func(ctx context.Context, d *Device, st *state, f Files, res Check) {
			checkAddon(ctx, d, st, f, res)
			checkLoRa(st, f, res)
		},
		restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
			s.run(ctx, d, cfg, e)
			restoreAddon(ctx, d, st, f, e, false)
			restoreLoRa(ctx, d, st, cfg, e)
		},
	}
}

// proAddon: SensorAddOnPro + LoRa (Pro classes).
func proAddon(s steps, check bool) g2Model {
	m := g2Model{restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		s.run(ctx, d, cfg, e)
		restoreAddon(ctx, d, st, f, e, true)
		restoreLoRa(ctx, d, st, cfg, e)
	}}
	if check {
		m.check = func(ctx context.Context, d *Device, st *state, f Files, res Check) {
			checkAddon(ctx, d, st, f, res)
			checkLoRa(st, f, res)
		}
	}
	return m
}

func plain(s steps) g2Model {
	return g2Model{restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		s.run(ctx, d, cfg, e)
	}}
}

// twoPM: switch or cover profile must match (Shelly*2PM*).
func twoPM(addon func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs), extraCheck func(ctx context.Context, d *Device, st *state, f Files, res Check)) g2Model {
	return g2Model{
		check: func(ctx context.Context, d *Device, st *state, f Files, res Check) {
			if (f["Shelly.GetDeviceInfo.json"].Get("profile").Text() == "switch") != (st.profile() == "switch") {
				res.put(ErrModeCover, "")
			}
			extraCheck(ctx, d, st, f, res)
		},
		restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
			backRelay := cfg.Path("sys", "device", "profile").Text() == "switch"
			if backRelay == (st.profile() == "switch") {
				steps{input(0), input(1)}.run(ctx, d, cfg, e)
				if backRelay {
					steps{sw(0), sw(1)}.run(ctx, d, cfg, e)
				} else {
					cover(0)(ctx, d, cfg, e)
				}
			} else {
				e.msg(ErrModeCover)
			}
			addon(ctx, d, st, f, cfg, e)
		},
	}
}

func addonOnly(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	restoreAddon(ctx, d, st, f, e, false)
}
func addonAndLoRa(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	restoreAddon(ctx, d, st, f, e, false)
	restoreLoRa(ctx, d, st, cfg, e)
}
func addonProLoRa(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	restoreAddon(ctx, d, st, f, e, true)
	restoreLoRa(ctx, d, st, cfg, e)
}
func checkAddonLoRa(ctx context.Context, d *Device, st *state, f Files, res Check) {
	checkAddon(ctx, d, st, f, res)
	checkLoRa(st, f, res)
}

// em3: EM (triphase) or three EM1 (monophase) — the device's current profile.
func em3(pro bool) g2Model {
	return g2Model{
		check: func(ctx context.Context, d *Device, st *state, f Files, res Check) {
			if (f["Shelly.GetDeviceInfo.json"].Get("profile").Text() == "triphase") != (st.profile() == "triphase") {
				res.put(ErrModeTriphase, "")
			}
			if pro {
				checkAddonLoRa(ctx, d, st, f, res)
			}
		},
		restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
			if st.profile() == "triphase" {
				emNoCT("EM.SetConfig", "em", 0)(ctx, d, cfg, e)
				return
			}
			steps{emNoCT("EM1.SetConfig", "em1", 0), emNoCT("EM1.SetConfig", "em1", 1), emNoCT("EM1.SetConfig", "em1", 2)}.run(ctx, d, cfg, e)
			if pro { // the original restores the add-ons only in the monophase branch
				addonProLoRa(ctx, d, st, f, cfg, e)
			}
		},
	}
}

// profileModel: the backup must have the device's current profile.
func profileModel(restoreProfile func(profile string) steps, tail func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs), n int, check func(ctx context.Context, d *Device, st *state, f Files, res Check)) g2Model {
	return g2Model{
		check: func(ctx context.Context, d *Device, st *state, f Files, res Check) {
			back := f["Shelly.GetDeviceInfo.json"].Get("profile").Text()
			if st.profile() != back {
				res.put(ErrProfile, "", st.profile(), back)
			}
			check(ctx, d, st, f, res)
		},
		restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
			inputs(n).run(ctx, d, cfg, e)
			if back := cfg.Path("sys", "device", "profile").Text(); back == st.profile() {
				restoreProfile(back).run(ctx, d, cfg, e)
			} else {
				e.msg(ErrProfile)
			}
			tail(ctx, d, st, f, cfg, e)
		},
	}
}

var (
	proDualCover = plain(steps{input(0), input(1), input(2), input(3), cover(0), cover(1), ui("Ui.SetConfig", "ui", false)})
	proDimmer1   = proDimmer("SPDM-001PE01EU", steps{input(0), input(1), light(0)})
	proDimmer2   = proDimmer("SPDM-002PE01EU", steps{input(0), input(1), light(0), input(2), input(3), light(1)})
)

func proDimmer(model string, s steps) g2Model {
	m := proAddon(s, false)
	m.check = func(ctx context.Context, d *Device, st *state, f Files, res Check) {
		if f["Shelly.GetDeviceInfo.json"].Get("model").Text() != model {
			res.put(ErrModel, "")
		}
		checkAddonLoRa(ctx, d, st, f, res)
	}
	return m
}

func ht(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	e.add(d.post(ctx, "HT_UI.SetConfig", ojson.Obj("config", cfg.Get("ht_ui"))))
	for _, t := range []struct{ key, method string }{{"temperature:0", "Temperature.SetConfig"}, {"humidity:0", "Humidity.SetConfig"}} {
		n := cfg.Get(t.key).Clone()
		n.Remove("id")
		e.add(d.post(ctx, t.method, ojson.Obj("id", 0, "config", n)))
	}
}

func blugw(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	d.post(ctx, "BluGw.SetConfig", ojson.Obj("config", ojson.Obj("sys_led_enable", cfg.Path("blugw", "sys_led_enable").Bool()))) // result not reported, as the original
}

var g2Table = map[string]g2Model{
	"Plus1": withAddon(steps{input(0), sw(0)}), "Plus1PM": withAddon(steps{input(0), sw(0)}),
	"Plus1Mini": plain(steps{input(0), sw(0)}), "Plus1PMMini": plain(steps{input(0), sw(0)}),
	"PlusPMMini": plain(steps{setIndexed("PM1.SetConfig", "pm1", 0)}),
	"Plus10V":    withAddon(steps{input(0), input(1), light(0)}),
	"Plus2PM":    twoPM(addonOnly, checkAddon),
	"PlusHT":     {restore: ht},
	"PlusPlugIT": plain(steps{sw(0)}), "PlugUS": plain(steps{sw(0)}),
	"PlusPlugS":  plain(steps{ui("PLUGS_UI.SetConfig", "plugs_ui", false), sw(0)}),
	"PlusPlugUK": plain(steps{ui("PLUGUK_UI.SetConfig", "pluguk_ui", false), sw(0)}),
	"PlusRGBWPM": profileModel(func(p string) steps {
		switch p {
		case "light":
			return steps{light(0), light(1), light(2), light(3)}
		case "rgbw":
			return steps{rgbw(0)}
		}
		return steps{rgb(0)}
	}, func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		e.add(d.post(ctx, "PlusRGBWPM.SetConfig", ojson.Obj("config", ojson.Obj("hf_mode", cfg.Path("plusrgbwpm", "hf_mode").Kind() == ojson.Bool && cfg.Path("plusrgbwpm", "hf_mode").Bool()))))
		restoreAddon(ctx, d, st, f, e, false)
	}, 4, checkAddon),
	"PlusSmoke": {restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		e.add(d.post(ctx, "Smoke.SetConfig", ojson.Obj("config", cfg.Get("smoke:0"))))
	}},
	"PlusUni":        withAddon(inputs(3)),
	"PlusI4":         withAddon(inputs(4)),
	"Pro1":           proAddon(steps{input(0), input(1), sw(0)}, true),
	"Pro1ProAddon":   proAddon(steps{input(0), input(1), sw(0)}, true),
	"Pro1PM":         proAddon(steps{input(0), input(1), sw(0)}, true),
	"Pro1PMProAddon": proAddon(steps{input(0), input(1), sw(0)}, true),
	"Pro2":           proAddon(steps{input(0), input(1), sw(0), sw(1)}, false),
	"Pro2ProAddon":   proAddon(steps{input(0), input(1), sw(0), sw(1)}, false),
	// ShellyPro2CB posts the voltmeter with CB.SetConfig; we use Voltmeter.SetConfig (FEATURE_PARITY O25).
	"ProCB":          plain(steps{setIndexed("CB.SetConfig", "cb", 0), setIndexed("Voltmeter.SetConfig", "voltmeter", 0)}),
	"Pro2PM":         twoPM(addonProLoRa, checkAddonLoRa),
	"Pro2PMProAddon": twoPM(addonProLoRa, checkAddonLoRa),
	"Pro3":           plain(steps{input(0), input(1), input(2), sw(0), sw(1), sw(2)}),
	"Pro3EM":         em3(true), "Pro3EMProAddon": em3(true),
	"Pro4PM":        plain(steps{input(0), input(1), input(2), input(3), sw(0), sw(1), sw(2), sw(3), ui("Ui.SetConfig", "ui", false)}),
	"ProEM":         proAddon(steps{sw(0), emNoCT("EM1.SetConfig", "em1", 0), emNoCT("EM1.SetConfig", "em1", 1)}, true),
	"ProEMProAddon": proAddon(steps{sw(0), emNoCT("EM1.SetConfig", "em1", 0), emNoCT("EM1.SetConfig", "em1", 1)}, true),
	"ProRGBWWPM": profileModel(func(p string) steps {
		switch p {
		case "light":
			return steps{light(0), light(1), light(2), light(3), light(4)}
		case "rgbcct":
			return steps{rgb(0), cct(0)}
		case "cctx2":
			return steps{cct(0), cct(1)}
		}
		return steps{rgb(0), light(0), light(1)}
	}, func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		if hf := cfg.Path("prorgbwwpm", "hf_mode"); hf.Exists() {
			e.add(d.post(ctx, "ProRGBWWPM.SetConfig", ojson.Obj("config", ojson.Obj("hf_mode", hf.Kind() == ojson.Bool && hf.Bool()))))
		}
		restoreLoRa(ctx, d, st, cfg, e)
	}, 5, func(ctx context.Context, d *Device, st *state, f Files, res Check) { checkLoRa(st, f, res) }),
	"PlusWallDimmer":  plain(steps{light(0), ui("WD_UI.SetConfig", "wd_ui", false)}),
	"WallDisplay":     wallDisplay(false),
	"WallDisplayV2":   wallDisplay(true),
	"BluGw":           {restore: blugw},
	"BluGwG3":         {restore: blugw},
	"Ogemray25":       plain(steps{input(0), sw(0)}),
	"XT1":             {restore: xt1Restore},
	"Dimmer0110VPMG3": g3Addon(steps{input(0), input(1), light(0)}),
	"S4DM-0010WW":     g3Addon(steps{input(0), input(1), light(0)}),
	"S1G3":            g3Addon(steps{input(0), sw(0)}), "S4SW-001X16EU": g3Addon(steps{input(0), sw(0)}),
	"S1PMG3": g3Addon(steps{input(0), sw(0)}), "S4SW-001P16EU": g3Addon(steps{input(0), sw(0)}),
	"S1LG3": plain(steps{input(0), input(1), sw(0)}), "S4SW-0A1X1EUL": plain(steps{input(0), input(1), sw(0)}),
	"S2LG3": plain(steps{input(0), input(1), sw(0), sw(1)}), "S4SW-0A2X4EUL": plain(steps{input(0), input(1), sw(0), sw(1)}),
	"S2PMG3": twoPM(addonAndLoRa, checkAddonLoRa), "S4SW-002P16EU": twoPM(addonAndLoRa, checkAddonLoRa),
	"S3EMG3":       em3(false),
	"DuoBulbG3":    plain(steps{cct(0)}),
	"RGBCCTBulbG3": plain(steps{setIndexed("RGBCCT.SetConfig", "rgbcct", 0)}),
	"Camera":       {restore: cameraRestore},
	"DimmerG3": {check: checkAddon, restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		steps{input(0), input(1), light(0)}.run(ctx, d, cfg, e)
	}},
	"S4DM-0A101WWL": {check: checkAddon, restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		steps{input(0), input(1), light(0)}.run(ctx, d, cfg, e)
	}},
	"EMG3":          g3Addon(steps{sw(0), emNoCT("EM1.SetConfig", "em1", 0), emNoCT("EM1.SetConfig", "em1", 1)}),
	"S4EM-002CXCEU": g3Addon(steps{sw(0), emNoCT("EM1.SetConfig", "em1", 0), emNoCT("EM1.SetConfig", "em1", 1)}),
	"HTG3":          {restore: ht},
	"I4G3":          withAddon(inputs(4)),
	"Mini1G3":       plain(steps{input(0), sw(0)}), "S4SW-001X8EU": plain(steps{input(0), sw(0)}),
	"Mini1PMG3": plain(steps{input(0), sw(0)}), "S4SW-001P8EU": plain(steps{input(0), sw(0)}),
	"MiniPMG3":        plain(steps{setIndexed("PM1.SetConfig", "pm1", 0)}),
	"PlugPMG3":        plain(steps{ui("PLUGPM_UI.SetConfig", "plugpm_ui", true)}),
	"PlugSG3":         plain(steps{ui("PLUGS_UI.SetConfig", "plugs_ui", true), sw(0)}),
	"PlugMG3":         plain(steps{ui("PLUGS_UI.SetConfig", "plugs_ui", true), sw(0)}),
	"OutdoorPlugSG3":  plain(steps{ui("PLUGS_UI.SetConfig", "plugs_ui", true), sw(0)}),
	"S2PMG3Shutter":   g3Addon(steps{input(0), input(1), cover(0)}),
	"XMOD1":           xmod1,
	"S4SN-0071A":      plain(steps{setIndexed("Flood.SetConfig", "flood", 0)}),
	"S4SN-0071Z":      plain(steps{setIndexed("Flood.SetConfig", "flood", 0)}),
	"S4EM-001PXCEU16": plain(steps{setIndexed("EM1.SetConfig", "em1", 0)}),
	"S4PL-00416EU":    plain(steps{sw(0), sw(1), sw(2), sw(3), ui("POWERSTRIP_UI.SetConfig", "powerstrip_ui", false)}),
	"S4SN-0U61X":      {restore: presenceRestore},
}

// ---- Wall Display --------------------------------------------------------------------

func wallDisplay(x2i bool) g2Model {
	return g2Model{
		check: func(ctx context.Context, d *Device, st *state, f Files, res Check) {
			if x2i { // the power base (channels) must match
				cur, err := d.get(ctx, "/rpc/Shelly.GetDeviceInfo")
				if err == nil && !ojson.Equal(f["Shelly.GetDeviceInfo.json"].Get("ch"), cur.Get("ch")) {
					res.put(ErrPowerBase, "")
					return
				}
			}
			if f["Shelly.GetConfig.json"].Get("thermostat:0").Exists() != st.cfg.Get("thermostat:0").Exists() {
				res.put(ErrModeTherm, "")
			}
		},
		restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
			therm, curTherm := cfg.Get("thermostat:0").Exists(), st.cfg.Get("thermostat:0").Exists()
			if x2i {
				if therm && curTherm {
					restoreThermostatWD(ctx, d, f, cfg, e)
				}
				for i := 0; i < 2; i++ {
					if st.cfg.Get("switch:"+itoa(i)).Exists() && cfg.Get("switch:"+itoa(i)).NonNull() {
						sw(i)(ctx, d, cfg, e)
					}
				}
				if cfg.Get("input:0").NonNull() {
					input(0)(ctx, d, cfg, e)
				}
			} else {
				switch {
				case therm && curTherm:
					restoreThermostatWD(ctx, d, f, cfg, e)
				case !therm && !curTherm:
					sw(0)(ctx, d, cfg, e)
				default:
					e.msg(ErrModeTherm)
				}
				input(0)(ctx, d, cfg, e)
			}
			steps{ui("Ui.SetConfig", "ui", false), setIndexed("Temperature.SetConfig", "temperature", 0),
				setIndexed("Humidity.SetConfig", "humidity", 0), setIndexed("Illuminance.SetConfig", "illuminance", 0)}.run(ctx, d, cfg, e)
		},
	}
}

// restoreThermostatWD: ThermostatG2.restore (without enable and target_C)
// and ScheduleManagerThermWD.restore (profiles replaced, rules recreated).
func restoreThermostatWD(ctx context.Context, d *Device, f Files, cfg *ojson.Value, e *errs) {
	th := cfg.Get("thermostat:0").Clone()
	for _, k := range []string{"id", "enable", "target_C"} {
		th.Remove(k)
	}
	e.add(d.post(ctx, "Thermostat.SetConfig", ojson.Obj("id", 0, "config", th)))
	cur, err := d.get(ctx, "/rpc/Thermostat.Schedule.ListProfiles?id=0")
	if err != nil {
		e.msg(errorText(err))
		return
	}
	for _, p := range cur.Get("profiles").Items() {
		e.add(d.post(ctx, "Thermostat.Schedule.DeleteProfile", ojson.Obj("id", 0, "profile_id", p.Get("id").Int())))
	}
	for _, p := range f["Thermostat.Schedule.ListProfiles.json"].Get("profiles").Items() {
		res, err := d.call(ctx, "Thermostat.Schedule.AddProfile", ojson.Obj("id", 0, "name", p.Get("name").Text()))
		if err != nil {
			e.msg(errorText(err))
			return
		}
		newID := res.Get("profile_id").Int()
		rules := f["Thermostat.Schedule.ListRules_profile_id-"+itoa(p.Get("id").Int())+".json"].Get("rules")
		for _, r := range rules.Items() {
			if _, err := d.call(ctx, "Thermostat.Schedule.CreateRule", ojson.Obj("id", 0, "config", ojson.Obj(
				"profile_id", newID, "target_C", ojson.Num(javaFloat(r.Get("target_C").Float())), "timespec", r.Get("timespec").Text(),
				"enable", r.Get("enable").Kind() == ojson.Bool && r.Get("enable").Bool()))); err != nil {
				e.msg(errorText(err))
				return
			}
		}
	}
}

// ---- others -----------------------------------------------------------------------

func xt1Restore(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	if d.Variant != xt1ST1820 && d.Variant != xt1ST802 {
		return
	}
	c := f["Service.GetConfig.json"].Clone()
	c.Remove("id")
	if d.Variant == xt1ST802 {
		c.Remove("thermostat_mode") // do not restore the mode
	}
	e.add(d.post(ctx, "Service.SetConfig", ojson.Obj("id", 0, "config", c)))
}

var xmod1 = g2Model{
	check: func(ctx context.Context, d *Device, st *state, f Files, res Check) {
		ni, no := xmodIO(ctx, d)
		stored := f["XMOD.GetInfo.json"].Path("jwt", "xmod1")
		if stored.Get("ni").Int() != ni || stored.Get("no").Int() != no {
			res.put(WarnXmodIO, "")
		}
		checkAddon(ctx, d, st, f, res)
	},
	restore: func(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
		ni, no := xmodIO(ctx, d)
		stored := f["XMOD.GetInfo.json"].Path("jwt", "xmod1")
		for i := 0; i < min(stored.Get("ni").Int(), ni); i++ {
			input(i)(ctx, d, cfg, e)
		}
		for i := 0; i < min(stored.Get("no").Int(), no); i++ {
			sw(i)(ctx, d, cfg, e)
		}
		restoreAddon(ctx, d, st, f, e, false)
	},
}

func xmodIO(ctx context.Context, d *Device) (int, int) {
	v, err := d.get(ctx, "/rpc/XMOD.GetInfo")
	if err != nil {
		return 0, 0
	}
	x := v.Path("jwt", "xmod1")
	return x.Get("ni").Int(), x.Get("no").Int()
}

// dynamicComponents lists the device's dynamic components with their config.
func dynamicComponents(ctx context.Context, d *Device) ([]*ojson.Value, error) {
	v, err := d.paged(ctx, "/rpc/Shelly.GetComponents?dynamic_only=true&include=[%22config%22]", "components")
	if err != nil {
		return nil, err
	}
	return v.Get("components").Items(), nil
}

// cameraRestore: storage and camera configuration, then the zones
// (configure existing, add new, delete the others).
func cameraRestore(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	e.add(d.post(ctx, "Storage.SetConfig", indexed(cfg, "storage", 0)))
	e.add(d.post(ctx, "Camera.SetConfig", indexed(cfg, "camera", 0)))
	comps, err := dynamicComponents(ctx, d)
	if err != nil {
		e.msg(errorText(err))
		return
	}
	current := map[string]bool{}
	for _, c := range comps {
		if k := c.Get("key").Text(); strings.HasPrefix(k, "camerazone:") {
			current[k] = true
		}
	}
	for _, c := range f["Shelly.GetComponents.json"].Get("components").Items() {
		key := c.Get("key").Text()
		if !strings.HasPrefix(key, "camerazone:") {
			continue
		}
		data := c.Get("config").Clone()
		if current[key] {
			id := data.Remove("id").Int()
			e.add(d.post(ctx, "CameraZone.SetConfig", ojson.Obj("id", id, "config", data)))
			delete(current, key)
		} else {
			data.Set("id", ojson.Int(0))
			e.add(d.post(ctx, "Camera.AddZone", data))
		}
	}
	for _, key := range sortedKeys(current) {
		id, _ := strconv.Atoi(strings.TrimPrefix(key, "camerazone:"))
		e.add(d.post(ctx, "Camera.DeleteZone", ojson.Obj("id", 0, "zone_id", id)))
	}
}

// presenceRestore: ShellyPresenceG4.restore.
func presenceRestore(ctx context.Context, d *Device, st *state, f Files, cfg *ojson.Value, e *errs) {
	e.add(d.post(ctx, "Illuminance.SetConfig", indexed(cfg, "illuminance", 0)))
	presence := cfg.Get("presence").Clone()
	backMain := presence.Remove("main_zone").Text()
	sensor := presence.Get("sensor")
	if sensor.Get("sensitivity").Text() != "custom" {
		for _, k := range []string{"points", "velocity", "snr", "max_velocity", "state"} {
			sensor.Remove(k)
		}
	} else {
		sensor.Remove("sensitivity")
	}
	e.add(d.post(ctx, "Presence.SetConfig", ojson.Obj("config", presence)))
	backComps := f["Shelly.GetComponents.json"].Get("components").Items()
	curMain := strings.TrimPrefix(st.cfg.Path("presence", "main_zone").Text(), "presencezone:")
	// PresenceZoneG4.restore: configure the zone; its result is not reported
	// (the original returns null).
	configZone := func(id string, backKey string) {
		for _, c := range backComps {
			if c.Get("key").Text() == backKey {
				data := c.Get("config").Clone()
				data.Remove("id")
				n, _ := strconv.Atoi(id)
				d.post(ctx, "PresenceZone.SetConfig", ojson.Obj("id", n, "config", data))
				break
			}
		}
		e.add(nil)
	}
	configZone(curMain, backMain)
	comps, err := dynamicComponents(ctx, d)
	if err != nil {
		e.msg(errorText(err))
		return
	}
	current := map[string]bool{}
	for _, c := range comps {
		if k := c.Get("key").Text(); strings.HasPrefix(k, "presencezone:") && k != "presencezone:"+curMain {
			current[k] = true
		}
	}
	for _, c := range backComps {
		key := c.Get("key").Text()
		if !strings.HasPrefix(key, "presencezone:") || key == backMain {
			continue
		}
		if current[key] {
			configZone(strings.TrimPrefix(key, "presencezone:"), key)
			delete(current, key)
		} else {
			data := c.Get("config").Clone()
			data.Remove("id")
			if _, err := d.call(ctx, "Presence.AddZone", ojson.Obj("config", data)); err != nil {
				e.msg(errorText(err))
			}
		}
	}
	for _, key := range sortedKeys(current) {
		id, _ := strconv.Atoi(strings.TrimPrefix(key, "presencezone:"))
		e.add(d.post(ctx, "Presence.DeleteZone", ojson.Obj("id", id)))
	}
}

// ---- dynamic components (virtual components, BTHome sensors, LNM) ---------------------

var virtualTypes = []string{"boolean", "number", "text", "enum", "group", "button"}

func isVirtual(t string) bool {
	for _, v := range virtualTypes {
		if v == t {
			return true
		}
	}
	return false
}

// checkDynamic: DynamicComponents.restoreCheck — warn when a backed-up
// BTHome device is not registered on the device with the same address.
func checkDynamic(ctx context.Context, d *Device, f Files, res Check) {
	stored := f["Shelly.GetComponents.json"].Get("components")
	if stored.Len() == 0 {
		return
	}
	cur, err := dynamicComponents(ctx, d)
	if err != nil {
		return
	}
	for _, s := range stored.Items() {
		key := strings.ToLower(s.Get("key").Text())
		if !strings.HasPrefix(key, "bthomedevice:") {
			continue
		}
		found := false
		for _, c := range cur {
			if strings.ToLower(c.Get("key").Text()) == key && ojson.Equal(c.Path("config", "addr"), s.Path("config", "addr")) {
				found = true
				break
			}
		}
		if !found {
			res.put(WarnBTHome, "")
			return
		}
	}
}

// restoreDynamic: DynamicComponents.restore — delete virtual components,
// BTHome sensors and LNM, recreate them from the backup (sensors only for
// BTHome devices still registered), then the group values.
func restoreDynamic(ctx context.Context, d *Device, f Files, e *errs) {
	stored := f["Shelly.GetComponents.json"]
	if !stored.Exists() {
		return
	}
	cur, err := dynamicComponents(ctx, d)
	if err != nil {
		return
	}
	var btAddrs []string
	for _, c := range cur {
		key := c.Get("key").Text()
		typ, idx, _ := strings.Cut(key, ":")
		switch {
		case isVirtual(typ):
			d.post(ctx, "Virtual.Delete", ojson.Obj("key", key))
		case strings.ToLower(typ) == "bthomesensor":
			n, _ := strconv.Atoi(idx)
			d.post(ctx, "BTHome.DeleteSensor", ojson.Obj("id", n))
		case strings.ToLower(typ) == "lnm":
			n, _ := strconv.Atoi(idx)
			d.post(ctx, "LNM.Delete", ojson.Obj("id", n))
		case strings.ToLower(typ) == "bthomedevice":
			btAddrs = append(btAddrs, c.Path("config", "addr").Text())
		}
	}
	hasAddr := func(a string) bool {
		for _, x := range btAddrs {
			if x == a {
				return true
			}
		}
		return false
	}
	var existing []string
	type gv struct {
		id  int
		val *ojson.Value
	}
	var groups []gv
	for _, s := range stored.Get("components").Items() {
		key := s.Get("key").Text()
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			continue
		}
		id, _ := strconv.Atoi(parts[1])
		config := s.Get("config").Clone()
		if config.Kind() != ojson.Object {
			config = ojson.NewObject()
		}
		switch {
		case isVirtual(parts[0]):
			config.Remove("id")
			e.add(d.post(ctx, "Virtual.Add", ojson.Obj("type", parts[0], "id", id, "config", config)))
			existing = append(existing, key)
			if v := s.Path("status", "value"); parts[0] == "group" && v.Len() > 0 {
				groups = append(groups, gv{id, v.Clone()})
			}
		case parts[0] == "bthomesensor" && hasAddr(s.Path("config", "addr").Text()):
			config.Remove("id")
			e.add(d.post(ctx, "BTHome.AddSensor", ojson.Obj("id", id, "config", config)))
			existing = append(existing, key)
		case parts[0] == "bthomedevice" && hasAddr(s.Path("config", "addr").Text()):
			config.Remove("id")
			config.Remove("addr")
			e.add(d.post(ctx, "BTHomeDevice.SetConfig", ojson.Obj("id", id, "config", config)))
			existing = append(existing, key)
		case parts[0] == "lnm":
			r := d.post(ctx, "LNM.Create", ojson.Obj("id", id, "config", ojson.Obj("addr", config.Get("addr"))))
			if r == nil {
				config.Remove("id")
				config.Remove("addr")
				r = d.post(ctx, "LNM.SetConfig", ojson.Obj("id", id, "config", config))
			}
			e.add(r)
		}
	}
	for _, g := range groups {
		kept := ojson.NewArray()
		for _, v := range g.val.Items() {
			for _, k := range existing {
				if k == v.Text() {
					kept.Append(v)
					break
				}
			}
		}
		e.add(d.post(ctx, "Group.Set", ojson.Obj("id", g.id, "value", kept)))
	}
}
