package sim

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"time"
)

// varied: readings that swing with Device.Vary; temperatures swing a little. Voltage,
// energy totals and everything else stay as in the fixture.
var warm = map[string]bool{"tC": true, "tF": true, "temperature": true}

var varied = map[string]bool{
	"power": true, "apower": true, "act_power": true, "aprt_power": true, "current": true,
	"a_act_power": true, "b_act_power": true, "c_act_power": true, "total_act_power": true,
	"a_current": true, "b_current": true, "c_current": true, "total_current": true,
}

// vary returns the status with its power and current readings scaled by a slow
// swing of about ±35 % and its temperatures by ±3 %, the phase taken from the
// device id so devices differ.
func vary(b []byte, id string, now time.Time) []byte {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return b
	}
	h := fnv.New32a()
	h.Write([]byte(id))
	phase := float64(h.Sum32()%1000) / 1000 * 2 * math.Pi
	t := float64(now.UnixMilli()) / 1000
	f := 1 + 0.25*math.Sin(t/9+phase) + 0.1*math.Sin(t/2.3+2*phase)
	walk(v, f, 1+0.03*math.Sin(t/13+phase))
	if out, err := json.Marshal(v); err == nil {
		return out
	}
	return b
}

func walk(v any, f, ft float64) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if n, ok := e.(float64); ok && n != 0 && (varied[k] || warm[k]) {
				g := f
				if warm[k] {
					g = ft
				}
				x[k] = math.Round(n*g*100) / 100
				continue
			}
			walk(e, f, ft)
		}
	case []any:
		for _, e := range x {
			walk(e, f, ft)
		}
	}
}
