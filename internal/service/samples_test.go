package service

import (
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
)

func TestSampleRing(t *testing.T) {
	m, _, ctx := newService(t, nil)
	_ = ctx
	now := time.Now().UnixMilli()
	temp := 41.5
	for i := 0; i < 5; i++ {
		m.recordSample(model.Device{ID: "X", Status: model.StatusOnline, LastSeen: now + int64(i)*1000, RSSI: -60 - i, InternalTemp: &temp})
	}
	m.recordSample(model.Device{ID: "X", Status: model.StatusOnline, LastSeen: now + 4000, RSSI: -1}) // no newer reading
	m.recordSample(model.Device{ID: "X", Status: model.StatusOffline, LastSeen: now + 9000})          // off line: not a reading
	got := m.Samples([]string{"X", "Y"}, 0)
	if len(got["X"]) != 5 || got["X"][4].RSSI != -64 || *got["X"][0].Temp != 41.5 || len(got["Y"]) != 0 {
		t.Fatalf("samples %+v", got)
	}
	if s := m.Samples([]string{"X"}, now+2000)["X"]; len(s) != 2 {
		t.Fatalf("since: %d", len(s))
	}
	old := SampleMaxLen
	SampleMaxLen = 3
	defer func() { SampleMaxLen = old }()
	m.recordSample(model.Device{ID: "X", Status: model.StatusOnline, LastSeen: now + 10000})
	if s := m.Samples([]string{"X"}, 0)["X"]; len(s) != 3 || s[2].T != now+10000 {
		t.Fatalf("capped: %+v", s)
	}
	m.ClearSamples([]string{"X"})
	if s := m.Samples([]string{"X"}, 0)["X"]; len(s) != 0 {
		t.Fatal("cleared")
	}
}

func TestEMEnergy(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plus1(t, m, ctx, map[string]string{
		"rpc_Shelly.GetConfig.json": `{"em1:0":{"id":0},"em1:1":{"id":1}}`,
		"rpc_EM1Data.GetData.json":  `{"data":[{"ts":1000,"period":60,"values":[[5,1],[7,2]]}],"next_record_ts":0}`,
	}, nil)
	list, err := m.EMEnergy(ctx, "AABBCC000002", 900, 2000)
	if err != nil || len(list) != 2 || list[0].Meter != "em1:0" || len(list[0].Data) != 2 || list[0].Data[1] != [2]float64{1060, 5} {
		t.Fatalf("%+v %v", list, err)
	}
}
