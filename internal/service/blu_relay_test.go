package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
)

const relayMAC, relayID = "7c:c6:b6:a5:c9:3d", "7CC6B6A5C93D"

// gateway starts a Gen3 gateway (Dimmer G3 or i4 G3 fixture) with its own MAC that
// relays the RC Button 4 measured on Wim's LAN, heard at lastSeen (unix seconds).
func gateway(t *testing.T, m *Devices, ctx context.Context, fixture, app, modelID, mac string, lastSeen int64) string {
	t.Helper()
	listInfos := fmt.Sprintf(`{"ts":%d,"offset":0,"count":1,"total":1,"devices":[{%q:{"name":null,"model":0,`+
		`"sdata":{"fcd2":"RAC8AWQ6ADoAOgA6AQ=="},"mdata":{},"last_seen":%d}}]}`, lastSeen+10, relayMAC, lastSeen)
	host := fmt.Sprintf("shelly%s-%s", app, mac)
	_, addr := startSimDev(t, fixtureDir(t, fixture, map[string]string{
		"shelly.json":                       fmt.Sprintf(`{"id":%q,"mac":%q,"model":"S3XX","gen":3,"app":%q,"auth_en":false}`, host, mac, app),
		"rpc_BLE.CloudRelay.ListInfos.json": listInfos,
		// No BTHome components (the recorded answer is paged; the simulator ignores offset).
		"rpc_Shelly.GetComponents.json": `{"components":[],"offset":0,"total":0}`,
	}), nil)
	m.handle(ctx, addr, host, true)
	waitDevice(t, m, mac, online)
	return host
}

func TestRelayedBLURows(t *testing.T) {
	m, _, ctx := newService(t, nil)
	host1 := gateway(t, m, ctx, "gen3/DimmerG3", "dimmerg3", "S3DM-0A101WWL", "AABBCC0000D1", 1791148000)

	// A row of its own: online once heard, battery and the four buttons, an estimate.
	d := waitDevice(t, m, relayID, func(d model.Device) bool { return d.Relay && d.Status == model.StatusOnline })
	if d.Gen != model.GenBTHome || d.Parent != "AABBCC0000D1" || d.LastSeen != 1791148000000 {
		t.Fatalf("relayed row %+v", d)
	}
	if d.TypeID != "BLU" || d.TypeName != "Blu Wall Switch 4 / RC Button 4 ?" {
		t.Fatalf("estimate %q %q", d.TypeID, d.TypeName)
	}
	if len(d.Meters) != 1 || d.Meters[0].Values[0].Value != 100 || len(d.Modules) != 4 || d.Modules[3].State != "press" {
		t.Fatalf("readings %+v %+v", d.Meters, d.Modules)
	}

	// A second gateway heard it later: the row follows it, the first is an alternative.
	gateway(t, m, ctx, "gen3/I4G3", "i4g3", "S3SN-0024X", "AABBCC0000D2", 1791148100)
	d = waitDevice(t, m, relayID, func(d model.Device) bool { return d.Parent == "AABBCC0000D2" })
	if !contains(d.Parents, host1) || d.LastSeen != 1791148100000 {
		t.Fatalf("second gateway %+v", d)
	}

	// Identified (the wizard): the model stays, a later refresh keeps it.
	if !m.setBLUModel(relayMAC, 7) {
		t.Fatal("setBLUModel")
	}
	m.mu.Lock()
	gw := m.devs["AABBCC0000D2"]
	m.mu.Unlock()
	m.refreshRelay(ctx, gw)
	if d, _ = m.Get(relayID); d.TypeID != "BLU7" || d.TypeName != "Blu RC Button 4" {
		t.Fatalf("identified model %q %q", d.TypeID, d.TypeName)
	}

	// A name of its own (archive); not for other devices.
	if err := m.SetBLUName(relayID, " Keuken knoppen "); err != nil {
		t.Fatal(err)
	}
	if d, _ = m.Get(relayID); d.Name != "Keuken knoppen" {
		t.Fatalf("name %q", d.Name)
	}
	if err := m.SetBLUName("AABBCC0000D1", "x"); err == nil {
		t.Fatal("named a gateway")
	}

	// Read only: no backup, no restore; the archive keeps name and model.
	lines, _ := m.Backup(ctx, []string{relayID})
	if len(lines) != 1 || lines[0].Result != ResultFail || lines[0].Message != ErrRelayed.Error() {
		t.Fatalf("backup: %+v", lines)
	}
	m.mu.Lock()
	re := m.devs[relayID]
	m.mu.Unlock()
	if _, err := m.restoreData(ctx, re, nil, nil, false); err == nil {
		t.Fatal("restore of a relayed device")
	}
	m.saveArchive()
	m.mu.Lock()
	a := m.archive[relayID]
	m.mu.Unlock()
	if a.Name != "Keuken knoppen" || a.TypeID != "BLU7" {
		t.Fatalf("archive %+v", a)
	}
}

func TestIdentifyBLU(t *testing.T) {
	ev := &recorder{}
	m, _, ctx := newService(t, nil)
	m.emit = func(typ string, data any) {
		ev.mu.Lock()
		ev.events = append(ev.events, fmt.Sprintf("%s %+v", typ, data))
		ev.mu.Unlock()
	}
	host := "shellydimmerg3-aabbcc0000d1"
	_, addr := startSimDev(t, fixtureDir(t, "gen3/DimmerG3", map[string]string{
		"shelly.json": fmt.Sprintf(`{"id":%q,"mac":"AABBCC0000D1","model":"S3DM-0A101WWL","gen":3,"app":"DimmerG3","auth_en":false}`, host),
		"rpc_BLE.CloudRelay.ListInfos.json": `{"ts":1791148010,"offset":0,"count":1,"total":1,"devices":[{"7c:c6:b6:a5:c9:3d":{"name":null,"model":0,` +
			`"sdata":{"fcd2":"RAC8AWQ6ADoAOgA6AQ=="},"mdata":{},"last_seen":1791148000}}]}`,
		"rpc_Shelly.GetComponents.json": `{"components":[],"offset":0,"total":0}`,
		// What the RC Button 4 answered in pairing mode on 2026-10-04, and one BLU not relayed.
		"_discovery.json": `[{"addr":"7c:c6:b6:a5:c9:3d","local_name":"SBBT-004CUS","rssi":-61,"encrypted":false,"shelly_mfdata":{"flags":17,"model_id":7,"mac":"7c:c6:b6:a5:c9:3d"}},` +
			`{"addr":"aa:bb:cc:00:00:99","local_name":"SBHT-003C","rssi":-70,"encrypted":false,"shelly_mfdata":{"flags":17,"model_id":3,"mac":"aa:bb:cc:00:00:99"}}]`,
	}), nil)
	m.handle(ctx, addr, host, true)
	waitDevice(t, m, "AABBCC0000D1", online)
	waitDevice(t, m, relayID, func(d model.Device) bool { return d.Relay })

	if gws := m.BLUGateways(); len(gws) != 1 || gws[0].ID != "AABBCC0000D1" {
		t.Fatalf("gateways %+v", gws)
	}
	if err := m.IdentifyBLU(relayID, 30); err != ErrNotBLUGateway {
		t.Fatalf("a BLU row as gateway: %v", err)
	}
	if err := m.IdentifyBLU("AABBCC0000D1", 30); err != nil {
		t.Fatal(err)
	}
	if err := m.IdentifyBLU("AABBCC0000D1", 30); err != ErrIdentifyBusy {
		t.Fatalf("second run: %v", err)
	}
	d := waitDevice(t, m, relayID, func(d model.Device) bool { return d.TypeID == "BLU7" })
	if d.TypeName != "Blu RC Button 4" {
		t.Fatalf("model %q", d.TypeName)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ev.mu.Lock()
		all := strings.Join(ev.events, "\n")
		ev.mu.Unlock()
		if strings.Contains(all, "state:done") {
			if !strings.Contains(all, "blu.discovered {ID:7CC6B6A5C93D") || !strings.Contains(all, "Listed:true") ||
				!strings.Contains(all, "MAC:aa:bb:cc:00:00:99") || !strings.Contains(all, "found:2") {
				t.Fatalf("events:\n%s", all)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no end event:\n%s", all)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := m.IdentifyBLU("AABBCC0000D1", 30); err != nil { // free again
		t.Fatalf("after the run: %v", err)
	}
}
