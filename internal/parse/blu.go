// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// blu/modules/Sensor (BTHome object id → meter type), SensorsCollection,
// DWSensor, MotionSensor, InputSensor and blu/BluTRV.

package parse

import (
	"sort"
	"strings"
)

// BTHome object ids with a special module instead of a measurement.
const (
	objButton = 0x3A
	objMotion = 0x21
	objWindow = 0x2D
)

// bthomeMeterType maps a BTHome object id to ShellyScanner's meter type.
func bthomeMeterType(obj int) string {
	switch obj {
	case 0x01:
		return BAT
	case 0x05:
		return L
	case 0x15:
		return BATE
	case 0x1E:
		return LIGHT
	case 0x2C:
		return VIB
	case 0x2E:
		return H
	case 0x3F:
		return ANG
	case 0x40:
		return DMM
	case 0x43:
		return I
	case 0x45:
		return T
	case 0x4A:
		return V
	case 0x5C:
		return W
	case 0x5F:
		return RAIN
	case 0x64:
		return LE
	}
	return ""
}

// BTHomeInput is what the service has read through the gateway.
type BTHomeInput struct {
	Index        string // bthomedevice component index
	KnownObjects []byte // BTHomeDevice.GetKnownObjects
	Components   []byte // Shelly.GetComponents?keys=[device, sensors…] (one or more pages merged as {"components":[…]})
}

// BTHome parses a BLU BTHome device: RSSI, one meter set from the measuring
// sensors (repeated temperatures become T, T1…; angles ANG, ANG1…), and
// modules for buttons, motion and door/window sensors.
func BTHome(in BTHomeInput) Readings {
	r := Readings{Uptime: -1}
	objs := decode(in.KnownObjects).Get("objects")
	objOf := map[string]int{}
	var order []string
	for i := 0; i < objs.Len(); i++ {
		o := objs.Idx(i)
		comp := o.Get("component").Str("")
		if strings.HasPrefix(comp, "bthomesensor:") {
			objOf[comp] = o.Get("obj_id").Int()
			order = append(order, comp)
		}
	}
	comps := map[string]node{}
	list := decode(in.Components).Get("components")
	for i := 0; i < list.Len(); i++ {
		c := list.Idx(i)
		comps[c.Get("key").Str("")] = c
	}
	if dev := comps["bthomedevice:"+in.Index]; dev.Exists() {
		r.RSSI = dev.Path("status", "rssi").Int()
	}
	sort.SliceStable(order, func(i, j int) bool { return compIndex(order[i]) < compIndex(order[j]) })

	var ms MeterSet
	tSeq, angSeq := []string{T, T1, T2, T3, T4}, []string{ANG, ANG1, ANG2}
	tn, an := 0, 0
	for _, key := range order {
		c := comps[key]
		name := c.Path("config", "name").Str("")
		val := c.Path("status", "value")
		switch obj := objOf[key]; obj {
		case objButton:
			r.Modules = append(r.Modules, Module{Kind: KindInput, Index: compIndex(key), Label: name, State: val.Str("")})
		case objMotion:
			r.Modules = append(r.Modules, Module{Kind: KindSensor, Label: labelOr(name, "Motion"), On: ptr(val.Bool())})
		case objWindow:
			r.Modules = append(r.Modules, Module{Kind: KindSensor, Label: labelOr(name, "Open"), On: ptr(val.Bool())})
		default:
			typ := bthomeMeterType(obj)
			if typ == "" {
				continue
			}
			if typ == T {
				if tn >= len(tSeq) {
					continue
				}
				typ = tSeq[tn]
				tn++
			}
			if typ == ANG {
				if an >= len(angSeq) {
					continue
				}
				typ = angSeq[an]
				an++
			}
			ms.Values = append(ms.Values, MeterValue{Type: typ, Value: val.Float(), Name: name})
		}
	}
	if len(ms.Values) > 0 {
		r.Meters = []MeterSet{ms}
	}
	return r
}

func labelOr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// BluTRVInput holds the gateway answers for a BLU TRV.
type BluTRVInput struct {
	Name         string
	Status       []byte // BluTrv.GetStatus
	RemoteStatus []byte // BluTrv.GetRemoteStatus
	RemoteConfig []byte // BluTrv.GetRemoteConfig (may be nil between configuration refreshes)
}

// BluTRV parses a BLU TRV: T (measured) and BAT, uptime, and the thermostat.
func BluTRV(in BluTRVInput) Readings {
	r := Readings{Uptime: -1}
	st := decode(in.Status)
	rs := decode(in.RemoteStatus).Get("status")
	r.RSSI = st.Get("rssi").Int()
	if up := rs.Path("sys", "uptime"); up.Exists() {
		r.Uptime = up.Int()
	}
	trv := rs.Get("trv:0")
	r.Meters = []MeterSet{set("", T, trv.Get("current_C").Float(), BAT, float64(st.Get("battery").Int()))}
	m := Module{Kind: KindThermostat, Label: in.Name, Target: ptr(trv.Get("target_C").Float()), Position: ptr(trv.Get("pos").Int())}
	if in.RemoteConfig != nil {
		m.On = ptr(decode(in.RemoteConfig).Path("config", "trv:0", "enable").Bool())
	}
	r.Modules = []Module{m}
	return r
}
