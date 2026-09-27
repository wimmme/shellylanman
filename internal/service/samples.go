// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// g2/modules/EMManager.getEnergyData and EM1Manager.getEnergyData.

package service

// Charts: the original samples a device only while its chart window is open;
// here every status update of an on-line device is kept in a per-device
// in-memory ring buffer, so a chart opens with history (DECISIONS Q7). The
// polling rate already slows down while no browser is connected (Q8).

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/parse"
)

// Ring buffer limits per device.
var (
	SampleKeep   = 24 * time.Hour
	SampleMaxLen = 20000
)

// Sample is one status reading for the charts.
type Sample struct {
	T      int64            `json:"t"` // unix ms (the device's last contact)
	RSSI   int              `json:"rssi"`
	Temp   *float64         `json:"temp,omitempty"`
	Meters []parse.MeterSet `json:"meters,omitempty"`
}

type sampleRing struct {
	mu   sync.Mutex
	data map[string][]Sample
}

func (m *Devices) samples() *sampleRing {
	m.samplesOnce.Do(func() { m.ring.data = map[string][]Sample{} })
	return &m.ring
}

// recordSample runs on every device update.
func (m *Devices) recordSample(d model.Device) {
	if d.Status != model.StatusOnline || d.LastSeen == 0 {
		return
	}
	r := m.samples()
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.data[d.ID]
	if n := len(list); n > 0 && list[n-1].T >= d.LastSeen {
		return // no new reading
	}
	list = append(list, Sample{T: d.LastSeen, RSSI: d.RSSI, Temp: d.InternalTemp, Meters: d.Meters})
	cut := 0
	oldest := time.Now().Add(-SampleKeep).UnixMilli()
	for cut < len(list) && (list[cut].T < oldest || len(list)-cut > SampleMaxLen) {
		cut++
	}
	if cut > 0 {
		list = append([]Sample(nil), list[cut:]...)
	}
	r.data[d.ID] = list
}

// Samples returns the kept readings of the given devices, newer than since (unix ms).
func (m *Devices) Samples(ids []string, since int64) map[string][]Sample {
	r := m.samples()
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string][]Sample{}
	for _, id := range ids {
		var sel []Sample
		for _, s := range r.data[id] {
			if s.T > since {
				sel = append(sel, s)
			}
		}
		if sel == nil {
			sel = []Sample{}
		}
		out[id] = sel
	}
	return out
}

// ClearSamples empties the buffers of the given devices (the chart's "Clear").
func (m *Devices) ClearSamples(ids []string) {
	r := m.samples()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		delete(r.data, id)
	}
}

// EMSeries is the energy of one EM meter: one line (EM1Data) or three (EMData phases a, b, c).
type EMSeries struct {
	Meter string       `json:"meter"` // "em:0" or "em1:N"
	Lines int          `json:"lines"`
	Data  [][2]float64 `json:"data"` // per line in turn: [unix s, Wh]
}

// EMEnergy reads active energy (minus returned energy) per record, for the
// EM chart: EMData.GetData (triphase profile, 3 lines) or EM1Data.GetData per
// em1 channel, following next_record_ts.
func (m *Devices) EMEnergy(ctx context.Context, id string, start, end int64) ([]EMSeries, error) {
	c, _, err := m.g2conn(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	cfg, _ := ojson.Parse(m.devs[id].rawConfig)
	m.mu.Unlock()
	var out []EMSeries
	read := func(method string, idx, lines int, pairs [][2]int) (EMSeries, error) {
		s := EMSeries{Meter: map[bool]string{true: "em:", false: "em1:"}[lines == 3] + strconv.Itoa(idx), Lines: lines}
		next := start
		for next > 0 {
			v, err := m.rpcGet(ctx, c, "/rpc/"+method+"?add_keys=false&id="+strconv.Itoa(idx)+"&ts="+strconv.FormatInt(next, 10)+"&end_ts="+strconv.FormatInt(end, 10))
			if err != nil {
				return s, err
			}
			for _, rec := range v.Get("data").Items() {
				ts := rec.Get("ts").Float()
				period := rec.Get("period").Float()
				for _, vals := range rec.Get("values").Items() {
					for _, p := range pairs {
						s.Data = append(s.Data, [2]float64{ts, vals.Idx(p[0]).Float() - vals.Idx(p[1]).Float()})
					}
					ts += period
				}
			}
			n := int64(v.Get("next_record_ts").Int())
			if n <= next {
				break
			}
			next = n
		}
		return s, nil
	}
	if cfg.Get("em:0").Exists() {
		// EMManager indexes: a/b/c total_act_energy and total_act_ret_energy
		s, err := read("EMData.GetData", 0, 3, [][2]int{{0, 2}, {16, 18}, {32, 34}})
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	for i := 0; i < 3; i++ {
		if !cfg.Get("em1:" + strconv.Itoa(i)).Exists() {
			continue
		}
		s, err := read("EM1Data.GetData", i, 1, [][2]int{{0, 1}})
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if out == nil {
		out = []EMSeries{}
	}
	return out, nil
}
