package parse

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, dir, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", dir, file))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	return b
}

// shape reduces readings to what ShellyScanner decides per model: the meter
// types of each set, the module kinds, and whether a temperature is shown.
type shape struct {
	meters  [][]string
	modules []string
	temp    bool
}

func shapeOf(r Readings) shape {
	s := shape{temp: r.InternalTemp != nil}
	for _, m := range r.Meters {
		var types []string
		for _, v := range m.Values {
			types = append(types, v.Type)
		}
		s.meters = append(s.meters, types)
	}
	for _, m := range r.Modules {
		s.modules = append(s.modules, m.Kind)
	}
	return s
}

func TestGen1Fixtures(t *testing.T) {
	cases := []struct {
		dir, typ string
		want     shape
	}{
		{"gen1/SHPLG-S", "SHPLG-S", shape{[][]string{{W}}, []string{KindRelay}, true}},
		{"gen1/SHSW-L", "SHSW-L", shape{[][]string{{W}}, []string{KindRelay}, true}},
		// Shelly 1 with no external sensors and no configured load: no measurements.
		{"gen1/SHSW-1", "SHSW-1", shape{nil, []string{KindRelay}, false}},
		{"gen1/SHIX3-1", "SHIX3-1", shape{nil, []string{KindInput, KindInput, KindInput}, false}},
		// RGBW2 in white mode: four lights, four power meters.
		{"gen1/SHRGBW2", "SHRGBW2", shape{[][]string{{W}, {W}, {W}, {W}}, []string{KindLight, KindLight, KindLight, KindLight}, false}},
		// UNI without external sensors: ADC voltage only.
		{"gen1/SHUNI-1", "SHUNI-1", shape{[][]string{{V}}, []string{KindRelay, KindRelay}, false}},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			r := Gen1(Gen1Input{TypeID: c.typ, Settings: fixture(t, c.dir, "settings.json"), Status: fixture(t, c.dir, "status.json")})
			if got := shapeOf(r); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("shape = %+v, want %+v", got, c.want)
			}
			st := decode(fixture(t, c.dir, "status.json"))
			if r.RSSI != st.Path("wifi_sta", "rssi").Int() || r.Uptime != st.Get("uptime").Int() || r.SSID != "REDACTED" {
				t.Fatalf("common fields: rssi %d uptime %d ssid %q", r.RSSI, r.Uptime, r.SSID)
			}
			if r.LogMode == "" {
				t.Fatal("log mode not set")
			}
		})
	}
}

func TestGen1Values(t *testing.T) {
	st := decode(fixture(t, "gen1/SHPLG-S", "status.json"))
	r := Gen1(Gen1Input{TypeID: "SHPLG-S", Settings: fixture(t, "gen1/SHPLG-S", "settings.json"), Status: fixture(t, "gen1/SHPLG-S", "status.json")})
	if *r.InternalTemp != st.Get("temperature").Float() || r.Meters[0].Values[0].Value != st.Path("meters", "0", "power").Float() {
		t.Fatalf("PlugS values: temp %v meters %+v", *r.InternalTemp, r.Meters)
	}
	if *r.Modules[0].On != st.Path("relays", "0", "ison").Bool() || r.Modules[0].Source == "" {
		t.Fatalf("PlugS relay: %+v", r.Modules[0])
	}
}

func TestGen2Fixtures(t *testing.T) {
	cases := []struct {
		dir, typ string
		want     shape
	}{
		{"gen2/Plus1", "Plus1", shape{nil, []string{KindRelay}, true}},
		{"gen4/S4SW-001P8EU", "S4SW-001P8EU", shape{[][]string{{W, V, I}}, []string{KindRelay}, true}},
		{"gen3/MiniPMG3", "MiniPMG3", shape{[][]string{{W, V, I, FREQ}}, nil, false}},
		{"gen3/DimmerG3", "DimmerG3", shape{[][]string{{W, V, I}}, []string{KindLight}, true}},
		{"gen3/Dimmer0110VPMG3", "Dimmer0110VPMG3", shape{[][]string{{W, V, I}}, []string{KindLight}, true}},
		{"gen3/I4G3", "I4G3", shape{nil, []string{KindInput, KindInput, KindInput, KindInput}, false}},
		// Plus UNI: counter set; its integrated add-on has no peripherals configured.
		{"gen2/PlusUni", "PlusUni", shape{[][]string{{NUM, FREQ}}, []string{KindRelay, KindRelay}, false}},
		// Profile "light": four lights with W/V/I each.
		{"gen2/PlusRGBWPM", "PlusRGBWPM", shape{[][]string{{W, V, I}, {W, V, I}, {W, V, I}, {W, V, I}},
			[]string{KindLight, KindLight, KindLight, KindLight}, true}},
		{"gen2/ProRGBWWPM", "ProRGBWWPM", shape{[][]string{{W, V, I}, {W, V, I}, {W, V, I}, {W, V, I}, {W, V, I}},
			[]string{KindLight, KindLight, KindLight, KindLight, KindLight}, true}},
		// Profile "triphase": phases a, b, c and the total.
		{"gen2/Pro3EM", "Pro3EM", shape{[][]string{{W, VA, PF, V, I, FREQ}, {W, VA, PF, V, I, FREQ}, {W, VA, PF, V, I, FREQ}, {W, VA, I}}, nil, true}},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			r := Gen2(Gen2Input{TypeID: c.typ, Config: fixture(t, c.dir, "rpc_Shelly.GetConfig.json"),
				Status: fixture(t, c.dir, "rpc_Shelly.GetStatus.json"), Peripherals: fixture(t, c.dir, "rpc_SensorAddon.GetPeripherals.json")})
			if got := shapeOf(r); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("shape = %+v, want %+v", got, c.want)
			}
			st := decode(fixture(t, c.dir, "rpc_Shelly.GetStatus.json"))
			if r.RSSI != st.Path("wifi", "rssi").Int() || r.Uptime != st.Path("sys", "uptime").Int() || r.LogMode == "" {
				t.Fatalf("common: rssi %d uptime %d log %q", r.RSSI, r.Uptime, r.LogMode)
			}
		})
	}
}

func TestGen2Values(t *testing.T) {
	dir := "gen3/MiniPMG3"
	st := decode(fixture(t, dir, "rpc_Shelly.GetStatus.json"))
	r := Gen2(Gen2Input{TypeID: "MiniPMG3", Config: fixture(t, dir, "rpc_Shelly.GetConfig.json"), Status: fixture(t, dir, "rpc_Shelly.GetStatus.json")})
	pm := st.Get("pm1:0")
	want := []MeterValue{{Type: W, Value: pm.Get("apower").Float()}, {Type: V, Value: pm.Get("voltage").Float()},
		{Type: I, Value: pm.Get("current").Float()}, {Type: FREQ, Value: pm.Get("freq").Float()}}
	if !reflect.DeepEqual(r.Meters[0].Values, want) {
		t.Fatalf("MiniPMG3 = %+v, want %+v", r.Meters[0].Values, want)
	}
	dir = "gen3/DimmerG3"
	st = decode(fixture(t, dir, "rpc_Shelly.GetStatus.json"))
	r = Gen2(Gen2Input{TypeID: "DimmerG3", DeviceName: "Kitchen", Config: fixture(t, dir, "rpc_Shelly.GetConfig.json"), Status: fixture(t, dir, "rpc_Shelly.GetStatus.json")})
	l := r.Modules[0]
	if *l.On != st.Path("light:0", "output").Bool() || *l.Brightness != st.Path("light:0", "brightness").Int() || *r.InternalTemp != st.Path("light:0", "temperature", "tC").Float() {
		t.Fatalf("DimmerG3 light %+v temp %v", l, *r.InternalTemp)
	}
}

func TestGen2Profiles(t *testing.T) {
	cfg := []byte(`{"sys":{"device":{"name":"Blinds","profile":"cover"}},"cover:0":{"name":""}}`)
	st := []byte(`{"cover:0":{"state":"stopped","pos_control":true,"current_pos":42,"apower":0,"voltage":230,"current":0,"pf":0,"source":"WS_in","temperature":{"tC":40.5}},"input:0":{"state":false}}`)
	r := Gen2(Gen2Input{TypeID: "Plus2PM", Config: cfg, Status: st})
	if got := shapeOf(r); !reflect.DeepEqual(got, shape{[][]string{{W, PF, V, I}}, []string{KindCover}, true}) {
		t.Fatalf("2PM cover shape %+v", got)
	}
	if m := r.Modules[0]; *m.Position != 42 || m.Label != "Blinds" || m.Source != "WS_in" {
		t.Fatalf("cover module %+v", m)
	}
	mono := []byte(`{"sys":{"device":{"profile":"monophase"}},"em1:0":{"name":"Oven"}}`)
	r = Gen2(Gen2Input{TypeID: "Pro3EM", Config: mono, Status: []byte(`{"em1:0":{"act_power":10},"em1:1":{},"em1:2":{}}`)})
	if len(r.Meters) != 3 || r.Meters[0].Label != "Oven" || r.Meters[0].Values[0].Value != 10 {
		t.Fatalf("3EM monophase %+v", r.Meters)
	}
}

func TestAddon(t *testing.T) {
	cfg := []byte(`{"sys":{"device":{"addon_type":"sensor"}},"temperature:100":{"name":"Water"},"temperature:101":{"name":"Air"},"input:100":{"name":"Door"},"voltmeter:100":{"name":"Tank"}}`)
	st := []byte(`{"switch:0":{"output":true},"input:0":{},"temperature:100":{"tC":12.5},"temperature:101":{"tC":20},"input:100":{"state":true},"voltmeter:100":{"voltage":3.3,"xvoltage":55}}`)
	per := []byte(`{"ds18b20":{"temperature:101":{},"temperature:100":{}},"digital_in":{"input:100":{}},"voltmeter":{"voltmeter:100":{}}}`)
	r := Gen2(Gen2Input{TypeID: "Plus1", Config: cfg, Status: st, Peripherals: per})
	if len(r.Meters) != 1 {
		t.Fatalf("addon meters %+v", r.Meters)
	}
	var got []string
	for _, v := range r.Meters[0].Values {
		got = append(got, v.Type+":"+v.Name)
	}
	want := "T:Water T1:Air EX:Door VL:Tank VX:Tank"
	if strings.Join(got, " ") != want {
		t.Fatalf("addon = %v, want %s", got, want)
	}
	// Not fitted (addon_type null): no add-on meters even with peripherals.
	r = Gen2(Gen2Input{TypeID: "Plus1", Config: []byte(`{"sys":{"device":{}}}`), Status: st, Peripherals: per})
	if len(r.Meters) != 0 {
		t.Fatalf("addon read without addon_type: %+v", r.Meters)
	}
}

func TestBTHome(t *testing.T) {
	objs := []byte(`{"objects":[{"obj_id":1,"component":"bthomesensor:200"},{"obj_id":69,"component":"bthomesensor:201"},{"obj_id":69,"component":"bthomesensor:203"},{"obj_id":46,"component":"bthomesensor:202"},{"obj_id":45,"component":"bthomesensor:204"},{"obj_id":58,"component":null}]}`)
	comps := []byte(`{"components":[
	  {"key":"bthomedevice:200","status":{"rssi":-71}},
	  {"key":"bthomesensor:200","config":{"name":"Battery"},"status":{"value":88}},
	  {"key":"bthomesensor:201","config":{"name":"In"},"status":{"value":21.5}},
	  {"key":"bthomesensor:202","config":{"name":""},"status":{"value":55}},
	  {"key":"bthomesensor:203","config":{"name":"Out"},"status":{"value":9}},
	  {"key":"bthomesensor:204","config":{"name":"Window"},"status":{"value":true}}]}`)
	r := BTHome(BTHomeInput{Index: "200", KnownObjects: objs, Components: comps})
	var got []string
	for _, v := range r.Meters[0].Values {
		got = append(got, v.Type)
	}
	if strings.Join(got, ",") != "BAT,T,H,T1" || r.RSSI != -71 || len(r.Modules) != 1 || !*r.Modules[0].On {
		t.Fatalf("BTHome = %v rssi %d modules %+v", got, r.RSSI, r.Modules)
	}
}

func TestBluTRV(t *testing.T) {
	r := BluTRV(BluTRVInput{Name: "Radiator", Status: []byte(`{"rssi":-60,"battery":80}`),
		RemoteStatus: []byte(`{"status":{"sys":{"uptime":100},"trv:0":{"current_C":19.5,"target_C":21,"pos":30}}}`),
		RemoteConfig: []byte(`{"config":{"trv:0":{"enable":true}}}`)})
	if r.Meters[0].Values[0].Value != 19.5 || r.Meters[0].Values[1].Value != 80 || *r.Modules[0].Target != 21 || !*r.Modules[0].On || r.Uptime != 100 {
		t.Fatalf("TRV %+v", r)
	}
}

func TestEveryRegistryTypeHasAParser(t *testing.T) {
	// Types in gen2Models are keyed by registry TypeID; spot-check the ones
	// sharing an app (Pro4PM vs Dual Cover, ProDimmerx 1PM vs 2PM).
	dual := Gen2(Gen2Input{TypeID: "Pro4PM", Config: []byte(`{"cover:0":{},"cover:1":{}}`), Status: []byte(`{"cover:0":{},"cover:1":{}}`)})
	if len(dual.Modules) != 2 || dual.Modules[0].Kind != KindCover {
		t.Fatalf("Pro Dual Cover modules %+v", dual.Modules)
	}
	two := Gen2(Gen2Input{TypeID: "ProDimmerx", Config: []byte(`{"light:0":{},"light:1":{}}`), Status: []byte(`{"light:0":{},"light:1":{}}`)})
	if len(two.Modules) != 2 {
		t.Fatalf("Pro Dimmer 2PM modules %+v", two.Modules)
	}
}
