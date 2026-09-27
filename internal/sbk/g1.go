// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// restore(JsonNode settings, List<String> errors) of every class in
// model/device/g1 and the restore of g1/modules Relay, Roller, LightWhite,
// LightRGBW, LightBulbRGB and ThermostatG1.

package sbk

import (
	"context"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/ojson"
)

type g1Restore func(ctx context.Context, d *Device, s *ojson.Value, e *errs)

// g1Settings: "/settings?" + the listed parameters (+ extra).
func g1Settings(ctx context.Context, d *Device, s *ojson.Value, e *errs, extra string, pars ...string) {
	e.add(d.cmd(ctx, "/settings?"+nodePars(s, pars...)+extra))
}

// relayG1: Relay.restore (without the state fields).
func relayG1(ctx context.Context, d *Device, s *ojson.Value, i int, e *errs) {
	r := s.Get("relays").Idx(i).Clone()
	for _, k := range []string{"ison", "has_timer", "overpower"} {
		r.Remove(k)
	}
	e.add(d.cmd(ctx, "/settings/relay/"+itoa(i)+"?"+entryPars(r)))
}

// rollerG1: Roller.restore (without state and calibration fields).
func rollerG1(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
	r := s.Get("rollers").Idx(0).Clone()
	for _, k := range []string{"state", "power", "is_valid", "safety_switch", "safety_mode", "safety_action", "safety_allowed_on_trigger", "positioning"} {
		r.Remove(k)
	}
	e.add(d.cmd(ctx, "/settings/roller/0?"+entryPars(r)))
}

// lightG1: LightWhite / LightRGBW / LightBulbRGB.restore — prefix is
// "/white/N", "/light/N" or "/color/N"; drop lists the removed fields.
func lightG1(ctx context.Context, d *Device, data *ojson.Value, prefix string, e *errs, drop ...string) *string {
	l := data.Clone()
	for _, k := range drop {
		l.Remove(k)
	}
	if l.Len() == 0 {
		return nil
	}
	return d.cmd(ctx, "/settings"+prefix+"?"+entryPars(l))
}

func nightMode(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
	nm := s.Get("night_mode")
	if nm.Get("enabled").Bool() {
		e.add(d.cmd(ctx, "/settings/night_mode?"+nodePars(nm, "enabled", "start_time", "end_time", "brightness")))
	} else {
		e.add(d.cmd(ctx, "/settings/night_mode?enabled=false"))
	}
}

// extSensors: the add-on sensors of Shelly 1 / 1PM (ext_temperature 0..2,
// ext_humidity 0, ext_switch 0).
func extSensors(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
	for i := 0; i < 3; i++ {
		e.add(d.cmd(ctx, "/settings/ext_temperature/"+itoa(i)+"?"+entryPars(s.Path("ext_temperature", itoa(i)))))
	}
	e.add(d.cmd(ctx, "/settings/ext_humidity/0?"+entryPars(s.Path("ext_humidity", "0"))))
	e.add(d.cmd(ctx, "/settings/ext_switch/0?"+entryPars(s.Path("ext_switch", "0"))))
}

// extUnit: "&ext_sensors_temperature_unit=" — the original appends the JSON
// node (with quotes); we send the value (FEATURE_PARITY O23).
func extUnit(s *ojson.Value) string {
	return "&ext_sensors_temperature_unit=" + s.Path("ext_sensors", "temperature_unit").Text()
}

func emeters(ctx context.Context, d *Device, s *ojson.Value, n int, e *errs) {
	for i := 0; i < n; i++ {
		e.add(d.cmd(ctx, "/settings/emeters/"+itoa(i)+"?"+nodePars(s.Get("emeters").Idx(i), "name", "appliance_type", "max_power")))
	}
}

// javaFloat formats like Java's Float.toString for ordinary values ("60.0").
func javaFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 32)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

var g1Models = map[string]g1Restore{
	"SHBTN-2": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) { // Button1
		g1Settings(ctx, d, s, e, "&multipush_time_between_pushes_ms_max="+itoa(s.Path("multipush_time_between_pushes_ms", "max").Int())+
			"&longpush_duration_ms_max="+itoa(s.Path("longpush_duration_ms", "max").Int()), "remain_awake", "led_status_disable")
		e.add(d.cmd(ctx, "/settings/input/0?name="+q(s.Get("inputs").Idx(0).Get("name").Text())))
	},
	"SHSW-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, extUnit(s), "longpush_time", "factory_reset_from_switch", "wifirecovery_reboot_enabled", "ext_switch_enable", "ext_switch_reverse")
		relayG1(ctx, d, s, 0, e)
		e.add(d.cmd(ctx, "/settings/power/0?power="+javaFloat(s.Get("relays").Idx(0).Get("power").Float())))
		extSensors(ctx, d, s, e)
	},
	"SHSW-L": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "longpush_time", "factory_reset_from_switch", "max_power", "supply_voltage", "led_status_disable")
		relayG1(ctx, d, s, 0, e)
	},
	"SHSW-PM": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, extUnit(s), "longpush_time", "factory_reset_from_switch", "max_power", "supply_voltage", "power_correction",
			"led_status_disable", "wifirecovery_reboot_enabled", "ext_switch_enable", "ext_switch_reverse")
		relayG1(ctx, d, s, 0, e)
		extSensors(ctx, d, s, e)
	},
	"SHSW-21": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "longpush_time", "factory_reset_from_switch", "mode", "wifirecovery_reboot_enabled")
		relaysOrRoller(ctx, d, s, e)
	},
	"SHSW-25": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "led_status_disable", "longpush_time", "factory_reset_from_switch", "mode", "wifirecovery_reboot_enabled")
		relaysOrRoller(ctx, d, s, e)
	},
	"SHEM-3": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "led_status_disable", "wifirecovery_reboot_enabled")
		relayG1(ctx, d, s, 0, e)
		emeters(ctx, d, s, 3, e)
	},
	"SHEM": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "led_status_disable", "wifirecovery_reboot_enabled")
		emeters(ctx, d, s, 2, e)
		relayG1(ctx, d, s, 0, e)
	},
	"SHBLB-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) { // ShellyBulb
		g1Settings(ctx, d, s, e, "", "mode")
		e.add(lightG1(ctx, d, s.Get("lights").Idx(0), "/light/0", e, "ison", "has_timer", "mode"))
	},
	"SHBDUO-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "transition")
		nightMode(ctx, d, s, e)
		e.add(lightG1(ctx, d, s.Get("lights").Idx(0), "/light/0", e, "ison"))
	},
	"SHCB-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) { // ShellyDUORGB
		g1Settings(ctx, d, s, e, "", "mode")
		nightMode(ctx, d, s, e)
		e.add(lightG1(ctx, d, s.Get("lights").Idx(0), "/light/0", e, "ison", "has_timer", "mode"))
	},
	"SHDW-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "dark_threshold", "twilight_threshold", "led_status_disable", "lux_wakeup_enable", "tilt_enabled",
			"vibration_enabled", "vibration_sensitivity", "reverse_open_close")
	},
	"SHDW-2": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "&"+nodePars(s.Get("sensor"), "temperature_threshold", "temperature_units"), "dark_threshold", "twilight_threshold",
			"led_status_disable", "lux_wakeup_enable", "tilt_enabled", "vibration_enabled", "vibration_sensitivity", "reverse_open_close", "temperature_offset")
	},
	"SHDM-1": dimmerG1, "SHDM-2": dimmerG1,
	"SHWT-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) { // ShellyFlood
		// The original concatenates the second half outside the request, so it
		// is never sent (FEATURE_PARITY O24); we send both in one request.
		e.add(d.cmd(ctx, "/settings?"+nodePars(s.Get("sensors"), "temperature_units", "temperature_threshold")+"&"+nodePars(s, "rain_sensor", "temperature_offset")))
	},
	"SHHT-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		sen := s.Get("sensors")
		g1Settings(ctx, d, s, e, "&"+nodePars(sen, "temperature_threshold", "humidity_threshold")+"&temperature_units="+sen.Get("temperature_unit").Text(),
			"external_power", "temperature_offset", "humidity_offset")
	},
	"SHIX3-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "&multipush_time_between_pushes_ms_max="+itoa(s.Path("multipush_time_between_pushes_ms", "max").Int())+
			"&longpush_duration_ms_max="+itoa(s.Path("longpush_duration_ms", "max").Int())+
			"&longpush_duration_ms_min="+itoa(s.Path("longpush_duration_ms", "min").Int()), "led_status_disable")
		for i := 0; i < 3; i++ {
			e.add(d.cmd(ctx, "/settings/input/"+itoa(i)+"?"+nodePars(s.Get("inputs").Idx(i), "name", "btn_type", "btn_reverse")))
		}
	},
	"SHMOS-01": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, motionPars(s.Get("motion")), "led_status_disable", "tamper_sensitivity", "dark_threshold", "twilight_threshold")
	},
	"SHMOS-02": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		sen := s.Get("sensors")
		g1Settings(ctx, d, s, e, motionPars(s.Get("motion"))+"&sensors.temperature_unit="+sen.Get("temperature_unit").Text()+
			"&sensors.temperature_threshohld="+sen.Get("temperature_threshohld").Text(),
			"led_status_disable", "tamper_sensitivity", "dark_threshold", "twilight_threshold", "temperature_offset")
	},
	"SHPLG-1": plugG1, "SHPLG2-1": plugG1, "SHPLG-U1": plugG1,
	"SHPLG-S": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "led_status_disable", "led_power_disable", "wifirecovery_reboot_enabled")
		relayG1(ctx, d, s, 0, e)
	},
	"SHRGBW2": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "", "mode", "led_status_disable", "factory_reset_from_switch")
		if s.Get("night_mode").Exists() {
			nightMode(ctx, d, s, e)
		}
		// The original ignores the results of the light restores.
		if s.Get("mode").Text() == "color" {
			lightG1(ctx, d, s.Get("lights").Idx(0), "/color/0", e, "ison")
		} else {
			for i := 0; i < 4; i++ {
				lightG1(ctx, d, s.Get("lights").Idx(i), "/white/"+itoa(i), e, "ison")
			}
		}
	},
	"SHTRV-01": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		disp := s.Get("display")
		e.add(d.cmd(ctx, "/settings?child_lock="+s.Get("child_lock").Text()+"&display_brightness="+disp.Get("brightness").Text()+"&display_flipped="+disp.Get("flipped").Text()))
		th := s.Get("thermostats").Idx(0)
		e.add(d.cmd(ctx, "/settings/thermostats/0?"+nodePars(th, "temperature_offset")+"&ext_t_enabled="+boolText(th.Path("ext_t", "enabled"))))
	},
	"SHUNI-1": func(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
		g1Settings(ctx, d, s, e, "&ext_sensors_temperature_unit="+s.Path("ext_sensors", "temperature_unit").Text(), "longpush_time", "factory_reset_from_switch")
		arrayAnswer := func(r *string) *string { // the UNI answers these with an array
			if r != nil && strings.HasPrefix(*r, "[") {
				return nil
			}
			return r
		}
		for i := 0; i < 3; i++ {
			if t := s.Path("ext_temperature", itoa(i)); t.NonNull() && t.Idx(0).Exists() {
				e.add(arrayAnswer(d.cmd(ctx, "/settings/ext_temperature/"+itoa(i)+"?"+entryPars(t.Idx(0)))))
			}
		}
		if h := s.Path("ext_humidity", "0"); h.NonNull() && h.Idx(0).Exists() {
			e.add(arrayAnswer(d.cmd(ctx, "/settings/ext_humidity/0?"+entryPars(h.Idx(0)))))
		}
		relayG1(ctx, d, s, 0, e)
		relayG1(ctx, d, s, 1, e)
		adc := s.Get("adcs").Idx(0)
		e.add(d.cmd(ctx, "/settings/adc/0?range="+adc.Get("range").Text()+"&offset="+adc.Get("offset").Text()))
		for i, act := range adc.Get("relay_actions").Items() {
			e.add(d.cmd(ctx, "/settings/adc/0/relay_actions."+itoa(i)+"?"+entryPars(act)))
		}
	},
}

func relaysOrRoller(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
	if s.Get("mode").Text() == "relay" {
		relayG1(ctx, d, s, 0, e)
		relayG1(ctx, d, s, 1, e)
	} else {
		rollerG1(ctx, d, s, e)
	}
}

func plugG1(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
	g1Settings(ctx, d, s, e, "", "wifirecovery_reboot_enabled")
	relayG1(ctx, d, s, 0, e)
}

func dimmerG1(ctx context.Context, d *Device, s *ojson.Value, e *errs) {
	g1Settings(ctx, d, s, e, "", "led_status_disable", "factory_reset_from_switch", "pulse_mode", "transition", "fade_rate", "min_brightness", "zcross_debounce")
	nightMode(ctx, d, s, e)
	wu := s.Get("warm_up")
	if wu.Get("enabled").Bool() {
		e.add(d.cmd(ctx, "/settings/warm_up?"+nodePars(wu, "enabled", "brightness", "time")))
	} else {
		e.add(d.cmd(ctx, "/settings/warm_up?enabled=false"))
	}
	e.add(lightG1(ctx, d, s.Get("lights").Idx(0), "/light/0", e, "ison"))
}

func motionPars(m *ojson.Value) string {
	return "&motion.sensitivity=" + m.Get("sensitivity").Text() + "&motion.blind_time_minutes=" + m.Get("blind_time_minutes").Text() +
		"&motion.pulse_count=" + m.Get("pulse_count").Text() + "&motion.operating_mode=" + m.Get("operating_mode").Text() +
		"&motion.enabled=" + m.Get("enabled").Text()
}

func itoa(i int) string { return strconv.Itoa(i) }
