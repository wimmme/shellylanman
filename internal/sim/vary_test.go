package sim

import (
	"encoding/json"
	"testing"
	"time"
)

func TestVary(t *testing.T) {
	in := []byte(`{"switch:0":{"apower":100,"voltage":230,"aenergy":{"total":5},"temperature":{"tC":50}},"meters":[{"power":0}]}`)
	seen := map[float64]bool{}
	for s := 0; s < 20; s++ {
		var v struct {
			Sw struct {
				APower  float64 `json:"apower"`
				Voltage float64 `json:"voltage"`
				AEnergy struct {
					Total float64 `json:"total"`
				} `json:"aenergy"`
				Temp struct {
					TC float64 `json:"tC"`
				} `json:"temperature"`
			} `json:"switch:0"`
			Meters []struct {
				Power float64 `json:"power"`
			} `json:"meters"`
		}
		if err := json.Unmarshal(vary(in, "AABBCC000002", time.Unix(int64(s*3), 0)), &v); err != nil {
			t.Fatal(err)
		}
		if v.Sw.Voltage != 230 || v.Sw.AEnergy.Total != 5 || v.Meters[0].Power != 0 {
			t.Fatalf("only non-zero power and current swing: %+v", v)
		}
		if v.Sw.APower < 50 || v.Sw.APower > 150 || v.Sw.Temp.TC < 48 || v.Sw.Temp.TC > 52 {
			t.Fatalf("swing too wide: %v %v", v.Sw.APower, v.Sw.Temp.TC)
		}
		seen[v.Sw.APower] = true
	}
	if len(seen) < 10 {
		t.Fatalf("the reading hardly moves: %v", seen)
	}
}
