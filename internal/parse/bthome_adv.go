package parse

// BTHome v2 advertisements as a gateway relays them for BLU devices it is not
// managing (BLE.CloudRelay.ListInfos → sdata["fcd2"], base64). ShellyLanMan's
// own (DECISIONS P14-1): ShellyScanner lists such devices without readings.
// Format: https://bthome.io/format/ — one device-information byte, then
// objects (id byte + fixed-size little-endian value).

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"sort"
)

// AdvObject is one decoded BTHome object.
type AdvObject struct {
	ID    int     `json:"id"`
	Value float64 `json:"value"`
}

// Advert is one decoded BTHome v2 advertisement.
type Advert struct {
	Encrypted bool        `json:"encrypted,omitempty"`
	Trigger   bool        `json:"trigger,omitempty"` // sent on an event, not periodically
	PacketID  int         `json:"packetId"`
	Objects   []AdvObject `json:"objects,omitempty"`
}

// objSpec: value size in bytes (0: variable, length byte first), signedness, scale.
type objSpec struct {
	size   int
	signed bool
	scale  float64
}

var advObjects = map[int]objSpec{
	0x00: {1, false, 1}, 0x01: {1, false, 1}, 0x02: {2, true, 0.01}, 0x03: {2, false, 0.01},
	0x04: {3, false, 0.01}, 0x05: {3, false, 0.01}, 0x06: {2, false, 0.01}, 0x07: {2, false, 0.01},
	0x08: {2, true, 0.01}, 0x09: {1, false, 1}, 0x0A: {3, false, 0.001}, 0x0B: {3, false, 0.01},
	0x0C: {2, false, 0.001}, 0x0D: {2, false, 1}, 0x0E: {2, false, 1}, 0x12: {2, false, 1},
	0x13: {2, false, 1}, 0x14: {2, false, 0.01}, 0x2E: {1, false, 1}, 0x2F: {1, false, 1},
	0x3A: {1, false, 1}, 0x3C: {2, false, 1}, 0x3D: {2, false, 1}, 0x3E: {4, false, 1},
	0x3F: {2, true, 0.1}, 0x40: {2, false, 1}, 0x41: {2, false, 0.1}, 0x42: {3, false, 0.001},
	0x43: {2, false, 0.001}, 0x44: {2, false, 0.01}, 0x45: {2, true, 0.1}, 0x46: {1, false, 0.1},
	0x47: {2, false, 0.1}, 0x48: {2, false, 1}, 0x49: {2, false, 0.001}, 0x4A: {2, false, 0.1},
	0x4B: {3, false, 0.001}, 0x4C: {4, false, 0.001}, 0x4D: {4, false, 0.001}, 0x4E: {4, false, 0.001},
	0x4F: {4, false, 0.001}, 0x50: {4, false, 1}, 0x51: {2, false, 0.001}, 0x52: {2, false, 0.001},
	0x53: {0, false, 1}, 0x54: {0, false, 1}, 0x55: {4, false, 0.001}, 0x56: {2, false, 1},
	0x57: {1, true, 1}, 0x58: {1, true, 0.35}, 0x59: {1, true, 1}, 0x5A: {2, true, 1},
	0x5B: {4, true, 1}, 0x5C: {4, true, 0.01}, 0x5D: {2, true, 0.001}, 0x5E: {2, false, 0.01},
	0x5F: {2, false, 0.1}, 0x60: {1, false, 1}, 0x61: {2, false, 1},
	0xF0: {2, false, 1}, 0xF1: {4, false, 1}, 0xF2: {3, false, 1},
}

func init() { // the binary sensors in 0x0F–0x2D are one byte each (0x12–0x14 are not binary)
	for id := 0x0F; id <= 0x2D; id++ {
		if _, ok := advObjects[id]; !ok {
			advObjects[id] = objSpec{1, false, 1}
		}
	}
}

// DecodeBTHomeBase64 decodes the base64 service data of a relayed device.
func DecodeBTHomeBase64(s string) (Advert, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return Advert{}, err
	}
	return DecodeBTHome(b)
}

// DecodeBTHome decodes BTHome v2 service data. Objects after an unknown id are
// dropped (their size is unknown); an encrypted payload yields no objects.
func DecodeBTHome(b []byte) (Advert, error) {
	if len(b) == 0 {
		return Advert{}, errors.New("empty BTHome data")
	}
	info := b[0]
	if info>>5 != 2 {
		return Advert{}, fmt.Errorf("BTHome version %d not supported", info>>5)
	}
	a := Advert{Encrypted: info&1 == 1, Trigger: info&4 == 4, PacketID: -1}
	if a.Encrypted {
		return a, nil
	}
	for i := 1; i < len(b); {
		id := int(b[i])
		spec, ok := advObjects[id]
		if !ok {
			break
		}
		i++
		size := spec.size
		if size == 0 { // text, raw: length byte, then the bytes
			if i >= len(b) {
				break
			}
			size = int(b[i]) + 1
		}
		if i+size > len(b) {
			break
		}
		if spec.size != 0 {
			var u uint64
			for k := size - 1; k >= 0; k-- {
				u = u<<8 | uint64(b[i+k])
			}
			v := float64(u)
			if spec.signed && u&(1<<(8*size-1)) != 0 {
				v = float64(int64(u) - int64(1)<<(8*size))
			}
			v = math.Round(v*spec.scale*1000) / 1000
			if id == 0x00 {
				a.PacketID = int(v)
			} else {
				a.Objects = append(a.Objects, AdvObject{ID: id, Value: v})
			}
		}
		i += size
	}
	return a, nil
}

// Object returns the first object with the id.
func (a Advert) Object(id int) (float64, bool) {
	for _, o := range a.Objects {
		if o.ID == id {
			return o.Value, true
		}
	}
	return 0, false
}

func (a Advert) count(id int) int {
	n := 0
	for _, o := range a.Objects {
		if o.ID == id {
			n++
		}
	}
	return n
}

// ModelID is the device type id (object 0xF0) Shelly BLU devices send at
// power-on and every 6 hours, or 0.
func (a Advert) ModelID() int {
	if v, ok := a.Object(0xF0); ok {
		return int(v)
	}
	return 0
}

// Estimate guesses the device from the objects it sends; "" when it cannot.
// Different models can send the same objects (Wall Switch 4 / RC Button 4).
func (a Advert) Estimate() string {
	has := func(id int) bool { _, ok := a.Object(id); return ok }
	switch {
	case a.count(objButton) == 4:
		return "Blu Wall Switch 4 / RC Button 4"
	case has(objWindow) && has(0x3F):
		return "Blu Door Window"
	case has(objMotion):
		return "Blu Motion"
	case (has(0x45) || has(0x02)) && (has(0x2E) || has(0x03)):
		return "Blu H&T"
	case a.count(objButton) == 1 && len(a.Objects) <= 2:
		return "Blu Button"
	}
	return ""
}

// buttonEvent: BTHome button event names (docs-ble, "Button press events").
func buttonEvent(v int) string {
	switch v {
	case 0x01:
		return "press"
	case 0x02:
		return "double_press"
	case 0x03:
		return "triple_press"
	case 0x04:
		return "long_press"
	case 0x05:
		return "long_double_press"
	case 0x06:
		return "long_triple_press"
	case 0x80, 0xFE:
		return "hold"
	}
	return ""
}

// BTHomeRelayed turns a relayed advertisement into readings like a BTHome row
// (BTHome above): one meter set with the measurements, inputs for the buttons
// (their last event), sensors for motion and door/window.
func BTHomeRelayed(a Advert) Readings {
	r := Readings{Uptime: -1}
	var ms MeterSet
	tSeq, angSeq := []string{T, T1, T2, T3, T4}, []string{ANG, ANG1, ANG2}
	tn, an, bn := 0, 0, 0
	for _, o := range a.Objects {
		switch o.ID {
		case objButton:
			r.Modules = append(r.Modules, Module{Kind: KindInput, Index: bn, Key: fmt.Sprintf("button:%d", bn),
				Label: fmt.Sprintf("%d", bn+1), State: buttonEvent(int(o.Value)), Enabled: ptr(true)})
			bn++
		case objMotion:
			r.Modules = append(r.Modules, Module{Kind: KindSensor, Label: "Motion", On: ptr(o.Value != 0)})
		case objWindow:
			r.Modules = append(r.Modules, Module{Kind: KindSensor, Label: "Open", On: ptr(o.Value != 0)})
		default:
			typ := bthomeMeterType(o.ID)
			switch o.ID { // two more objects Shelly BLU devices use (0.01 °C / %)
			case 0x02:
				typ = T
			case 0x03:
				typ = HD
			}
			if typ == "" {
				continue
			}
			if typ == T {
				if tn >= len(tSeq) {
					continue
				}
				typ, tn = tSeq[tn], tn+1
			}
			if typ == ANG {
				if an >= len(angSeq) {
					continue
				}
				typ, an = angSeq[an], an+1
			}
			ms.Values = append(ms.Values, MeterValue{Type: typ, Value: o.Value})
		}
	}
	if len(ms.Values) > 0 {
		sort.SliceStable(ms.Values, func(i, j int) bool { return ms.Values[i].Type == BAT && ms.Values[j].Type != BAT })
		r.Meters = []MeterSet{ms}
	}
	r.Layout = layoutOf(r.Modules)
	return r
}
