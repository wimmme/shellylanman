package parse

import (
	"reflect"
	"strings"
	"testing"
)

func kinds(r Readings) string {
	var k []string
	for _, m := range r.Modules {
		k = append(k, m.Kind+"@"+m.Key)
	}
	return strings.Join(k, " ")
}

// The layout follows the array type of getModules() in each Java class.
func TestLayouts(t *testing.T) {
	cases := []struct {
		name   string
		r      Readings
		layout string
		mods   string
	}{
		{"plug S", Gen1(Gen1Input{TypeID: "SHPLG-S", Settings: fixture(t, "gen1/SHPLG-S", "settings.json"), Status: fixture(t, "gen1/SHPLG-S", "status.json")}),
			LayoutRelay, "relay@relay/0"},
		{"RGBW2 white", Gen1(Gen1Input{TypeID: "SHRGBW2", Settings: []byte(`{"mode":"white"}`), Status: []byte(`{"lights":[{},{},{},{}]}`)}),
			LayoutMixed, "light@white/0 light@white/1 light@white/2 light@white/3"},
		{"RGBW2 colour", Gen1(Gen1Input{TypeID: "SHRGBW2", Settings: []byte(`{"mode":"color"}`), Status: []byte(`{"lights":[{"gain":40,"white":9}]}`)}),
			LayoutRGBW, "rgbw@color/0"},
		{"Bulb", Gen1(Gen1Input{TypeID: "SHBLB-1", Settings: []byte(`{}`), Status: []byte(`{"lights":[{"mode":"color","gain":30,"brightness":70,"temp":4000}]}`)}),
			LayoutRGBCCT, "rgbcct@light/0"},
		{"2PM cover", Gen2(Gen2Input{TypeID: "Plus2PM", Config: []byte(`{"sys":{"device":{"profile":"cover"}}}`), Status: []byte(`{"cover:0":{"pos_control":false},"input:1":{"state":true}}`)}),
			LayoutRoller, "cover@cover:0"},
		{"RGBWW rgbcct", Gen2(Gen2Input{TypeID: "ProRGBWWPM", Config: []byte(`{"sys":{"device":{"profile":"rgbcct"}},"cct:0":{"ct_range":[3000,5000]}}`), Status: []byte(`{}`)}),
			LayoutMixed, "rgb@rgb:0 cct@cct:0"},
		{"RGBCCT bulb", Gen2(Gen2Input{TypeID: "RGBCCTBulbG3", Config: []byte(`{}`), Status: []byte(`{"rgbcct:0":{"mode":"rgb","brightness":20,"rgb":[1,2,3]}}`)}),
			LayoutRGBCCT, "rgbcct@rgbcct:0"},
		{"i4", Gen2(Gen2Input{TypeID: "PlusI4", Config: []byte(`{"input:0":{"enable":true}}`), Status: []byte(`{}`)}),
			LayoutMixed, "input@input:0 input@input:1 input@input:2 input@input:3"},
		{"Wall Display thermostat", Gen2(Gen2Input{TypeID: "WallDisplay", Config: []byte(`{"thermostat:0":{"name":"Living"},"switch:0":{}}`), Status: []byte(`{"thermostat:0":{"enable":true,"target_C":21.5,"output":true},"switch:0":{}}`)}),
			LayoutThermostat, "thermostat@thermostat:0"},
		{"Wall Display relay", Gen2(Gen2Input{TypeID: "WallDisplay", Config: []byte(`{"switch:0":{}}`), Status: []byte(`{"switch:0":{"output":true}}`)}),
			LayoutRelay, "relay@switch:0"},
		{"Pro CB", Gen2(Gen2Input{TypeID: "ProCB", Config: []byte(`{"cb:0":{"name":""}}`), Status: []byte(`{"cb:0":{"output":true,"safety":true}}`)}),
			LayoutBreaker, "cb@cb:0"},
		{"Camera", Gen2(Gen2Input{TypeID: "Camera", Config: []byte(`{}`), Status: []byte(`{"camera:0":{"privacy":true,"motion":true}}`)}),
			LayoutMixed, "camera@camera:0"},
		{"TRV G1", Gen1(Gen1Input{TypeID: "SHTRV-01", Settings: []byte(`{"thermostats":[{"t_auto":{"enabled":true},"schedule_profile_names":["Living","Night"]}]}`),
			Status: []byte(`{"thermostats":[{"pos":12.4,"target_t":{"value":20.5},"schedule":true,"schedule_profile":2,"tmp":{"value":19}}]}`)}),
			LayoutTRVG1, "thermostat@thermostats/0"},
	}
	for _, c := range cases {
		if c.r.Layout != c.layout || kinds(c.r) != c.mods {
			t.Errorf("%s: layout %q modules %q, want %q %q", c.name, c.r.Layout, kinds(c.r), c.layout, c.mods)
		}
	}
}

func TestControlFields(t *testing.T) {
	bulb := Gen1(Gen1Input{TypeID: "SHBLB-1", Settings: []byte(`{}`), Status: []byte(`{"lights":[{"mode":"color","gain":30,"brightness":70,"temp":4000}]}`)}).Modules[0]
	if !*bulb.ColorMode || *bulb.Gain != 30 || *bulb.Brightness != 70 || *bulb.TempK != 4000 || bulb.TMin != 3000 || bulb.TMax != 6500 {
		t.Errorf("bulb %+v", bulb)
	}
	dim := Gen1(Gen1Input{TypeID: "SHDM-2", Settings: []byte(`{}`), Status: []byte(`{"lights":[{"brightness":5}]}`)}).Modules[0]
	if *dim.Min != 1 || dim.Key != "light/0" {
		t.Errorf("Gen1 dimmer minimum brightness %+v", dim)
	}
	cover := Gen2(Gen2Input{TypeID: "Plus2PM", Config: []byte(`{"sys":{"device":{"profile":"cover"}}}`), Status: []byte(`{"cover:0":{},"input:1":{"state":true}}`)}).Modules[0]
	if *cover.InputOn || !*cover.InputOn1 {
		t.Errorf("cover inputs %+v", cover)
	}
	roll := Gen1(Gen1Input{TypeID: "SHSW-25", Settings: []byte(`{"mode":"roller"}`), Status: []byte(`{"rollers":[{"positioning":true,"current_pos":101}]}`)}).Modules[0]
	if *roll.Calibrated || roll.Position != nil {
		t.Errorf("Gen1 roller with position > 100 must be uncalibrated: %+v", roll)
	}
	cct := Gen2(Gen2Input{TypeID: "ProRGBWWPM", Config: []byte(`{"sys":{"device":{"profile":"cctx2"}},"cct:0":{"ct_range":[3000,5000]}}`), Status: []byte(`{}`)}).Modules[0]
	if cct.TMin != 3000 || cct.TMax != 5000 {
		t.Errorf("ct_range %+v", cct)
	}
	wd := Gen2(Gen2Input{TypeID: "WallDisplay", Config: []byte(`{"thermostat:0":{"name":"Living"}}`), Status: []byte(`{"thermostat:0":{"enable":true,"target_C":21.5,"output":true}}`)}).Modules[0]
	if wd.Label != "Living" || !*wd.Enabled || !*wd.Running || *wd.Target != 21.5 || *wd.Min != 5 || *wd.Max != 35 || wd.Div != 2 {
		t.Errorf("wall display thermostat %+v", wd)
	}
	cb := Gen2(Gen2Input{TypeID: "ProCB", Config: []byte(`{"sys":{"device":{"name":"Board"}},"cb:0":{"name":""}}`), Status: []byte(`{"cb:0":{"safety":true}}`)}).Modules[0]
	if cb.Label != "" || !*cb.Locked {
		t.Errorf("CB label has no fallback, safety is the lock: %+v", cb)
	}
	trv := Gen1(Gen1Input{TypeID: "SHTRV-01", Settings: []byte(`{"thermostats":[{"t_auto":{"enabled":true},"schedule_profile_names":["Living","Night"]}]}`),
		Status: []byte(`{"thermostats":[{"pos":12.4,"target_t":{"value":20.5},"schedule":true,"schedule_profile":2}]}`)}).Modules[0]
	if trv.Label != "Night" || !*trv.Enabled || !*trv.Schedule || *trv.Target != 20.5 || *trv.Position != 12 || *trv.Min != 4 || *trv.Max != 31 {
		t.Errorf("TRV G1 %+v", trv)
	}
}

func TestProAddonDigitalOut(t *testing.T) {
	cfg := []byte(`{"sys":{"device":{"addon_type":"sensor"}},"switch:100":{"name":"Out"}}`)
	st := []byte(`{"switch:0":{"output":false},"switch:100":{"output":true}}`)
	per := []byte(`{"digital_out":{"switch:100":{}}}`)
	r := Gen2(Gen2Input{TypeID: "Pro1", Config: cfg, Status: st, Peripherals: per})
	if r.Layout != LayoutRelay || kinds(r) != "relay@switch:0 relay@switch:100" || r.Modules[1].Label != "Out" || !*r.Modules[1].On {
		t.Fatalf("Pro1 + digital out: %s %s %+v", r.Layout, kinds(r), r.Modules)
	}
	r = Gen2(Gen2Input{TypeID: "ProDimmerx", Config: cfg, Status: st, Peripherals: per})
	if r.Layout != LayoutMixed || kinds(r) != "light@light:0 relay@switch:100" {
		t.Fatalf("Pro Dimmer + digital out: %s %s", r.Layout, kinds(r))
	}
	r = Gen2(Gen2Input{TypeID: "Plus1", Config: cfg, Status: st, Peripherals: per})
	if len(r.Modules) != 1 {
		t.Fatalf("Plus add-on has no digital out module: %s", kinds(r))
	}
}

func TestXT1(t *testing.T) {
	c := []byte(`{"components":[
	 {"key":"boolean:202","status":{"value":true}},
	 {"key":"number:200","status":{"value":45}},
	 {"key":"number:201","config":{"meta":{"ui":{"unit":"°C"}}},"status":{"value":20.5}},
	 {"key":"number:202","config":{"min":5,"max":35,"meta":{"ui":{"unit":"°C"}}},"status":{"value":22}}]}`)
	r := Gen2(Gen2Input{TypeID: "XT1", Variant: "linkedgo-st1820-floor-thermostat", Config: []byte(`{}`), Status: []byte(`{}`), Components: c})
	m := r.Modules[0]
	if r.Layout != LayoutThermostat || m.Key != "xt1:202:202:C" || !*m.Enabled || *m.Target != 22 || *m.Min != 5 || *m.Max != 35 {
		t.Fatalf("ST1820 %s %+v", r.Layout, m)
	}
	if got := r.Meters[0].Values; got[0].Value != 20.5 || got[1].Value != 45 {
		t.Fatalf("ST1820 meters %+v", got)
	}
	f := []byte(`{"components":[
	 {"key":"enum:201","status":{"value":"heat"}},
	 {"key":"boolean:201","status":{"value":false}},
	 {"key":"number:201","config":{"meta":{"ui":{"unit":"°F"}}},"status":{"value":68}},
	 {"key":"number:203","config":{"min":41,"max":95,"meta":{"ui":{"unit":"°F"}}},"status":{"value":71.6}}]}`)
	r = Gen2(Gen2Input{TypeID: "XT1", Variant: "linkedgo-st-802-hvac", Config: []byte(`{}`), Status: []byte(`{}`), Components: f})
	m = r.Modules[0]
	if m.Key != "xt1:201:203:F" || *m.Target != 22 || *m.Min != 5 || *m.Max != 35 || r.Meters[0].Values[0].Value != 20 {
		t.Fatalf("ST802 °F %+v %+v", m, r.Meters)
	}
	dry := []byte(`{"components":[{"key":"enum:201","status":{"value":"dry"}},{"key":"boolean:201","status":{"value":true}},{"key":"number:202","status":{"value":55}}]}`)
	r = Gen2(Gen2Input{TypeID: "XT1", Variant: "linkedgo-st-802-hvac", Config: []byte(`{}`), Status: []byte(`{}`), Components: dry})
	if r.Layout != LayoutRelay || r.Modules[0].Key != "boolean:201" || r.Modules[0].Label != "Humidity: 55%" || !*r.Modules[0].On {
		t.Fatalf("ST802 dry %s %+v", r.Layout, r.Modules)
	}
	if XT1Keys("linkedgo-st-802-hvac")[5] != "enum:201" || XT1Keys("") != nil {
		t.Fatal("XT1Keys")
	}
}

func TestInputEvents(t *testing.T) {
	actions := []byte(`{"actions":{"shortpush_url":[{"index":0,"enabled":true,"urls":["http://a/1"]},{"index":1,"enabled":true,"urls":[]}],
	  "longpush_url":[{"index":0,"enabled":false,"urls":["http://a/2"]}],
	  "double_shortpush_url":[{"index":0,"enabled":true,"urls":["http://a/3","http://a/4"]}]}}`)
	r := Gen1(Gen1Input{TypeID: "SHIX3-1", Settings: []byte(`{"inputs":[{"name":"Door"},{},{}]}`), Status: []byte(`{"inputs":[{"input":1},{"input":0},{"input":0}]}`), Actions: actions})
	var got []string
	for _, e := range r.Modules[0].Events {
		got = append(got, e.Event+"="+map[bool]string{true: "on", false: "off"}[e.Enabled])
	}
	if strings.Join(got, " ") != "shortpush_url=on longpush_url=off double_shortpush_url=on" || !reflect.DeepEqual(r.Modules[0].Events[2].URLs, []string{"http://a/3", "http://a/4"}) {
		t.Fatalf("i3 events keep the JSON order: %v", got)
	}
	if r.Modules[0].Label != "Door" || !*r.Modules[0].InputOn || len(r.Modules[1].Events) != 1 || r.Modules[1].Events[0].Enabled {
		t.Fatalf("i3 inputs %+v", r.Modules)
	}

	hooks := []byte(`{"hooks":[{"id":1,"cid":0,"event":"input.button_push","enable":true,"urls":["http://127.0.0.1/rpc/Switch.Toggle?id=0"]},
	  {"id":2,"cid":1,"event":"input.button_longpush","enable":false,"urls":["http://x"]},
	  {"id":3,"cid":0,"event":"input.button_doublepush","enable":true,"urls":["http://y"]},
	  {"id":4,"cid":200,"event":"bthomesensor.single_push","enable":true,"urls":["http://z"]}]}`)
	g := Gen2(Gen2Input{TypeID: "PlusI4", Config: []byte(`{"input:0":{"name":"Hall","enable":true},"input:1":{"enable":false}}`), Status: []byte(`{"input:0":{"state":true}}`), Webhooks: hooks})
	if e := g.Modules[0].Events; len(e) != 2 || e[0].Event != "input.button_push" || e[1].Event != "input.button_doublepush" || e[0].URLs[0] != "http://127.0.0.1/rpc/Switch.Toggle?id=0" {
		t.Fatalf("i4 input 0 events %+v", e)
	}
	if *g.Modules[1].Enabled || len(g.Modules[1].Events) != 1 {
		t.Fatalf("i4 input 1 %+v", g.Modules[1])
	}
}

func TestBTHomeInputs(t *testing.T) {
	objs := []byte(`{"objects":[{"obj_id":58,"component":"bthomesensor:201"}]}`)
	comps := []byte(`{"components":[{"key":"bthomedevice:200","status":{"rssi":-60}},{"key":"bthomesensor:201","config":{"name":"Btn"},"status":{"value":""}}]}`)
	hooks := []byte(`{"hooks":[{"cid":201,"event":"bthomesensor.single_push","enable":true,"urls":["http://a"]},
	  {"cid":200,"event":"bthomedevice.single_push","enable":true,"condition":"ev.idx == 1","urls":["http://b"]},
	  {"cid":200,"event":"bthomedevice.double_push","enable":true,"condition":"ev.sensors[96][0].value === 2 && ev.idx === 0","urls":["http://c"]},
	  {"cid":200,"event":"bthomedevice.long_push","enable":false,"condition":"ev.idx == 1","urls":["http://d"]},
	  {"cid":300,"event":"bthomedevice.single_push","enable":true,"urls":["http://e"]}]}`)
	r := BTHome(BTHomeInput{Index: "200", KnownObjects: objs, Components: comps, Webhooks: hooks})
	if r.Layout != LayoutMixed || len(r.Modules) != 3 {
		t.Fatalf("modules %s %+v", r.Layout, r.Modules)
	}
	if m := r.Modules[0]; m.Label != "Btn" || len(m.Events) != 1 || m.Events[0].Event != "bthomesensor.single_push" {
		t.Fatalf("button sensor %+v", m)
	}
	// Sorted conditions: "ev.idx == 1" < "ev.sensors…".
	if m := r.Modules[1]; m.Label != "1" || len(m.Events) != 2 || m.Events[1].Enabled {
		t.Fatalf("device input 1 %+v", m)
	}
	if m := r.Modules[2]; m.Label != "ch 2 - 0" || len(m.Events) != 1 {
		t.Fatalf("device input 2 %+v", m)
	}
}

func TestUpdateAvailable(t *testing.T) { // DECISIONS P19-1: what the device itself says
	cfg := []byte(`{}`)
	g1 := func(status string) Readings {
		return Gen1(Gen1Input{TypeID: "SHPLG-S", Settings: cfg, Status: []byte(status)})
	}
	g2 := func(status string) Readings {
		return Gen2(Gen2Input{TypeID: "Plus1", Config: cfg, Status: []byte(status)})
	}
	if r := g1(`{"has_update":false,"update":{"has_update":false,"new_version":"20230913/v1.14.0"}}`); r.UpdateAvailable || r.UpdateVersion != "" {
		t.Fatalf("Gen1 up to date: %+v", r)
	}
	if r := g1(`{"has_update":true,"update":{"has_update":true,"new_version":"20240101/v1.15.0"}}`); !r.UpdateAvailable || r.UpdateVersion != "20240101/v1.15.0" {
		t.Fatalf("Gen1 update: %+v", r)
	}
	if r := g1(`{"update":{"has_update":true}}`); !r.UpdateAvailable || r.UpdateVersion != "" {
		t.Fatalf("Gen1 update without version: %+v", r)
	}
	if r := g2(`{"sys":{"available_updates":{}}}`); r.UpdateAvailable {
		t.Fatalf("Gen2 nothing available: %+v", r)
	}
	if r := g2(`{"sys":{"available_updates":{"beta":{"version":"1.5.0-beta1"}}}}`); r.UpdateAvailable {
		t.Fatalf("a beta alone is no update arrow: %+v", r)
	}
	if r := g2(`{"sys":{"available_updates":{"stable":{"version":"1.4.4"},"beta":{"version":"1.5.0-beta1"}}}}`); !r.UpdateAvailable || r.UpdateVersion != "1.4.4" {
		t.Fatalf("Gen2 stable update: %+v", r)
	}
	if r := g2(`{}`); r.UpdateAvailable {
		t.Fatalf("Gen2 without sys: %+v", r)
	}
}
