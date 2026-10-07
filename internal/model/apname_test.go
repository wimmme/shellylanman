package model

import "testing"

func TestParseAPName(t *testing.T) {
	for _, c := range []struct {
		ssid                 string
		ok                   bool
		gen, key, mac, model string
	}{
		// Gen2+, as the device names its access point (seen on real devices and in Shelly's docs)
		{"ShellyPlugSG3-54320467CBD4", true, "2", "PlugSG3", "54320467CBD4", "Plug S G3"},
		{"ShellyPlus2PM-A8032AB636EC", true, "2", "Plus2PM", "A8032AB636EC", "Shelly +2PM"},
		{"ShellyPro4PM-F008D1D89064", true, "2", "Pro4PM", "F008D1D89064", ""},
		{"ShellyMini1PMG4-A085E3B64248", true, "2", "Mini1PMG4", "A085E3B64248", ""}, // Gen4: not in the registry, capitals kept
		// the same as a host name (all lower case): the registry restores the spelling
		{"shellyplugsg3-54320467cbd4", true, "2", "PlugSG3", "54320467CBD4", "Plug S G3"},
		{"  ShellyDimmerG3-E4B063D99D7C ", true, "2", "DimmerG3", "E4B063D99D7C", "Shelly Dimmer G3"},
		// Gen1: the host name, a MAC of 12 or 6 digits
		{"shellyplug-s-80646F838136", true, "1", "SHPLG-S", "80646F838136", "PlugS"},
		{"shelly1-BA6201", true, "1", "SHSW-1", "", "Shelly 1"},
		{"shelly1l-E8DB84A1F0B7", true, "1", "SHSW-L", "E8DB84A1F0B7", "Shelly 1L"},
		{"shellyrgbw2-A894A1", true, "1", "SHRGBW2", "", "Shelly RGBW2"},
		{"shellyix3-98CDAC24F7D1", true, "1", "SHIX3-1", "98CDAC24F7D1", "Shelly I3"},
		// not readable
		{"", false, "", "", "", ""},
		{"HomeWifi", false, "", "", "", ""},
		{"ShellyPlus2PM", false, "", "", "", ""},
		{"ShellyPlus2PM-XYZ", false, "", "", "", ""},
		{"ShellyPlus2PM-A8032A", false, "", "", "", ""},         // Gen2+ names carry the whole MAC
		{"shellymini1pmg4-a085e3b64248", false, "", "", "", ""}, // unknown to the registry and no capitals: the index is case-sensitive
		{"shellyunknown-80646F838136", false, "", "", "", ""},
	} {
		got, ok := ParseAPName(c.ssid)
		if ok != c.ok {
			t.Errorf("%q: ok %v, want %v (%+v)", c.ssid, ok, c.ok, got)
			continue
		}
		if ok && (got.Gen != c.gen || got.Key != c.key || got.MAC != c.mac || got.Name != c.model) {
			t.Errorf("%q: %+v, want gen %s key %s mac %s name %q", c.ssid, got, c.gen, c.key, c.mac, c.model)
		}
	}
}

func TestDefaultAPName(t *testing.T) {
	if got := DefaultAPName("2", "shellyplugsg3-54320467cbd4", "PlugSG3", "54320467cbd4"); got != "ShellyPlugSG3-54320467CBD4" {
		t.Fatal(got)
	}
	if got := DefaultAPName("1", "shellyplug-s-80646F838136", "", "80646F838136"); got != "shellyplug-s-80646F838136" {
		t.Fatal(got)
	}
	if DefaultAPName("2", "x", "", "AA") != "" {
		t.Fatal("no app, no name")
	}
}

func TestModelChoices(t *testing.T) {
	list := ModelChoices()
	if len(list) < 60 {
		t.Fatalf("only %d models", len(list))
	}
	seen := map[string]bool{}
	var plugS, plusPlugS bool
	for _, m := range list {
		k := m.Gen + "/" + m.Key
		if seen[k] {
			t.Errorf("%s twice", k)
		}
		seen[k] = true
		if m.Name == "" || (m.Gen != "1" && m.Gen != "2") {
			t.Errorf("bad choice %+v", m)
		}
		plugS = plugS || m.Key == "SHPLG-S" && m.Gen == "1"
		plusPlugS = plusPlugS || m.Key == "PlusPlugS" && m.Gen == "2"
		if len(m.Key) > 8 && m.Key[len(m.Key)-8:] == "ProAddon" {
			t.Errorf("%s: the add-on variants are left out", m.Key)
		}
	}
	if !plugS || !plusPlugS {
		t.Fatal("known models missing")
	}
}
