// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// AbstractG1Device.fillSettings/fillStatus, the fillStatus and meters of the
// classes in model/device/g1 and the g1 modules Relay, LightWhite, Roller,
// LightRGBW, LightBulbRGB, ThermostatG1 and Actions.

package parse

import (
	"math"
	"strconv"
)

// Gen1Input is what the service has read from a Gen1 device.
type Gen1Input struct {
	TypeID     string
	DeviceName string
	Settings   []byte // /settings
	Status     []byte // /status
	Actions    []byte // /settings/actions (i3, Button 1)
}

// Gen1 parses a Gen1 device.
func Gen1(in Gen1Input) Readings {
	set_, st := decode(in.Settings), decode(in.Status)
	r := gen1Common(set_, st)
	if f := gen1Models[in.TypeID]; f != nil {
		name := r.Name
		if name == "" {
			name = in.DeviceName
		}
		f(&g1ctx{set: set_, st: st, name: name, r: &r, actions: in.Actions})
		if r.Layout == "" {
			r.Layout = layoutOf(r.Modules)
		}
	}
	return r
}

// gen1Common: AbstractG1Device.fillSettings + fillStatus.
func gen1Common(set_, st node) Readings {
	r := Readings{Uptime: -1}
	r.Name = set_.Get("name").Str("")
	switch d := set_.Get("debug_enable"); {
	case !d.Exists():
		r.LogMode = "UNDEFINED"
	case d.Bool():
		r.LogMode = "FILE"
	default:
		r.LogMode = "NONE"
	}
	r.MQTTEnabled = set_.Path("mqtt", "enable").Bool()
	r.CloudEnabled = st.Path("cloud", "enabled").Bool()
	r.CloudConnected = st.Path("cloud", "connected").Bool()
	r.RSSI = st.Path("wifi_sta", "rssi").Int()
	r.SSID = st.Path("wifi_sta", "ssid").Str("")
	if up := st.Get("uptime"); up.Exists() {
		r.Uptime = up.Int()
	}
	r.MQTTConnected = st.Path("mqtt", "connected").Bool()
	return r
}

type g1ctx struct {
	set, st node
	name    string
	r       *Readings
	actions []byte
}

func (c *g1ctx) meters(s ...MeterSet) { c.r.Meters = append(c.r.Meters, s...) }
func (c *g1ctx) modules(m ...Module)  { c.r.Modules = append(c.r.Modules, m...) }
func (c *g1ctx) power(i int) float64  { return c.st.Get("meters").Idx(i).Get("power").Float() }
func (c *g1ctx) w(i int) MeterSet     { return set("", W, c.power(i)) }

func (c *g1ctx) tempAt(keys ...string) {
	if t := c.st.Path(keys...); t.Exists() {
		c.r.InternalTemp = ptr(t.Float())
	}
}

func (c *g1ctx) label(settingsArray string, i int) string {
	if n := c.set.Get(settingsArray).Idx(i).Get("name").Str(""); n != "" {
		return n
	}
	return c.name
}

// relay: g1 Relay ("ison", "source"; input state when given).
func (c *g1ctx) relay(i int, withInput bool) Module {
	s := c.st.Get("relays").Idx(i)
	m := Module{Kind: KindRelay, Index: i, Key: "relay/" + strconv.Itoa(i), Label: c.label("relays", i), On: ptr(s.Get("ison").Bool()), Source: s.Get("source").Str("-")}
	if withInput {
		m.InputOn = ptr(c.st.Get("inputs").Idx(i).Get("input").Int() != 0)
	}
	return m
}

// light: g1 LightWhite on /light/N (dimmers, DUO: minimum brightness 1).
func (c *g1ctx) light(i int, withInput bool) Module {
	s := c.st.Get("lights").Idx(i)
	m := Module{Kind: KindLight, Index: i, Key: "light/" + strconv.Itoa(i), Label: c.label("lights", i), On: ptr(s.Get("ison").Bool()),
		Brightness: ptr(s.Get("brightness").Int()), Source: s.Get("source").Str("-"), Min: fptr(1), Max: fptr(100)}
	if withInput {
		m.InputOn = ptr(c.st.Get("inputs").Idx(i).Get("input").Int() != 0)
	}
	return m
}

// white: g1 LightWhite on /white/N (RGBW2 white mode, minimum 0).
func (c *g1ctx) white(i int) Module {
	m := c.light(i, false)
	m.Key, m.Min = "white/"+strconv.Itoa(i), fptr(0)
	return m
}

// colorRGBW: g1 LightRGBW (RGBW2 colour mode); its label is the device name.
func (c *g1ctx) colorRGBW() Module {
	s := c.st.Get("lights").Idx(0)
	return Module{Kind: KindRGBW, Key: "color/0", Label: c.name, On: ptr(s.Get("ison").Bool()), Source: s.Get("source").Str("-"),
		RGB: []int{s.Get("red").Int(), s.Get("green").Int(), s.Get("blue").Int()}, Gain: ptr(s.Get("gain").Int()),
		Brightness: ptr(s.Get("gain").Int()), White: ptr(s.Get("white").Int())}
}

// bulbRGB: g1 LightBulbRGB (Bulb, DUO RGBW): colour mode with gain, white
// mode with brightness and temperature 3000–6500 K.
func (c *g1ctx) bulbRGB() Module {
	s := c.st.Get("lights").Idx(0)
	return Module{Kind: KindRGBCCT, Key: "light/0", Label: c.label("lights", 0), On: ptr(s.Get("ison").Bool()), Source: s.Get("source").Str("-"),
		ColorMode: ptr(s.Get("mode").Str("") == "color"), RGB: []int{s.Get("red").Int(), s.Get("green").Int(), s.Get("blue").Int()},
		Gain: ptr(s.Get("gain").Int()), Brightness: ptr(s.Get("brightness").Int()), TempK: ptr(s.Get("temp").Int()),
		Min: fptr(0), Max: fptr(100), TMin: 3000, TMax: 6500}
}

// roller: g1 Roller (label is the device name).
func (c *g1ctx) roller() Module {
	s := c.st.Get("rollers").Idx(0)
	cal := s.Get("positioning").Bool()
	pos := s.Get("current_pos").Int()
	if pos > 100 { // Roller.fillStatus: an out-of-range position means not calibrated
		cal = false
	}
	m := Module{Kind: KindCover, Key: "roller/0", Label: c.name, State: s.Get("state").Str(""), Calibrated: ptr(cal), Source: s.Get("source").Str("-")}
	if cal {
		m.Position = ptr(pos)
	}
	return m
}

// inputs: Actions input list (i3, Button 1) with the action URLs of each
// input, in the order of /settings/actions.
func (c *g1ctx) inputs() {
	events := gen1Actions(c.actions)
	for i := 0; i < c.st.Get("inputs").Len(); i++ {
		in := c.st.Get("inputs").Idx(i)
		c.modules(Module{Kind: KindInput, Index: i, Key: "input/" + strconv.Itoa(i), Label: c.label("inputs", i), InputOn: ptr(in.Get("input").Int() != 0),
			State: in.Get("event").Str(""), Enabled: ptr(true), Events: events[i]})
	}
}

// extSensors: the Shelly 1 / 1PM add-on meters, decided by /settings (and
// /status for the external switch), exactly as the classes build them.
func (c *g1ctx) extSensors() (MeterSet, bool) {
	extT := c.st.Get("ext_temperature")
	tC := func(k string) float64 { return extT.Path(k, "tC").Float() }
	switch {
	case c.set.Get("ext_humidity").Len() > 0:
		return set("", T, tC("0"), H, float64(c.st.Path("ext_humidity", "0", "hum").Int())), true
	case c.set.Get("ext_temperature").Len() > 0:
		s := MeterSet{}
		for i, typ := range []string{T, T1, T2} {
			if c.set.Get("ext_temperature").Has(strconv.Itoa(i)) {
				s.Values = append(s.Values, MeterValue{Type: typ, Value: tC(strconv.Itoa(i))})
			}
		}
		return s, true
	case c.st.Get("ext_switch").Len() > 0:
		closed := c.st.Path("ext_switch", "0", "input").Int() != 0
		if c.set.Get("ext_switch_reverse").Bool() {
			closed = !closed
		}
		v := 0.0
		if closed {
			v = 1
		}
		return set("", EX, v), true
	}
	return MeterSet{}, false
}

func sensorBat(c *g1ctx) float64 { return float64(c.st.Path("bat", "value").Int()) }

var gen1Models = map[string]func(c *g1ctx){
	"SHSW-1": func(c *g1ctx) { // Shelly1: [ext], then W from the configured load
		rel := c.relay(0, true)
		c.modules(rel)
		if ext, ok := c.extSensors(); ok {
			c.meters(ext)
		}
		if pow := c.set.Get("relays").Idx(0).Get("power").Float(); pow > 0 {
			v := 0.0
			if *rel.On {
				v = pow
			}
			c.meters(set("", W, v))
		}
	},
	"SHSW-PM": func(c *g1ctx) { // Shelly1PM: W, then [ext]
		c.modules(c.relay(0, true))
		c.tempAt("temperature")
		c.meters(c.w(0))
		if ext, ok := c.extSensors(); ok {
			c.meters(ext)
		}
	},
	"SHSW-L": func(c *g1ctx) {
		c.modules(c.relay(0, true))
		c.tempAt("temperature")
		c.meters(c.w(0))
	},
	"SHPLG-S": func(c *g1ctx) {
		c.modules(c.relay(0, false))
		c.tempAt("temperature")
		c.meters(c.w(0))
	},
	"SHPLG-1":  plug1,
	"SHPLG2-1": plug1,
	"SHPLG-U1": plug1,
	"SHSW-21": func(c *g1ctx) {
		c.meters(c.w(0))
		if c.set.Get("mode").Str("") == "relay" {
			c.modules(c.relay(0, true), c.relay(1, true))
		} else {
			c.modules(c.roller())
		}
	},
	"SHSW-25": func(c *g1ctx) {
		c.tempAt("temperature")
		c.meters(set("", W, c.power(0), V, c.st.Get("voltage").Float()), c.w(1))
		if c.set.Get("mode").Str("") == "relay" {
			c.modules(c.relay(0, true), c.relay(1, true))
		} else {
			c.modules(c.roller())
		}
	},
	"SHEM": func(c *g1ctx) {
		c.modules(c.relay(0, false))
		for i := 0; i < 2; i++ {
			e := c.st.Get("emeters").Idx(i)
			c.meters(set(c.set.Get("emeters").Idx(i).Get("name").Str(""), W, e.Get("power").Float(), VAR, e.Get("reactive").Float(),
				PF, e.Get("pf").Float(), V, e.Get("voltage").Float()))
		}
	},
	"SHEM-3": func(c *g1ctx) {
		c.modules(c.relay(0, false))
		for i := 0; i < 3; i++ {
			e := c.st.Get("emeters").Idx(i)
			c.meters(set(c.set.Get("emeters").Idx(i).Get("name").Str(""), W, e.Get("power").Float(), PF, e.Get("pf").Float(),
				V, e.Get("voltage").Float(), I, e.Get("current").Float()))
		}
	},
	"SHBLB-1":  func(c *g1ctx) { c.modules(c.bulbRGB()); c.meters(c.w(0)) },
	"SHCB-1":   func(c *g1ctx) { c.modules(c.bulbRGB()); c.meters(c.w(0)) },
	"SHBDUO-1": func(c *g1ctx) { c.modules(c.light(0, false)); c.meters(c.w(0)) },
	"SHDM-1":   dimmerG1,
	"SHDM-2":   dimmerG1,
	"SHRGBW2": func(c *g1ctx) {
		if c.set.Get("mode").Str("") == "color" {
			c.modules(c.colorRGBW())
			c.meters(c.w(0))
			return
		}
		for i := 0; i < 4; i++ {
			c.modules(c.white(i))
			c.meters(c.w(i))
		}
	},
	"SHIX3-1": func(c *g1ctx) { c.inputs() },
	"SHUNI-1": func(c *g1ctx) {
		c.modules(c.relay(0, true), c.relay(1, true))
		s := MeterSet{}
		extT := c.st.Get("ext_temperature")
		for i, typ := range []string{T, T1, T2} {
			if c.set.Get("ext_temperature").Has(strconv.Itoa(i)) {
				s.Values = append(s.Values, MeterValue{Type: typ, Value: extT.Path(strconv.Itoa(i), "tC").Float()})
			}
		}
		if c.set.Get("ext_humidity").Has("0") {
			s.Values = append(s.Values, MeterValue{Type: H, Value: float64(c.st.Path("ext_humidity", "0", "hum").Int())})
		}
		s.Values = append(s.Values, MeterValue{Type: V, Value: c.st.Get("adcs").Idx(0).Get("voltage").Float()})
		c.meters(s)
	},
	// battery devices
	"SHBTN-2": func(c *g1ctx) { c.meters(set("", BAT, sensorBat(c))); c.inputs() },
	"SHDW-1": func(c *g1ctx) {
		c.meters(set("", BAT, sensorBat(c)))
		c.modules(Module{Kind: KindSensor, Label: "Open", On: ptr(c.st.Path("sensor", "state").Str("") == "open")})
	},
	"SHDW-2": func(c *g1ctx) {
		c.meters(set("", BAT, sensorBat(c), T, c.st.Path("tmp", "tC").Float(), L, float64(c.st.Path("lux", "value").Int())))
		c.modules(Module{Kind: KindSensor, Label: "Open", On: ptr(c.st.Path("sensor", "state").Str("") == "open")})
	},
	"SHWT-1": func(c *g1ctx) {
		c.meters(set("", BAT, sensorBat(c), T, c.st.Path("tmp", "tC").Float()))
		c.modules(Module{Kind: KindSensor, Label: "Flood", On: ptr(c.st.Get("flood").Bool())})
	},
	"SHHT-1": func(c *g1ctx) {
		c.meters(set("", T, c.st.Path("tmp", "tC").Float(), H, float64(c.st.Path("hum", "value").Int()), BAT, sensorBat(c)))
	},
	"SHMOS-01": func(c *g1ctx) {
		c.meters(set("", L, float64(c.st.Path("lux", "value").Int()), BAT, sensorBat(c)))
		c.modules(Module{Kind: KindSensor, Label: "Motion", On: ptr(c.st.Path("sensor", "motion").Bool())})
	},
	"SHMOS-02": func(c *g1ctx) {
		c.meters(set("", T, c.st.Path("tmp", "value").Float(), L, float64(c.st.Path("lux", "value").Int()), BAT, sensorBat(c)))
		c.modules(Module{Kind: KindSensor, Label: "Motion", On: ptr(c.st.Path("sensor", "motion").Bool())})
	},
	"SHTRV-01": func(c *g1ctx) {
		// ThermostatG1: enabled is t_auto (settings); the label is the
		// current schedule profile; target 4–31 °C in 0.5 steps.
		th := c.st.Get("thermostats").Idx(0)
		ts := c.set.Get("thermostats").Idx(0)
		c.meters(set("", BAT, sensorBat(c), T, th.Path("tmp", "value").Float()))
		profile := ""
		if p := th.Get("schedule_profile").Int(); p >= 1 {
			profile = ts.Get("schedule_profile_names").Idx(p - 1).Str("")
		}
		c.modules(Module{Kind: KindThermostat, Key: "thermostats/0", Label: profile, Enabled: ptr(ts.Path("t_auto", "enabled").Bool()),
			Running: ptr(th.Get("pos").Float() > 0), Schedule: ptr(th.Get("schedule").Bool()), Target: ptr(th.Path("target_t", "value").Float()),
			Position: ptr(int(math.Round(th.Get("pos").Float()))), Min: fptr(4), Max: fptr(31), Div: 2})
		c.r.Layout = LayoutTRVG1
	},
}

func plug1(c *g1ctx) {
	c.modules(c.relay(0, false))
	c.meters(c.w(0))
}

func dimmerG1(c *g1ctx) {
	c.modules(c.light(0, true))
	c.tempAt("tmp", "tC")
	c.meters(c.w(0))
}
