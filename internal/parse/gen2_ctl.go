// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// SensorAddOnPro.getDigitalOut and the Pro classes that add it to their
// modules; g3 PbSXT1St1820, PbSXT1St802 and modules/XT1Thermostat.

package parse

import (
	"fmt"
	"math"
	"strings"
)

// Models built with SensorAddOnPro that append the add-on digital output
// (switch:100) to their relays (ShellyPro1, Pro1PM, Pro2, Pro2PM, ProDimmer1,
// ProDimmer2, ProEM50).
var proAddonOut = map[string]bool{
	"Pro1": true, "Pro1ProAddon": true, "Pro1PM": true, "Pro1PMProAddon": true, "Pro2": true, "Pro2ProAddon": true,
	"Pro2PM": true, "Pro2PMProAddon": true, "ProDimmerx": true, "ProDimmerxProAddon": true, "ProEM": true, "ProEMProAddon": true,
}

// digitalOut: the first "switch:N" in the peripherals' digital_out, as a relay.
func (c *g2ctx) digitalOut(periph node) {
	for _, k := range periph.Get("digital_out").Keys() {
		var idx int
		if _, err := fmt.Sscanf(k, "switch:%d", &idx); err == nil {
			c.modules(c.relay(idx, ""))
			return
		}
	}
}

// XT1 service components per svc0.type (the keys each class asks
// Shelly.GetComponents for on every status refresh).
var xt1Keys = map[string][]string{
	"linkedgo-st1820-floor-thermostat": {"boolean:202", "number:200", "number:201", "number:202"},
	"linkedgo-st-802-hvac":             {"boolean:201", "number:200", "number:201", "number:202", "number:203", "enum:201"},
}

// XT1Keys returns the components to read for an XT1 variant, or nil.
func XT1Keys(variant string) []string { return xt1Keys[variant] }

// xt1: LinkedGo ST1820 (floor thermostat) and ST802 (HVAC; in "dry" mode the
// module is an on/off for the humidity target instead of a thermostat).
func xt1(c *g2ctx) {
	var enableID, targetID string
	switch c.variant {
	case "linkedgo-st1820-floor-thermostat":
		enableID, targetID = "202", "202"
	case "linkedgo-st-802-hvac":
		enableID, targetID = "201", "203"
	default:
		return // plain XT1: no modules or measures
	}
	comp := map[string]node{}
	list := c.comps.Get("components")
	for i := 0; i < list.Len(); i++ {
		comp[list.Idx(i).Get("key").Str("")] = list.Idx(i)
	}
	celsius := func(n node) bool { return n.Path("config", "meta", "ui", "unit").Str("") == "°C" }
	toC := func(f float64) float64 { return math.Round((f-32)*(5.0/9.0*10)) / 10 }

	cur := comp["number:201"]
	t := cur.Path("status", "value").Float()
	if !celsius(cur) {
		t = (t - 32) * 5 / 9
	}
	c.meters(set("", T, t, H, comp["number:200"].Path("status", "value").Float()))

	en := comp["boolean:"+enableID].Path("status", "value").Bool()
	if c.variant == "linkedgo-st-802-hvac" && strings.ToLower(comp["enum:201"].Path("status", "value").Str("")) == "dry" {
		hum := comp["number:202"].Path("status", "value").Int()
		c.modules(Module{Kind: KindRelay, Key: "boolean:" + enableID, Label: fmt.Sprintf("Humidity: %d%%", hum), On: ptr(en), InputOn: ptr(false)})
		return
	}
	tgt := comp["number:"+targetID]
	unit := "C"
	target, lo, hi := tgt.Path("status", "value").Float(), tgt.Path("config", "min").Float(), tgt.Path("config", "max").Float()
	if !celsius(tgt) {
		unit = "F"
		target, lo, hi = toC(target), toC(lo), toC(hi)
	}
	c.modules(Module{Kind: KindThermostat, Key: "xt1:" + enableID + ":" + targetID + ":" + unit, Enabled: ptr(en), Running: ptr(false),
		Target: ptr(target), Min: fptr(lo), Max: fptr(hi), Div: 2})
}
