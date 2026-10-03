package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/discovery"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/sim"
	"github.com/wimmme/shellylanman/internal/store"
)

// fixtureDir copies testdata/<rel> to a temp dir and applies overrides
// (file name → content).
func fixtureDir(t *testing.T, rel string, overrides map[string]string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", rel)
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(src, e.Name()))
		os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644)
	}
	for name, content := range overrides {
		os.WriteFile(filepath.Join(dst, name), []byte(content), 0o644)
	}
	return dst
}

func startSim(t *testing.T, dir string, configure func(*sim.Device)) string {
	t.Helper()
	d, err := sim.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if configure != nil {
		configure(d)
	}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) emit(typ string, _ any) {
	r.mu.Lock()
	r.events = append(r.events, typ)
	r.mu.Unlock()
}

// newService returns a started service in offline mode (no network discovery);
// tests feed addresses through handle().
func newService(t *testing.T, mutate func(*store.Settings)) (*Devices, *store.Store, context.Context) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Update(func(s *store.Settings) {
		s.Scan.Mode = store.ScanOffline
		if mutate != nil {
			mutate(s)
		}
	})
	ProbeTimeout = 2 * time.Second
	errorsRetryAfter, ghostsRetryAfter = time.Hour, time.Hour
	rec := &recorder{}
	m := NewDevices(st, shelly.NewClient(), rec.emit, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); m.Wait() })
	m.Start(ctx)
	return m, st, ctx
}

func waitDevice(t *testing.T, m *Devices, id string, cond func(model.Device) bool) model.Device {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if d, ok := m.Get(id); ok && cond(d) {
			return d
		}
		time.Sleep(20 * time.Millisecond)
	}
	d, _ := m.Get(id)
	t.Fatalf("device %s: condition not met; last %+v", id, d)
	return d
}

func online(d model.Device) bool { return d.Status == model.StatusOnline }

func TestGen1AndGen2Identification(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g1 := startSim(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	g2 := startSim(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json": `{"name":null,"id":"shellyplus1-aabbcc000009","mac":"AABBCC000009","slot":0,"model":"SNSW-001X16EU","gen":2,"fw_id":"x","ver":"1.7.5","app":"Plus1","auth_en":false,"auth_domain":null}`,
	}), nil)
	m.handle(ctx, g1, "shellyplug-s-aabbcc000001", true)
	m.handle(ctx, g2, "shellyplus1-aabbcc000009", true)

	d1 := waitDevice(t, m, "AABBCC000001", online)
	if d1.Gen != "1" || d1.TypeName != "PlugS" || d1.Hostname != "shellyplug-s-AABBCC000001" || d1.Name != "Test" || !d1.Managed || d1.LastSeen == 0 {
		t.Fatalf("gen1 device = %+v", d1)
	}
	d2 := waitDevice(t, m, "AABBCC000009", online)
	if d2.Gen != "2" || d2.TypeName != "Shelly +1" || d2.Hostname != "shellyplus1-aabbcc000009" {
		t.Fatalf("gen2 device = %+v", d2)
	}
	// Found again under its custom mDNS name: still one row per MAC.
	m.handle(ctx, g2, "lampenhallbovenswitch", true)
	time.Sleep(300 * time.Millisecond)
	if n := len(m.List()); n != 2 {
		t.Fatalf("%d devices, want 2", n)
	}
}

func TestUnmanagedAndNotShelly(t *testing.T) {
	m, _, ctx := newService(t, nil)
	notShelly := httptest.NewServer(http.NotFoundHandler())
	defer notShelly.Close()
	addr := strings.TrimPrefix(notShelly.URL, "http://")

	m.handle(ctx, addr, "printer", true) // not shelly*, ignored
	if len(m.List()) != 0 {
		t.Fatal("non-Shelly host listed")
	}
	m.handle(ctx, addr, "shellybulbduo-aabbcc0000ff", true) // shelly* host: listed with error
	d := waitDevice(t, m, "AABBCC0000FF", func(d model.Device) bool { return true })
	if d.Status != model.StatusError || d.TypeName != "Generic" || d.Gen != "-" || d.Managed || d.Error == "" {
		t.Fatalf("unmanaged device = %+v", d)
	}
}

func TestProtectedGen2NeedsCredentials(t *testing.T) {
	m, _, ctx := newService(t, nil)
	dir := fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json": `{"id":"shellyplus1-aabbcc000002","mac":"AABBCC000002","model":"SNSW-001X16EU","gen":2,"app":"Plus1","auth_en":true}`,
	})
	addr := startSim(t, dir, func(d *sim.Device) { d.Password = "s3cret" })
	m.handle(ctx, addr, "shellyplus1-aabbcc000002", true)
	d := waitDevice(t, m, "AABBCC000002", func(d model.Device) bool { return d.Status == model.StatusLogin })
	if d.TypeName != "Shelly +1" {
		t.Fatalf("protected device not identified: %+v", d)
	}
	if err := m.SetGlobalCredentials(shelly.Credentials{Password: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	waitDevice(t, m, "AABBCC000002", online)
	if !m.Credentials().GlobalSet {
		t.Fatal("global credentials not reported")
	}
}

func TestProtectedGen1PerDeviceCredentials(t *testing.T) {
	m, _, ctx := newService(t, nil)
	dir := fixtureDir(t, "gen1/SHPLG-S", map[string]string{
		"shelly.json": `{"type":"SHPLG-S","mac":"AABBCC000003","auth":true,"fw":"x"}`,
	})
	addr := startSim(t, dir, func(d *sim.Device) { d.User, d.Password = "admin", "pw" })
	m.handle(ctx, addr, "shellyplug-s-aabbcc000003", true)
	waitDevice(t, m, "AABBCC000003", func(d model.Device) bool { return d.Status == model.StatusLogin })
	if err := m.SetDeviceCredentials("AABBCC000003", shelly.Credentials{User: "admin", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	waitDevice(t, m, "AABBCC000003", online)
}

func TestRangeExtenderClients(t *testing.T) {
	m, _, ctx := newService(t, nil)
	behind := startSim(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	_, port, _ := strings.Cut(behind, ":")
	cfg, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "gen2/Plus1/rpc_Shelly.GetConfig.json"))
	var c map[string]any
	json.Unmarshal(cfg, &c)
	c["wifi"].(map[string]any)["ap"] = map[string]any{"enable": true, "range_extender": map[string]any{"enable": true}}
	cb, _ := json.Marshal(c)
	ext := startSim(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"rpc_Shelly.GetConfig.json":   string(cb),
		"rpc_WiFi.ListAPClients.json": `{"ts":1,"ap_clients":[{"mac":"AA:BB:CC:00:00:01","ip":"192.168.33.2","mport":` + port + `,"since":1}]}`,
	}), nil)
	m.handle(ctx, ext, "shellyplus1-aabbcc000001", true)
	d := waitDevice(t, m, "AABBCC000001", func(d model.Device) bool { return d.Gen == "1" && online(d) })
	if strconv.Itoa(d.Port) != port {
		t.Fatalf("extender client at port %d, want %s", d.Port, port)
	}
}

func TestBLUThroughGateway(t *testing.T) {
	m, _, ctx := newService(t, nil)
	comps := `{"components":[
	  {"key":"blutrv:200","status":{"rssi":-70,"last_updated_ts":1790000000},"config":{"id":200,"addr":"aa:bb:cc:00:00:20","name":"Radiator","trv":"bthomedevice:201"},"attrs":{}},
	  {"key":"bthomedevice:201","status":{"rssi":-70},"config":{"id":201,"addr":"aa:bb:cc:00:00:20","name":"Radiator"},"attrs":{"model_id":8}},
	  {"key":"bthomedevice:202","status":{"rssi":-60,"last_updated_ts":1790000000},"config":{"id":202,"addr":"aa:bb:cc:00:00:21","name":"Hall H&T"},"attrs":{"model_id":3}},
	  {"key":"bthomedevice:203","status":{"rssi":0},"config":{"id":203,"addr":"aa:bb:cc:00:00:22","name":"Asleep"},"attrs":{"model_id":1}}
	],"offset":0,"total":4}`
	gw := startSim(t, fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json":                           `{"id":"shellydimmerg3-aabbcc000010","mac":"AABBCC000010","model":"S3DM-0A101WWL","gen":3,"app":"DimmerG3","auth_en":false}`,
		"rpc_Shelly.GetComponents.json":         comps,
		"rpc_BTHomeDevice.GetKnownObjects.json": `{"id":202,"objects":[{"obj_id":69,"idx":0,"component":"bthomesensor:204"},{"obj_id":1,"idx":0,"component":"bthomesensor:205"},{"obj_id":58,"idx":0,"component":null}]}`,
		"rpc_BluTrv.GetRemoteDeviceInfo.json":   `{"device_info":{"id":"shellyblutrv-aabbcc000020"}}`,
		"rpc_BluTrv.GetStatus.json":             `{"id":200,"rssi":-70,"last_updated_ts":1790000000,"battery":90}`,
		"rpc_BluTrv.GetConfig.json":             `{"id":200,"name":"Radiator"}`,
		"rpc_BluTrv.Call.json":                  `{"rules":[{"rule_id":1,"timespec":"0 0 7 * * *","target_C":21}]}`,
	}), nil)
	m.handle(ctx, gw, "shellydimmerg3-aabbcc000010", true)

	trv := waitDevice(t, m, "AABBCC000020", online)
	if trv.TypeName != "Blu TRV" || trv.Gen != "blu" || trv.Hostname != "shellyblutrv-aabbcc000020" || trv.Parent != "AABBCC000010" {
		t.Fatalf("TRV = %+v", trv)
	}
	ht := waitDevice(t, m, "AABBCC000021", online)
	if ht.TypeName != "Blu H&T" || ht.Gen != "bth" || ht.Hostname != "Bs1s69-aa:bb:cc:00:00:21" || ht.Name != "Hall H&T" {
		t.Fatalf("H&T = %+v", ht)
	}
	asleep := waitDevice(t, m, "AABBCC000022", func(model.Device) bool { return true })
	if asleep.Status != model.StatusOffline {
		t.Fatalf("BLU with rssi 0 should be offline: %+v", asleep)
	}
	// The TRV's own bthomedevice:201 must not become a second row.
	n := 0
	for _, d := range m.List() {
		if d.ID == "AABBCC000020" {
			n++
		}
	}
	if n != 1 || len(m.List()) != 4 {
		t.Fatalf("devices: %d (TRV rows %d), want 4 (1)", len(m.List()), n)
	}
	// The scheduler's calls to a TRV go to the gateway as BluTrv.Call.
	res, err := m.DeviceRPC(ctx, "AABBCC000020", "TRV.ListScheduleRules", []byte(`{"id":0}`))
	if err != nil || !strings.Contains(string(res), `"rule_id":1`) {
		t.Fatalf("TRV rpc %s %v", res, err)
	}
}

func TestArchiveGhostsAndNotes(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	st.Update(func(s *store.Settings) { s.Scan.Mode = store.ScanOffline })
	st.SaveArchive([]store.ArchivedDevice{{TypeID: "SHPLG-S", TypeName: "PlugS", Host: "shellyplug-s-aabbcc000001", MAC: "AABBCC000001",
		IP: "192.0.2.1", Port: 80, Gen: "1", Last: 1790000000000, Note: "garden pump", Keyword: "outside"}})
	errorsRetryAfter, ghostsRetryAfter = time.Hour, time.Hour
	m := NewDevices(st, shelly.NewClient(), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); m.Wait() }()
	m.Start(ctx)
	g, ok := m.Get("AABBCC000001")
	if !ok || g.Status != model.StatusGhost || g.Note != "garden pump" {
		t.Fatalf("ghost = %+v, %v", g, ok)
	}
	// The device shows up: it replaces the ghost and keeps the note.
	addr := startSim(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	m.handle(m.run, addr, "shellyplug-s-aabbcc000001", true)
	d := waitDevice(t, m, "AABBCC000001", online)
	if d.Note != "garden pump" || d.Keyword != "outside" || d.IP != "127.0.0.1" {
		t.Fatalf("device after ghost = %+v", d)
	}
	m.saveArchive()
	arc, _ := st.LoadArchive()
	if len(arc) != 1 || arc[0].IP != "127.0.0.1" || arc[0].Note != "garden pump" || arc[0].Name != "Test" {
		t.Fatalf("archive = %+v", arc)
	}
	// Only ghosts can be removed.
	if err := m.Remove("AABBCC000001"); err == nil {
		t.Fatal("online device removed")
	}
}

func TestIPScanMode(t *testing.T) {
	addr := startSim(t, fixtureDir(t, "gen1/SHPLG-S", nil), nil)
	_, port, _ := strings.Cut(addr, ":")
	p, _ := strconv.Atoi(port)
	st, _ := store.Open(t.TempDir())
	if _, err := st.Update(func(s *store.Settings) {
		s.Scan.Mode = store.ScanIP
		s.Scan.Ranges = []discovery.Range{{Base: "127.0.0", First: 1, Last: 1}}
	}); err != nil {
		t.Fatal(err)
	}
	m := NewDevices(st, shelly.NewClient(), nil, nil)
	m.IPScanPort = p
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); m.Wait() }()
	m.Start(ctx)
	waitDevice(t, m, "AABBCC000001", online)
}

func TestPollingIntervalFollowsViewers(t *testing.T) {
	m, _, _ := newService(t, nil)
	if iv, _ := m.interval(); iv != PresenceInterval {
		t.Fatalf("no viewers: %v", iv)
	}
	_, wake := m.interval()
	m.SetViewers(1)
	select {
	case <-wake:
	default:
		t.Fatal("pollers not woken when a browser connects")
	}
	if iv, _ := m.interval(); iv != 2*time.Second {
		t.Fatalf("with viewers: %v", iv)
	}
}

func TestFailedDeviceIsRetried(t *testing.T) {
	m, _, ctx := newService(t, nil)
	old := errorsRetryEvery
	errorsRetryEvery = 200 * time.Millisecond
	defer func() { errorsRetryEvery = old }()
	d, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", nil), nil)
	d.SetDown(true) // still starting when discovered
	m.handle(ctx, addr, "shellyplus1-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", func(x model.Device) bool { return x.Error != "" })
	go m.retryErrorsLoop(ctx)
	time.Sleep(400 * time.Millisecond) // the first retries still fail
	d.SetDown(false)
	waitDevice(t, m, "AABBCC000001", online)
}

func TestRescanKeepsDevicesSearching(t *testing.T) {
	oldProbe, oldAfter := ProbeTimeout, searchProbeAfter
	t.Cleanup(func() { ProbeTimeout, searchProbeAfter = oldProbe, oldAfter })
	st, _ := store.Open(t.TempDir())
	st.Update(func(s *store.Settings) {
		s.Scan.Mode = store.ScanIP
		s.Scan.Ranges = []discovery.Range{{Base: "127.0.0", First: 2, Last: 2}} // the device is not in the range
		s.Archive.Use = false
	})
	errorsRetryAfter, ghostsRetryAfter = time.Hour, time.Hour
	m := NewDevices(st, shelly.NewClient(), nil, nil)
	m.IPScanPort = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); m.Wait() }()
	m.Start(ctx)
	ProbeTimeout, searchProbeAfter = 500*time.Millisecond, 100*time.Millisecond
	d, addr := startSimDev(t, fixtureDir(t, "gen2/Plus1", nil), nil)
	m.handle(m.run, addr, "shellyplus1-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", online)

	// Rescan: the device stays listed as "searching", then is found at its last address.
	m.Rescan()
	if x, ok := m.Get("AABBCC000001"); !ok || x.Status != model.StatusSearching {
		t.Fatalf("right after the rescan: %+v, %v", x, ok)
	}
	waitDevice(t, m, "AABBCC000001", online)

	// Not found again, archive on: it becomes a ghost when the search ends.
	st.Update(func(s *store.Settings) { s.Archive.Use = true })
	m.saveArchive()
	d.SetDown(true)
	m.Rescan()
	waitDevice(t, m, "AABBCC000001", func(x model.Device) bool { return x.Status == model.StatusGhost })

	// Archive off: it leaves the list.
	st.Update(func(s *store.Settings) { s.Archive.Use = false })
	d.SetDown(false)
	m.handle(m.run, addr, "shellyplus1-aabbcc000001", true)
	waitDevice(t, m, "AABBCC000001", online)
	d.SetDown(true)
	m.Rescan()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := m.Get("AABBCC000001"); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("device still listed after the search")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
