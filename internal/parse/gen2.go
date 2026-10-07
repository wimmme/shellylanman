// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// AbstractG2Device.fillSettings/fillStatus and the fillStatus/meters of the
// model classes in model/device/g2, g3 and g4, and the g2 modules Relay,
// LightWhite, Roller and Input.

package parse

import (
	"fmt"
	"sort"
	"strings"
)

// Gen2Input is what the service has read from a Gen2+ device.
type Gen2Input struct {
	TypeID      string
	DeviceName  string // for module labels that fall back to the device name
	Config      []byte // Shelly.GetConfig
	Status      []byte // Shelly.GetStatus
	Peripherals []byte // SensorAddon.GetPeripherals, when an add-on is fitted
	Webhooks    []byte // Webhook.List, for models whose inputs show their actions (i4)
	Variant     string // XT1: svc0.type
	Components  []byte // XT1: Shelly.GetComponents of the service components (XT1Keys)
}

// AddonSensor is sys.device.addon_type for the Sensor Add-on (Plus and Pro).
const AddonSensor = "sensor"

// Gen2 parses a Gen2/3/4 device.
func Gen2(in Gen2Input) Readings {
	cfg, st := decode(in.Config), decode(in.Status)
	r := gen2Common(cfg, st)
	m := gen2Models[in.TypeID]
	if m != nil {
		c := &g2ctx{cfg: cfg, st: st, name: r.Name, r: &r}
		if r.Name == "" {
			c.name = in.DeviceName
		}
		c.hooks = decode(in.Webhooks)
		c.variant, c.comps = in.Variant, decode(in.Components)
		m(c)
		if in.Peripherals != nil && UsesAddon(in.TypeID, r.AddonType) {
			p := decode(in.Peripherals)
			if a := addon(p, cfg, st); len(a.Values) > 0 {
				r.Meters = append(r.Meters, a)
			}
			if proAddonOut[in.TypeID] {
				c.digitalOut(p)
			}
		}
		if r.Layout == "" {
			r.Layout = layoutOf(r.Modules)
		}
	}
	return r
}

// gen2Common: AbstractG2Device.fillSettings + fillStatus.
func gen2Common(cfg, st node) Readings {
	r := Readings{Uptime: -1}
	sys := cfg.Get("sys")
	r.Name = sys.Path("device", "name").Str("")
	r.AddonType = sys.Path("device", "addon_type").Str("")
	dbg := sys.Get("debug")
	switch {
	case dbg.Path("websocket", "enable").Bool():
		r.LogMode = "SOCKET"
	case dbg.Path("mqtt", "enable").Bool():
		r.LogMode = "MQTT"
	case dbg.Get("udp").Exists() && dbg.Path("udp", "addr").Str("") != "":
		r.LogMode = "UDP"
	default:
		r.LogMode = "NONE"
	}
	r.CloudEnabled = cfg.Path("cloud", "enable").Bool()
	r.MQTTEnabled = cfg.Path("mqtt", "enable").Bool()
	r.RangeExtender = cfg.Path("wifi", "ap", "range_extender", "enable").Bool()

	r.CloudConnected = st.Path("cloud", "connected").Bool()
	r.RSSI = st.Path("wifi", "rssi").Int()
	r.SSID = st.Path("wifi", "ssid").Str("")
	if up := st.Path("sys", "uptime"); up.Exists() {
		r.Uptime = up.Int()
	}
	r.RebootRequired = st.Path("sys", "restart_required").Bool()
	if stable := st.Path("sys", "available_updates", "stable"); stable.Exists() {
		r.UpdateAvailable = true
		r.UpdateVersion = stable.Get("version").Str("")
	}
	r.MQTTConnected = st.Path("mqtt", "connected").Bool()
	return r
}

type g2ctx struct {
	cfg, st node
	name    string // device name, the fallback label of relays, lights, covers
	r       *Readings
	hooks   node   // Webhook.List
	variant string // XT1 svc0.type
	comps   node   // XT1 Shelly.GetComponents
}

func (c *g2ctx) profile() string { return c.cfg.Path("sys", "device", "profile").Str("") }

func (c *g2ctx) temp(key string) {
	if t := c.st.Path(key, "temperature", "tC"); t.Exists() {
		c.r.InternalTemp = ptr(t.Float())
	}
}

func (c *g2ctx) meters(s ...MeterSet) { c.r.Meters = append(c.r.Meters, s...) }
func (c *g2ctx) modules(m ...Module)  { c.r.Modules = append(c.r.Modules, m...) }

// wvi: MetersWVI (apower, voltage, current).
func (c *g2ctx) wvi(key string) MeterSet {
	s := c.st.Get(key)
	return set("", W, s.Get("apower").Float(), V, s.Get("voltage").Float(), I, s.Get("current").Float())
}

// wvipf: MetersWVIpf / the W, PF, V, I sets of the 2PM and 4PM classes.
func (c *g2ctx) wvipf(key string) MeterSet {
	s := c.st.Get(key)
	return set("", W, s.Get("apower").Float(), PF, s.Get("pf").Float(), V, s.Get("voltage").Float(), I, s.Get("current").Float())
}

func (c *g2ctx) label(key, fallbackInput string) string {
	if n := c.cfg.Path(key, "name").Str(""); n != "" {
		return n
	}
	if fallbackInput != "" {
		if n := c.cfg.Path(fallbackInput, "name").Str(""); n != "" {
			return n
		}
	}
	return c.name
}

// relay: g2 Relay (switch:N, optional input for label fallback and state).
func (c *g2ctx) relay(idx int, input string) Module {
	key := fmt.Sprintf("switch:%d", idx)
	s := c.st.Get(key)
	m := Module{Kind: KindRelay, Index: idx, Key: key, Label: c.label(key, input), On: ptr(s.Get("output").Bool()), Source: s.Get("source").Str("-")}
	if input != "" {
		m.InputOn = ptr(c.st.Path(input, "state").Bool())
	}
	return m
}

// light: g2 LightWhite on light:N (or any white-like component).
func (c *g2ctx) light(key string, idx int, input string) Module {
	s := c.st.Get(key)
	m := Module{Kind: KindLight, Index: idx, Key: key, Label: c.label(key, ""), On: ptr(s.Get("output").Bool()),
		Brightness: ptr(s.Get("brightness").Int()), Source: s.Get("source").Str("-"), Min: fptr(0), Max: fptr(100)}
	if input != "" {
		m.InputOn = ptr(c.st.Path(input, "state").Bool())
	}
	return m
}

// cct: g2 LightCCT; the colour temperature range comes from ct_range.
func (c *g2ctx) cct(key string, idx int, input string) Module {
	m := c.light(key, idx, input)
	m.Kind = KindCCT
	m.TempK = ptr(c.st.Path(key, "ct").Int())
	m.TMin, m.TMax = 2700, 6500
	if r := c.cfg.Path(key, "ct_range"); r.Len() == 2 {
		m.TMin, m.TMax = r.Idx(0).Int(), r.Idx(1).Int()
	}
	return m
}

func (c *g2ctx) rgb(key string, idx int, input string, kind string) Module {
	s := c.st.Get(key)
	m := Module{Kind: kind, Index: idx, Key: key, Label: c.label(key, ""), On: ptr(s.Get("output").Bool()),
		Brightness: ptr(s.Get("brightness").Int()), Gain: ptr(s.Get("brightness").Int()), Source: s.Get("source").Str("-")}
	if rgb := s.Get("rgb"); rgb.Len() == 3 {
		m.RGB = []int{rgb.Idx(0).Int(), rgb.Idx(1).Int(), rgb.Idx(2).Int()}
	}
	if kind == KindRGBW {
		m.White = ptr(s.Get("white").Int())
	}
	if kind == KindRGBCCT { // g3 LightRGBCCT: brightness is also the gain
		m.TempK = ptr(s.Get("ct").Int())
		m.ColorMode = ptr(s.Get("mode").Str("") == "rgb")
		m.Min, m.Max, m.TMin, m.TMax = fptr(0), fptr(100), 2700, 6500
	}
	if input != "" {
		m.InputOn = ptr(c.st.Path(input, "state").Bool())
	}
	return m
}

// cover: g2 Roller.
func (c *g2ctx) cover(idx int) Module {
	key := fmt.Sprintf("cover:%d", idx)
	s := c.st.Get(key)
	cal := s.Get("pos_control").Bool()
	m := Module{Kind: KindCover, Index: idx, Key: key, Label: c.label(key, ""), State: s.Get("state").Str(""), Calibrated: ptr(cal), Source: s.Get("source").Str("-")}
	if cal {
		m.Position = ptr(s.Get("current_pos").Int())
	}
	return m
}

// input: g2 Input; the events are the webhooks of this input (Webhooks:
// hooks with cid < 200 grouped by event origin + cid, in list order).
func (c *g2ctx) input(idx int) Module {
	key := fmt.Sprintf("input:%d", idx)
	return Module{Kind: KindInput, Index: idx, Key: key, Label: c.cfg.Path(key, "name").Str(""), InputOn: ptr(c.st.Path(key, "state").Bool()),
		Enabled: ptr(c.cfg.Path(key, "enable").Bool()), Events: hookEvents(c.hooks, "input", idx, nil)}
}

// hookEvents lists the webhooks of component origin:cid; cond, when not nil,
// keeps only hooks with that condition (BLU device inputs).
func hookEvents(hooks node, origin string, cid int, cond *string) []InputEvent {
	var out []InputEvent
	list := hooks.Get("hooks")
	for i := 0; i < list.Len(); i++ {
		h := list.Idx(i)
		ev := h.Get("event").Str("")
		dot := strings.IndexByte(ev, '.')
		if h.Get("cid").Int() != cid || dot <= 0 || ev[:dot] != origin {
			continue
		}
		if cond != nil && h.Get("condition").Str("") != *cond {
			continue
		}
		e := InputEvent{Event: ev, Enabled: h.Get("enable").Bool()}
		for j := 0; j < h.Get("urls").Len(); j++ {
			e.URLs = append(e.URLs, h.Get("urls").Idx(j).Str(""))
		}
		out = append(out, e)
	}
	return out
}

func sensorModule(label string, on bool) Module {
	return Module{Kind: KindSensor, Label: label, On: ptr(on)}
}

// em1: EM1Meters (em1:N) — label from configuration, W VA PF V I FREQ.
func (c *g2ctx) em1(idx int) MeterSet {
	key := fmt.Sprintf("em1:%d", idx)
	s := c.st.Get(key)
	return set(c.cfg.Path(key, "name").Str(""), W, s.Get("act_power").Float(), VA, s.Get("aprt_power").Float(),
		PF, s.Get("pf").Float(), V, s.Get("voltage").Float(), I, s.Get("current").Float(), FREQ, s.Get("freq").Float())
}

// emPhases: EMPhaseMeters a, b, c (only phase a gets em:0's name, as in
// ShellyScanner) followed by EMTotalMeters.
func (c *g2ctx) emPhases() {
	em := c.st.Get("em:0")
	for i, p := range []string{"a", "b", "c"} {
		label := ""
		if i == 0 {
			label = c.cfg.Path("em:0", "name").Str("")
		}
		c.meters(set(label, W, em.Get(p+"_act_power").Float(), VA, em.Get(p+"_aprt_power").Float(), PF, em.Get(p+"_pf").Float(),
			V, em.Get(p+"_voltage").Float(), I, em.Get(p+"_current").Float(), FREQ, em.Get(p+"_freq").Float()))
	}
	total := set("", W, em.Get("total_act_power").Float(), VA, em.Get("total_aprt_power").Float(), I, em.Get("total_current").Float())
	total.Total = true
	c.meters(total)
}

// ---- model families -----------------------------------------------------------

type modelFn func(c *g2ctx)

// 1-channel relay with input, internal temperature, optional W/V/I.
func relay1(withMeters, withInput bool) modelFn {
	return func(c *g2ctx) {
		in := ""
		if withInput {
			in = "input:0"
		}
		c.modules(c.relay(0, in))
		c.temp("switch:0")
		if withMeters {
			c.meters(c.wvi("switch:0"))
		}
	}
}

// Dimmer / 0-10 V: light:0 + input:0, temperature, W/V/I.
func dimmer(withMeters bool) modelFn {
	return func(c *g2ctx) {
		c.modules(c.light("light:0", 0, "input:0"))
		c.temp("light:0")
		if withMeters {
			c.meters(c.wvi("light:0"))
		}
	}
}

// 2PM family: two relays or one cover by profile, W/PF/V/I per channel.
func twoPM(c *g2ctx) {
	if c.profile() == "switch" {
		c.modules(c.relay(0, "input:0"), c.relay(1, "input:1"))
		c.meters(c.wvipf("switch:0"), c.wvipf("switch:1"))
		c.temp("switch:0")
		return
	}
	cv := c.cover(0)
	cv.InputOn, cv.InputOn1 = ptr(c.st.Path("input:0", "state").Bool()), ptr(c.st.Path("input:1", "state").Bool())
	c.modules(cv)
	c.meters(c.wvipf("cover:0"))
	c.temp("cover:0")
}

func relays(n int, withInputs bool) modelFn {
	return func(c *g2ctx) {
		for i := 0; i < n; i++ {
			in := ""
			if withInputs {
				in = fmt.Sprintf("input:%d", i)
			}
			c.modules(c.relay(i, in))
		}
		c.temp("switch:0")
	}
}

func inputs(n int) modelFn {
	return func(c *g2ctx) {
		for i := 0; i < n; i++ {
			c.modules(c.input(i))
		}
	}
}

// pm1WVIF: Mini PM / Mini PM G3 — W, V, I, FREQ from pm1:0.
func pm1WVIF(c *g2ctx) {
	s := c.st.Get("pm1:0")
	c.meters(set("", W, s.Get("apower").Float(), V, s.Get("voltage").Float(), I, s.Get("current").Float(), FREQ, s.Get("freq").Float()))
}

// ht: H&T (+ and G3) — T, H, BAT.
func ht(c *g2ctx) {
	c.meters(set("", T, c.st.Path("temperature:0", "tC").Float(), H, float64(int(c.st.Path("humidity:0", "rh").Float())),
		BAT, float64(c.st.Path("devicepower:0", "battery", "percent").Int())))
}

// emTriMono: Pro 3EM, 3EM-63: phases + total (profile "triphase") or three em1 channels.
func emTriMono(c *g2ctx) {
	if c.profile() == "triphase" {
		c.emPhases()
	} else {
		c.meters(c.em1(0), c.em1(1), c.em1(2))
	}
	if t := c.st.Path("temperature:0", "tC"); t.Exists() {
		c.r.InternalTemp = ptr(t.Float())
	}
}

// emRelay: Pro EM-50, EM G3/G4 — relay + two em1 channels.
func emRelay(c *g2ctx) {
	c.modules(c.relay(0, ""))
	c.temp("switch:0")
	c.meters(c.em1(0), c.em1(1))
}

func plusRGBW(c *g2ctx) {
	switch c.profile() {
	case "light":
		for i := 0; i < 4; i++ {
			key := fmt.Sprintf("light:%d", i)
			c.modules(c.light(key, i, fmt.Sprintf("input:%d", i)))
			c.meters(c.wvi(key))
		}
		c.temp("light:0")
	case "rgb":
		c.modules(c.rgb("rgb:0", 0, "input:0", KindRGB))
		c.meters(c.wvi("rgb:0"))
		c.temp("rgb:0")
	default: // rgbw
		c.modules(c.rgb("rgbw:0", 0, "input:0", KindRGBW))
		c.meters(c.wvi("rgbw:0"))
		c.temp("rgbw:0")
	}
}

// proRGBWW: profiles light, rgbcct, cctx2, rgbx2light (ShellyProRGBWW).
func proRGBWW(c *g2ctx) {
	switch c.profile() {
	case "light":
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("light:%d", i)
			c.modules(c.light(key, i, fmt.Sprintf("input:%d", i)))
			c.meters(c.wvi(key))
		}
		c.temp("light:0")
	case "rgbcct":
		c.modules(c.rgb("rgb:0", 0, "input:0", KindRGB), c.cct("cct:0", 0, "input:2"))
		c.meters(c.wvi("rgb:0"), c.wvi("cct:0"))
		c.temp("rgb:0")
	case "cctx2":
		c.modules(c.cct("cct:0", 0, "input:0"), c.cct("cct:1", 1, "input:2"))
		c.meters(c.wvi("cct:0"), c.wvi("cct:1"))
		c.temp("cct:0")
	default: // rgbx2light
		c.modules(c.rgb("rgb:0", 0, "input:0", KindRGB), c.light("light:0", 0, "input:2"), c.light("light:1", 1, "input:3"))
		c.meters(c.wvi("rgb:0"), c.wvi("light:0"), c.wvi("light:1"))
		c.temp("rgb:0")
	}
}

func wallDisplay(c *g2ctx) {
	c.meters(set("", T, c.st.Path("temperature:0", "tC").Float(), H, c.st.Path("humidity:0", "rh").Float(), L, float64(c.st.Path("illuminance:0", "lux").Int())))
	// WallDisplay.getModules: the thermostat when configured, else the relay.
	if c.cfg.Get("thermostat:0").Exists() {
		th := c.st.Get("thermostat:0")
		c.modules(Module{Kind: KindThermostat, Key: "thermostat:0", Label: c.cfg.Path("thermostat:0", "name").Str(""),
			Enabled: ptr(th.Get("enable").Bool()), Running: ptr(th.Get("output").Bool()), Target: ptr(th.Get("target_C").Float()),
			Min: fptr(5), Max: fptr(35), Div: 2})
		return
	}
	c.modules(c.relay(0, "input:0"))
}

var gen2Models = map[string]modelFn{
	// relays
	"Plus1": relay1(false, true), "Plus1PM": relay1(true, true), "Plus1Mini": relay1(false, true), "Plus1PMMini": relay1(true, true),
	"S1G3": relay1(false, true), "S1PMG3": relay1(true, true), "Mini1G3": relay1(false, true), "Mini1PMG3": relay1(true, true),
	"S1LG3": relay1(false, false), "Ogemray25": relay1(true, true),
	"S4SW-001X16EU": relay1(false, true), "S4SW-001P16EU": relay1(true, true), "S4SW-001X8EU": relay1(false, true),
	"S4SW-001P8EU": relay1(true, true), "S4SW-0A1X1EUL": relay1(false, false),
	"Pro1": relay1(false, false), "Pro1ProAddon": relay1(false, false),
	"Pro1PM":         func(c *g2ctx) { relay1(false, false)(c); c.meters(c.wvipf("switch:0")) },
	"Pro1PMProAddon": func(c *g2ctx) { relay1(false, false)(c); c.meters(c.wvipf("switch:0")) },
	"Pro2":           relays(2, true), "Pro2ProAddon": relays(2, true), "Pro3": relays(3, true),
	"S2LG3": relays(2, true), "S4SW-0A2X4EUL": relays(2, true),
	"Pro4PM": func(c *g2ctx) {
		if c.cfg.Get("cover:0").Exists() && !c.cfg.Get("switch:0").Exists() { // Pro Dual Cover (same app)
			proDualCover(c)
			return
		}
		ins := []string{"input:0", "input:1", "input:1", "input:1"} // ShellyPro4PM reads input:1 for channels 3 and 4 (FEATURE_PARITY O13)
		for i := 0; i < 4; i++ {
			c.modules(c.relay(i, ins[i]))
			c.meters(c.wvipf(fmt.Sprintf("switch:%d", i)))
		}
		c.temp("switch:0")
	},
	"S4PL-00416EU": func(c *g2ctx) {
		for i := 0; i < 4; i++ {
			c.modules(c.relay(i, ""))
			c.meters(c.wvipf(fmt.Sprintf("switch:%d", i)))
		}
		c.temp("switch:0")
	},
	// plugs
	"PlusPlugS": relay1(true, false), "PlusPlugIT": relay1(true, false), "PlusPlugUK": relay1(true, false), "PlugUS": relay1(true, false),
	"PlugSG3": relay1(true, false), "OutdoorPlugSG3": relay1(true, false),
	"PlugPMG3": func(c *g2ctx) { c.meters(c.wvi("pm1:0")) }, "PlugMG3": func(c *g2ctx) { c.meters(c.wvi("pm1:0")) },
	"PlusPMMini": pm1WVIF, "MiniPMG3": pm1WVIF,
	// dimmers
	"Plus10V": dimmer(true), "Dimmer0110VPMG3": dimmer(true), "S4DM-0010WW": dimmer(true),
	"DimmerG3": dimmer(true), "S4DM-0A101WWL": dimmer(true),
	"ProDimmerx": func(c *g2ctx) {
		if c.cfg.Get("light:1").Exists() { // Pro Dimmer 2PM
			c.modules(c.light("light:0", 0, "input:0"), c.light("light:1", 1, "input:2"))
			c.meters(c.wvi("light:0"), c.wvi("light:1"))
			c.temp("light:0")
			return
		}
		dimmer(true)(c)
	},
	"PlusWallDimmer": func(c *g2ctx) {
		l := c.light("light:0", 0, "")
		l.Min = fptr(1) // ShellyWallDimmer: new LightWhite(this, 1, 0)
		c.modules(l)
	},
	// 2PM / covers
	"Plus2PM": twoPM, "Pro2PM": twoPM, "Pro2PMProAddon": twoPM, "S2PMG3": twoPM, "S4SW-002P16EU": twoPM,
	"S2PMG3Shutter": func(c *g2ctx) {
		c.modules(c.cover(0))
		c.meters(c.wvipf("cover:0"))
		c.temp("cover:0")
	},
	// inputs
	"PlusI4": inputs(4), "I4G3": inputs(4),
	"PlusUni": func(c *g2ctx) {
		c.modules(c.relay(0, "input:0"), c.relay(1, "input:1"))
		in2 := c.st.Get("input:2")
		c.meters(set("", NUM, float64(in2.Path("counts", "total").Int()), FREQ, float64(in2.Get("freq").Int())))
	},
	// energy meters
	"Pro3EM": emTriMono, "Pro3EMProAddon": emTriMono, "S3EMG3": emTriMono,
	"ProEM": emRelay, "ProEMProAddon": emRelay, "EMG3": emRelay, "S4EM-002CXCEU": emRelay,
	"S4EM-001PXCEU16": func(c *g2ctx) { c.meters(c.em1(0)) },
	// sensors
	"PlusHT": ht, "HTG3": ht,
	"PlusSmoke": func(c *g2ctx) {
		c.meters(set("", BAT, float64(c.st.Path("devicepower:0", "battery", "percent").Int())))
		c.modules(Module{Kind: KindSensor, Label: "Smoke", On: ptr(c.st.Path("smoke:0", "alarm").Bool())})
	},
	"S4SN-0071A": floodG4, "S4SN-0071Z": floodG4,
	"S4SN-0U61X": func(c *g2ctx) {
		lvl := -1.0
		switch c.st.Path("illuminance:0", "illumination").Str("") {
		case "dark":
			lvl = 0
		case "twilight":
			lvl = 1
		case "bright":
			lvl = 2
		}
		c.meters(set("", LE, lvl))
	},
	// lights
	"PlusRGBWPM": plusRGBW, "ProRGBWWPM": proRGBWW,
	"DuoBulbG3": func(c *g2ctx) {
		l := c.cct("cct:0", 0, "")
		l.TMin, l.TMax = 2700, 6500 // ShellyBulbDuoG3: fixed range
		c.modules(l)
		c.meters(set("", W, c.st.Path("cct:0", "apower").Float()))
	},
	"RGBCCTBulbG3": func(c *g2ctx) {
		c.modules(c.rgb("rgbcct:0", 0, "", KindRGBCCT))
		c.meters(set("", W, c.st.Path("rgbcct:0", "apower").Float()))
	},
	// others
	"WallDisplay": wallDisplay, "WallDisplayV2": wallDisplay,
	"ProCB": func(c *g2ctx) {
		cb := c.st.Get("cb:0")
		c.modules(Module{Kind: KindBreaker, Key: "cb:0", Label: c.cfg.Path("cb:0", "name").Str(""), On: ptr(cb.Get("output").Bool()),
			Locked: ptr(cb.Get("safety").Bool()), Source: cb.Get("source").Str("-")})
		c.meters(set("", V, c.st.Path("voltmeter:0", "voltage").Float()))
		if t := cb.Path("temperature", "tC"); t.Exists() {
			c.r.InternalTemp = ptr(t.Float())
		}
	},
	"Camera": func(c *g2ctx) {
		cam := c.st.Get("camera:0")
		c.modules(Module{Kind: KindCamera, Key: "camera:0", Label: c.name, On: ptr(cam.Get("privacy").Bool()), Motion: ptr(cam.Get("motion").Bool())})
	},
	"XT1": xt1,
	"XMOD1": func(c *g2ctx) {
		if c.st.Get("switch:0").Exists() {
			c.modules(c.relay(0, ""))
		}
		if c.st.Get("input:0").Exists() {
			c.modules(c.input(0))
		}
	},
}

func floodG4(c *g2ctx) {
	c.meters(set("", BAT, float64(c.st.Path("devicepower:0", "battery", "percent").Int())))
	c.modules(Module{Kind: KindSensor, Label: "Flood", On: ptr(c.st.Path("flood:0", "alarm").Bool())})
}

func proDualCover(c *g2ctx) {
	for i := 0; i < 2; i++ {
		cv := c.cover(i)
		cv.InputOn = ptr(c.st.Path(fmt.Sprintf("input:%d", 2*i), "state").Bool())
		cv.InputOn1 = ptr(c.st.Path(fmt.Sprintf("input:%d", 2*i+1), "state").Bool())
		c.modules(cv)
		c.meters(c.wvipf(fmt.Sprintf("cover:%d", i)))
	}
	c.temp("cover:0")
}

// Models that read a Sensor Add-on when one is fitted (classes creating
// SensorAddOn / SensorAddOnPro).
var addonModels = map[string]bool{
	"Plus1": true, "Plus1PM": true, "Plus2PM": true, "PlusI4": true, "PlusUni": true, "Plus10V": true, "PlusRGBWPM": true,
	"Pro1": true, "Pro1ProAddon": true, "Pro1PM": true, "Pro1PMProAddon": true, "Pro2": true, "Pro2ProAddon": true,
	"Pro2PM": true, "Pro2PMProAddon": true, "Pro3EM": true, "Pro3EMProAddon": true, "ProDimmerx": true, "ProDimmerxProAddon": true,
	"ProEM": true, "ProEMProAddon": true,
	"S1G3": true, "S1PMG3": true, "S2PMG3": true, "S2PMG3Shutter": true, "Dimmer0110VPMG3": true, "DimmerG3": true, "EMG3": true, "I4G3": true, "XMOD1": true,
	"S4SW-001X16EU": true, "S4SW-001P16EU": true, "S4SW-002P16EU": true, "S4DM-0A101WWL": true, "S4DM-0010WW": true, "S4EM-002CXCEU": true,
}

// addon: SensorAddOn / SensorAddOnPro — DHT22 (T, H), DS18B20 (T..T4),
// digital input (EX), analog input (PERC), voltmeter (VL and, with a custom
// expression, VX). Names come from the peripheral configuration.
func addon(periph, cfg, st node) MeterSet {
	var s MeterSet
	add := func(typ, id string, v float64) {
		s.Values = append(s.Values, MeterValue{Type: typ, Value: v, Name: cfg.Path(id, "name").Str("")})
	}
	sorted := func(n node) []string {
		k := n.Keys()
		sort.Slice(k, func(i, j int) bool { return compIndex(k[i]) < compIndex(k[j]) })
		return k
	}
	tTypes := []string{T, T1, T2, T3, T4}
	tn := 0
	if dht := periph.Get("dht22"); dht.Len() > 0 {
		for _, id := range sorted(dht) {
			if strings.HasPrefix(id, "temperature") && tn < 5 {
				add(tTypes[tn], id, st.Path(id, "tC").Float())
				tn++
			}
		}
		for _, id := range sorted(dht) {
			if strings.HasPrefix(id, "humidity") {
				add(H, id, float64(st.Path(id, "rh").Int()))
			}
		}
	}
	for _, id := range sorted(periph.Get("ds18b20")) {
		if tn < 5 {
			add(tTypes[tn], id, st.Path(id, "tC").Float())
			tn++
		}
	}
	for _, id := range sorted(periph.Get("digital_in")) {
		v := 0.0
		if st.Path(id, "state").Bool() {
			v = 1
		}
		add(EX, id, v)
	}
	for _, id := range sorted(periph.Get("analog_in")) {
		add(PERC, id, st.Path(id, "percent").Float())
	}
	for _, id := range sorted(periph.Get("voltmeter")) {
		add(VL, id, st.Path(id, "voltage").Float())
		if x := st.Path(id, "xvoltage"); x.Exists() {
			add(VX, id, x.Float())
		}
	}
	return s
}

// compIndex: "temperature:101" → 101.
func compIndex(key string) int {
	_, n, ok := strings.Cut(key, ":")
	if !ok {
		return 0
	}
	v := 0
	for _, r := range n {
		if r < '0' || r > '9' {
			break
		}
		v = v*10 + int(r-'0')
	}
	return v
}

// UsesAddon reports whether the device reads SensorAddon.GetPeripherals: a
// fitted Sensor Add-on on a model that supports one, or the Plus UNI, whose
// add-on is integrated (ShellyPlusUNI: "integrated").
func UsesAddon(typeID, addonType string) bool {
	return typeID == "PlusUni" || (addonType == AddonSensor && addonModels[typeID])
}
