package parse

import "testing"

// Measured on Wim's LAN (docs/blu-cloud-relay.md): a BLU RC Button 4 relayed by two gateways.
func TestDecodeRelayedRCButton4(t *testing.T) {
	a, err := DecodeBTHomeBase64("RAC8AWQ6ADoAOgA6AQ==")
	if err != nil {
		t.Fatal(err)
	}
	if a.Encrypted || !a.Trigger || a.PacketID != 188 || len(a.Objects) != 5 {
		t.Fatalf("advert %+v", a)
	}
	if b, _ := a.Object(0x01); b != 100 {
		t.Fatalf("battery %v", b)
	}
	if a.ModelID() != 0 || a.Estimate() != "Blu Wall Switch 4 / RC Button 4" {
		t.Fatalf("model %d, estimate %q", a.ModelID(), a.Estimate())
	}
	r := BTHomeRelayed(a)
	if len(r.Meters) != 1 || r.Meters[0].Values[0] != (MeterValue{Type: BAT, Value: 100}) {
		t.Fatalf("meters %+v", r.Meters)
	}
	if len(r.Modules) != 4 || r.Modules[3].State != "press" || r.Modules[0].State != "" || r.Modules[3].Label != "4" {
		t.Fatalf("buttons %+v", r.Modules)
	}
	// "Hold" on buttons 3 and 4 (0x80), from the same device.
	a, _ = DecodeBTHomeBase64("RADrAWQ6ADoAOoA6gA==")
	if r := BTHomeRelayed(a); r.Modules[2].State != "hold" || r.Modules[3].State != "hold" {
		t.Fatalf("hold %+v", r.Modules)
	}
}

func TestDecodeBTHomeObjects(t *testing.T) {
	// H&T: packet 7, battery 95, temperature 21.5 °C (0x45, 0.1), humidity 48 % (0x2E).
	a, err := DecodeBTHome([]byte{0x44, 0x00, 0x07, 0x01, 95, 0x45, 0xD7, 0x00, 0x2E, 48})
	if err != nil || a.Estimate() != "Blu H&T" {
		t.Fatalf("%+v %v %q", a, err, a.Estimate())
	}
	r := BTHomeRelayed(a)
	if len(r.Meters[0].Values) != 3 || r.Meters[0].Values[1] != (MeterValue{Type: T, Value: 21.5}) || r.Meters[0].Values[2] != (MeterValue{Type: H, Value: 48}) {
		t.Fatalf("meters %+v", r.Meters)
	}
	// Negative temperature: -3.2 °C.
	a, _ = DecodeBTHome([]byte{0x40, 0x45, 0xE0, 0xFF})
	if v, _ := a.Object(0x45); v != -3.2 {
		t.Fatalf("negative %v", v)
	}
	// Door/Window: illuminance 120.5 lx (3 bytes, 0.01), window open, rotation 12.3°.
	a, _ = DecodeBTHome([]byte{0x44, 0x05, 0x12, 0x2F, 0x00, 0x2D, 1, 0x3F, 123, 0})
	if a.Estimate() != "Blu Door Window" {
		t.Fatalf("estimate %q %+v", a.Estimate(), a)
	}
	if r := BTHomeRelayed(a); len(r.Modules) != 1 || r.Modules[0].Label != "Open" || !*r.Modules[0].On {
		t.Fatalf("window %+v", r.Modules)
	}
	// The 6-hourly information packet: device type id 7 (RC Button 4), firmware.
	a, _ = DecodeBTHome([]byte{0x40, 0x00, 0x01, 0xF0, 0x07, 0x00, 0xF1, 0x00, 0x16, 0x00, 0x01})
	if a.ModelID() != 7 {
		t.Fatalf("model id %+v", a)
	}
	// Unknown object: the rest is dropped; encrypted: no objects; not v2: error.
	if a, _ = DecodeBTHome([]byte{0x40, 0x01, 50, 0x99, 1, 2, 0x45, 0, 0}); len(a.Objects) != 1 {
		t.Fatalf("unknown object %+v", a)
	}
	if a, _ = DecodeBTHome([]byte{0x41, 0x01, 50}); !a.Encrypted || len(a.Objects) != 0 {
		t.Fatalf("encrypted %+v", a)
	}
	if _, err := DecodeBTHome([]byte{0x20}); err == nil {
		t.Fatal("version 1 accepted")
	}
	// 0x12 (CO2) is two bytes, not a binary sensor.
	if a, _ = DecodeBTHome([]byte{0x40, 0x12, 0xE2, 0x04}); len(a.Objects) != 1 || a.Objects[0].Value != 1250 {
		t.Fatalf("co2 %+v", a)
	}
}
