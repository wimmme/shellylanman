package model

import (
	"testing"

	"github.com/wimmme/shellylanman/internal/shelly"
)

func TestLookup(t *testing.T) {
	cases := []struct {
		info         shelly.Info
		id, name     string
		battery, pro bool
		known        bool
	}{
		{shelly.Info{Type: "SHPLG-S"}, "SHPLG-S", "PlugS", false, false, true},
		{shelly.Info{Type: "SHHT-1"}, "SHHT-1", "Shelly H&T", true, false, true},
		{shelly.Info{Type: "SHXX-9"}, "SHXX-9", "Generic G1", false, false, false},
		{shelly.Info{Gen: 2, App: "Plus1"}, "Plus1", "Shelly +1", false, false, true},
		{shelly.Info{Gen: 2, App: "Pro3EM", Model: "SPEM-003CEBEU63"}, "Pro3EM", "Shelly Pro 3EM", false, true, true},
		{shelly.Info{Gen: 2, App: "Pro4PM", Model: "SPSH-002PE16EU"}, "Pro4PM", "Shelly Pro Dual Cover", false, true, true},
		{shelly.Info{Gen: 2, App: "Pro4PM", Model: "SPSW-004PE16EU"}, "Pro4PM", "Shelly Pro 4PM", false, true, true},
		{shelly.Info{Gen: 2, App: "ProDimmerx", Model: "SPDM-002PE01EU"}, "ProDimmerx", "Shelly Pro Dimmer 2PM", false, true, true},
		{shelly.Info{Gen: 2, App: "PlusHT"}, "PlusHT", "Shelly +H&T", true, false, true},
		{shelly.Info{Gen: 2, App: "BluGw"}, "BluGw", "Shelly BLU Gateway", false, false, true},
		{shelly.Info{Gen: 3, App: "DimmerG3"}, "DimmerG3", "Shelly Dimmer G3", false, false, true},
		{shelly.Info{Gen: 3, App: "XT1", Svc0Type: "linkedgo-st-802-hvac"}, "XT1", "LinkedGo ST802", false, false, true},
		{shelly.Info{Gen: 3, App: "XT1", Svc0Type: "other"}, "XT1", "XT1", false, false, true},
		{shelly.Info{Gen: 4, App: "Mini1PMG4", Model: "S4SW-001P8EU"}, "S4SW-001P8EU", "Shelly Mini 1PM G4", false, false, true},
		{shelly.Info{Gen: 4, Model: "S4SN-0071A"}, "S4SN-0071A", "Shelly Flood G4", true, false, true},
		{shelly.Info{Gen: 4, Model: "S4XX"}, "S4XX", "Generic G4", false, false, false},
	}
	for _, c := range cases {
		m := Lookup(c.info)
		if m.TypeID != c.id || m.TypeName != c.name || m.Battery != c.battery || m.Pro != c.pro || m.Known != c.known {
			t.Errorf("Lookup(%+v) = %+v", c.info, m)
		}
	}
}

func TestRegistrySize(t *testing.T) {
	// ShellyScanner 1.3.4 knows 27 Gen1, 33 Gen2, 28 Gen3 and 15 Gen4 models
	// (FEATURE_PARITY.md §2). Addon aliases and model splits make the maps differ.
	n2 := 0
	names := map[string]bool{}
	for _, e := range gen2 {
		names[e.name] = true
	}
	for _, m := range gen2ByModel {
		for _, e := range m {
			names[e.name] = true
		}
	}
	n2 = len(names)
	n3 := len(gen3) + len(xt1BySvc0)
	if len(gen1) != 27 || n2 != 33 || n3 != 28 || len(gen4) != 15 {
		t.Fatalf("registry sizes G1=%d G2=%d G3=%d G4=%d, want 27/33/28/15", len(gen1), n2, n3, len(gen4))
	}
}

func TestBLUTypeName(t *testing.T) {
	if BLUTypeName(3) != "Blu H&T" || BLUTypeName(0x203A) != "Blu 3" || BLUTypeName(999) != "Generic BTHome" {
		t.Fatal("BLU type names wrong")
	}
}
