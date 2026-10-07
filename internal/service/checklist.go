// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// view/checklist/CheckListView (g1Row, g2Row, gateways and the row actions),
// AbstractG1Device/AbstractG2Device setEcoMode, setLEDMode, setDebugMode,
// WIFIManagerG2.enableAP, WIFIManager*.enableRoaming,
// RangeExtenderManager.enable and ScheduleManager (auto firmware update).

package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/parse"
)

// Checklist cell values, as the Java table: Boolean, String, Integer, a list
// (BLE) or null. TRUE_STR / FALSE_STR / NOT_APPLICABLE_STR are the symbols
// the original shows.
const (
	TrueStr  = "✓"
	FalseStr = "✗"
	NAStr    = "-"
)

// ChecklistRow is one device of the checklist.
type ChecklistRow struct {
	ID       string       `json:"id"`
	Host     string       `json:"host"`
	Address  string       `json:"address"`
	Status   model.Status `json:"status"`
	Gen      string       `json:"gen"`
	Eco      any          `json:"eco"`
	LED      any          `json:"led"`
	Logs     any          `json:"logs"`
	BLE      any          `json:"ble"`
	AP       any          `json:"ap"`
	Roaming  any          `json:"roaming"`
	WiFi1    any          `json:"wifi1"`
	WiFi2    any          `json:"wifi2"`
	Extender any          `json:"extender"`
	Scripts  any          `json:"scripts"`
	AutoFW   any          `json:"autoFW"`
}

// BLEItem is a BLU device seen by a gateway, or a gateway seeing a BLU device.
type BLEItem struct {
	ID       string `json:"id,omitempty"`   // device ID when it is in the list
	Name     string `json:"name,omitempty"` // its name
	MAC      string `json:"mac,omitempty"`
	Address  string `json:"address,omitempty"`  // gateway address
	LastSeen int64  `json:"lastSeen,omitempty"` // gateway: unix s the BLU device was last seen
}

func boolVal(n parse.Node) any {
	if !n.Exists() {
		return nil
	}
	return n.Bool()
}

// Checklist computes the rows of the given devices (all when ids is empty).
func (m *Devices) Checklist(ctx context.Context, ids []string) []ChecklistRow {
	if len(ids) == 0 {
		for _, d := range m.List() {
			ids = append(ids, d.ID)
		}
	}
	rows := make([]ChecklistRow, len(ids))
	gwMap := &bleGateways{byMAC: map[string][]BLEItem{}}
	var wg sync.WaitGroup
	blu := false
	for i, id := range ids {
		m.mu.Lock()
		e, ok := m.devs[id]
		m.mu.Unlock()
		if !ok {
			continue
		}
		if e.blu != nil {
			blu = true
		}
		wg.Add(1)
		go func(i int, e *entry) {
			defer wg.Done()
			rows[i] = m.checklistRow(ctx, e, gwMap)
		}(i, e)
	}
	wg.Wait()
	if blu { // gateways of BLU devices: every other on-line, mains-powered Gen2+ device
		inList := map[string]bool{}
		for _, id := range ids {
			inList[id] = true
		}
		for _, d := range m.List() {
			if !inList[d.ID] && d.Status == model.StatusOnline && !d.Battery && (d.Gen == "2" || d.Gen == "3" || d.Gen == "4") {
				m.mu.Lock()
				e := m.devs[d.ID]
				m.mu.Unlock()
				if e != nil && e.conn != nil {
					_, _ = m.gateways(ctx, e, gwMap)
				}
			}
		}
		for i := range rows {
			if g := gwMap.byMAC[model.NormalizeMAC(rowMAC(m, rows[i].ID))]; rows[i].Gen == model.GenBLU || rows[i].Gen == model.GenBTHome {
				if g == nil {
					g = []BLEItem{}
				}
				rows[i].BLE = g
			}
		}
	}
	out := rows[:0]
	for _, r := range rows {
		if r.ID != "" {
			out = append(out, r)
		}
	}
	return out
}

func rowMAC(m *Devices, id string) string {
	d, _ := m.Get(id)
	return d.MAC
}

type bleGateways struct {
	mu    sync.Mutex
	byMAC map[string][]BLEItem
}

func (m *Devices) checklistRow(ctx context.Context, e *entry, gw *bleGateways) ChecklistRow {
	m.mu.Lock()
	d := e.dev
	cfgRaw, conn := e.rawConfig, e.conn
	m.mu.Unlock()
	r := ChecklistRow{ID: d.ID, Host: extendedHost(d), Address: d.Address(), Status: d.Status, Gen: d.Gen}
	if e.blu != nil || conn == nil || d.Status == model.StatusGhost || !d.Managed {
		return r
	}
	if d.Gen == "1" {
		n, err := getNode(ctx, conn, "/settings")
		if err != nil {
			if !d.Battery {
				return r
			}
			n = parse.Decode(cfgRaw) // battery device asleep: stored settings
		}
		g1Row(&r, d, n)
		return r
	}
	cfg, err := getNode(ctx, conn, "/rpc/Shelly.GetConfig")
	var st *parse.Node
	if err == nil {
		if s, err := getNode(ctx, conn, "/rpc/Shelly.GetStatus"); err == nil {
			st = &s
		}
	} else {
		if !d.Battery {
			return r
		}
		cfg = parse.Decode(cfgRaw)
	}
	m.g2Row(ctx, &r, e, d, cfg, st, gw)
	return r
}

// extendedHost: UtilMiscellaneous.getExtendedHostName — "name (hostname)".
func extendedHost(d model.Device) string {
	if d.Name != "" && d.Name != d.Hostname {
		return d.Name + " (" + d.Hostname + ")"
	}
	return d.Hostname
}

func g1Row(r *ChecklistRow, d model.Device, s parse.Node) {
	r.Eco = boolVal(s.Get("eco_mode_enabled"))
	r.LED = boolVal(s.Get("led_status_disable"))
	// The debug mode as read now (the original updates it from the answer of
	// setDebugMode before redrawing the row).
	if dbg := s.Get("debug_enable"); dbg.Exists() {
		r.Logs = dbg.Bool()
	} else {
		r.Logs = NAStr
	}
	switch {
	case !s.Get("ap_roaming").Exists():
		r.Roaming = NAStr
	case s.Path("ap_roaming", "enabled").Bool():
		r.Roaming = s.Path("ap_roaming", "threshold").Str(strconv.Itoa(s.Path("ap_roaming", "threshold").Int()))
	default:
		r.Roaming = FalseStr
	}
	r.WiFi1, r.WiFi2 = NAStr, NAStr
	if s.Path("wifi_sta", "enabled").Bool() {
		r.WiFi1 = map[bool]string{true: TrueStr, false: FalseStr}[s.Path("wifi_sta", "ipv4_method").Str("") == "static"]
	}
	if s.Path("wifi_sta1", "enabled").Bool() {
		r.WiFi2 = map[bool]string{true: TrueStr, false: FalseStr}[s.Path("wifi_sta1", "ipv4_method").Str("") == "static"]
	}
	r.BLE, r.AP, r.Extender, r.Scripts, r.AutoFW = nil, NAStr, NAStr, NAStr, NAStr
}

func (m *Devices) g2Row(ctx context.Context, r *ChecklistRow, e *entry, d model.Device, cfg parse.Node, st *parse.Node, gw *bleGateways) {
	r.Eco = boolVal(cfg.Path("sys", "device", "eco_mode"))
	r.AP = boolVal(cfg.Path("wifi", "ap", "enable"))
	var logs []string
	if cfg.Path("sys", "debug", "websocket", "enable").Bool() {
		logs = append(logs, "socket")
	}
	if cfg.Path("sys", "debug", "mqtt", "enable").Bool() {
		logs = append(logs, "mqtt")
	}
	if a := cfg.Path("sys", "debug", "udp", "addr"); a.Exists() && !a.IsNull() {
		logs = append(logs, "udp")
	}
	if len(logs) == 0 {
		r.Logs = false
	} else {
		r.Logs = strings.Join(logs, ", ")
	}
	live := st != nil
	switch {
	case d.Battery:
		r.BLE = NAStr
	case !cfg.Path("ble", "enable").Exists() || cfg.Path("ble", "enable").Bool():
		if live {
			if list, err := m.gateways(ctx, e, gw); err == nil {
				r.BLE = list
			} else if cfg.Path("ble", "enable").Exists() {
				r.BLE = TrueStr
			}
		}
	default:
		r.BLE = FalseStr
	}
	switch {
	case !cfg.Path("wifi", "roam").Exists():
		r.Roaming = NAStr
	case cfg.Path("wifi", "roam", "interval").Int() > 0:
		r.Roaming = strconv.Itoa(cfg.Path("wifi", "roam", "rssi_thr").Int())
	default:
		r.Roaming = FalseStr
	}
	r.WiFi1, r.WiFi2 = NAStr, NAStr
	if cfg.Path("wifi", "sta", "enable").Bool() {
		r.WiFi1 = map[bool]string{true: TrueStr, false: FalseStr}[cfg.Path("wifi", "sta", "ipv4mode").Str("") == "static"]
	}
	if cfg.Path("wifi", "sta1", "enable").Bool() {
		r.WiFi2 = map[bool]string{true: TrueStr, false: FalseStr}[cfg.Path("wifi", "sta1", "ipv4mode").Str("") == "static"]
	}
	ext := cfg.Path("wifi", "ap", "range_extender", "enable")
	switch {
	case !ext.Exists():
		r.Extender = NAStr
	case !live || !ext.Bool():
		r.Extender = FalseStr
	default:
		r.Extender = st.Path("wifi", "ap_client_count").Int()
	}
	r.LED, r.Scripts, r.AutoFW = NAStr, NAStr, NAStr
	if !live {
		return
	}
	if !d.Battery {
		if n, err := getNode(ctx, e.conn, "/rpc/Script.List"); err == nil && n.Get("scripts").Exists() {
			enabled := 0
			for i := 0; i < n.Get("scripts").Len(); i++ {
				if n.Get("scripts").Idx(i).Get("enable").Bool() {
					enabled++
				}
			}
			r.Scripts = fmt.Sprintf("%d / %d", n.Get("scripts").Len(), enabled)
		}
	}
	if stage, err := autoFWUpdate(ctx, e); err == nil {
		if stage == "" {
			r.AutoFW = FalseStr
		} else {
			r.AutoFW = stage
		}
	}
}

// gateways: BLE.CloudRelay.ListInfos of a gateway (paged); the BLU devices it
// relays, and for each BLU MAC the gateways that see it.
func (m *Devices) gateways(ctx context.Context, e *entry, gw *bleGateways) ([]BLEItem, error) {
	m.mu.Lock()
	gwDev := e.dev
	m.mu.Unlock()
	list := []BLEItem{}
	offset := 0
	for {
		path := "/rpc/BLE.CloudRelay.ListInfos"
		if offset > 0 {
			path += "?offset=" + strconv.Itoa(offset)
		}
		n, err := getNode(ctx, e.conn, path)
		if err != nil {
			return nil, err
		}
		devs := n.Get("devices")
		count := 0
		for _, mac := range devs.Keys() {
			count++
			item := BLEItem{MAC: mac}
			if d, ok := m.Get(model.NormalizeMAC(mac)); ok {
				item.ID, item.Name = d.ID, descName(d)
			}
			list = append(list, item)
			gw.mu.Lock()
			key := model.NormalizeMAC(mac)
			gw.byMAC[key] = append(gw.byMAC[key], BLEItem{ID: gwDev.ID, Name: descName(gwDev), Address: gwDev.Address(),
				LastSeen: int64(devs.Path(mac, "last_seen").Int())})
			gw.mu.Unlock()
		}
		total := n.Get("total").Int()
		offset += count
		if count == 0 || offset >= total {
			break
		}
	}
	return list, nil
}

// autoFWUpdate: ScheduleManager.autoFWUpdate — the stage of the first enabled
// schedule that calls Shelly.Update, "" when none.
func autoFWUpdate(ctx context.Context, e *entry) (string, error) {
	jobs, err := scheduleJobs(ctx, e)
	if err != nil {
		return "", err
	}
	for _, j := range jobs {
		if j.stage != "" {
			return j.stage, nil
		}
	}
	return "", nil
}

type fwJob struct {
	id    int
	stage string
}

// scheduleJobs lists the enabled schedules that call Shelly.Update.
func scheduleJobs(ctx context.Context, e *entry) ([]fwJob, error) {
	n, err := getNode(ctx, e.conn, "/rpc/Schedule.List")
	if err != nil {
		return nil, err
	}
	var out []fwJob
	jobs := n.Get("jobs")
	for i := 0; i < jobs.Len(); i++ {
		j := jobs.Idx(i)
		if !j.Get("enable").Bool() {
			continue
		}
		calls := j.Get("calls")
		for k := 0; k < calls.Len(); k++ {
			if strings.EqualFold(calls.Idx(k).Get("method").Str(""), "Shelly.Update") {
				out = append(out, fwJob{id: j.Get("id").Int(), stage: calls.Idx(k).Path("params", "stage").Str("")})
				break
			}
		}
	}
	return out, nil
}

// Checklist actions.
const (
	CheckEco      = "eco"      // Value: new state
	CheckLED      = "led"      // Gen1
	CheckLogs     = "logs"     // Gen1: file log on/off; Gen2+: Mode "socket" or "mqtt" toggled to Value
	CheckAP       = "ap"       // Gen2+; Gen1 too (the wizards)
	CheckRoaming  = "roaming"  // Value: enable
	CheckExtender = "extender" // Gen2+; Value: enable
	CheckAutoFW   = "autofw"   // Gen2+; Mode: "stable", "beta" or "none"
)

// ChecklistAction is one toolbar/popup action on the selected rows.
type ChecklistAction struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action"`
	Value  bool     `json:"value"`
	Mode   string   `json:"mode,omitempty"`
}

// ChecklistApply runs an action on each device and returns the error lines
// (LocalSelectedAction: "name - message") and the re-read rows.
func (m *Devices) ChecklistApply(ctx context.Context, a ChecklistAction) ([]ResultLine, []ChecklistRow, error) {
	var lines []ResultLine
	for _, id := range a.IDs {
		m.mu.Lock()
		e, ok := m.devs[id]
		var d model.Device
		if ok {
			d = e.dev
		}
		m.mu.Unlock()
		if !ok {
			return nil, nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		t := cfgTarget{e: e, d: d, gen1: d.Gen == "1", blu: e.blu != nil}
		var err error
		if !t.usable() {
			err = fmt.Errorf("Status-%s", strings.ToUpper(string(d.Status)))
		} else {
			err = m.checkAction(ctx, t, a)
		}
		if err != nil {
			lines = append(lines, ResultLine{DeviceRef: DeviceRef{ID: id, Name: descName(d)}, Result: ResultFail, Message: msgOf(err)})
		}
	}
	return lines, m.Checklist(ctx, a.IDs), nil
}

func (m *Devices) checkAction(ctx context.Context, t cfgTarget, a ChecklistAction) error {
	c := t.e.conn
	on := strconv.FormatBool(a.Value)
	switch a.Action {
	case CheckEco:
		if t.gen1 {
			err := g1cmd(ctx, c, "/settings?eco_mode_enabled="+on)
			if err == nil { // AbstractG1Device.setEcoMode: the device needs a reboot
				m.apply(t.e, func(d *model.Device) { t.e.g1Reboot = true; d.RebootRequired = true })
			}
			return err
		}
		return g2call(ctx, c, "Sys.SetConfig", map[string]any{"config": map[string]any{"device": map[string]any{"eco_mode": a.Value}}})
	case CheckLED:
		if t.gen1 {
			return g1cmd(ctx, c, "/settings?led_status_disable="+on)
		}
	case CheckLogs:
		if t.gen1 { // setDebugMode(FILE, enable)
			return g1cmd(ctx, c, "/settings?debug_enable="+on)
		}
		switch a.Mode {
		case "socket":
			return g2call(ctx, c, "Sys.SetConfig", map[string]any{"config": map[string]any{"debug": map[string]any{"websocket": map[string]any{"enable": a.Value}}}})
		case "mqtt":
			return g2call(ctx, c, "Sys.SetConfig", map[string]any{"config": map[string]any{"debug": map[string]any{"mqtt": map[string]any{"enable": a.Value}}}})
		}
	case CheckAP:
		if t.gen1 { // not in the original; the access-point wizards need the AP on (DECISIONS §29)
			return g1cmd(ctx, c, "/settings/ap?enabled="+on)
		}
		return g2call(ctx, c, "WiFi.SetConfig", map[string]any{"config": map[string]any{"ap": map[string]any{"enable": a.Value}}})
	case CheckRoaming:
		if t.gen1 {
			return g1cmd(ctx, c, "/settings?ap_roaming_enabled="+on)
		}
		interval := 0
		if a.Value {
			interval = 60
		}
		return g2call(ctx, c, "WiFi.SetConfig", map[string]any{"config": map[string]any{"roam": map[string]any{"interval": interval}}})
	case CheckExtender:
		if !t.gen1 {
			if a.Value {
				return g2call(ctx, c, "WiFi.SetConfig", map[string]any{"config": map[string]any{"ap": map[string]any{"enable": true, "range_extender": map[string]any{"enable": true}}}})
			}
			return g2call(ctx, c, "WiFi.SetConfig", map[string]any{"config": map[string]any{"ap": map[string]any{"range_extender": map[string]any{"enable": false}}}})
		}
	case CheckAutoFW:
		if !t.gen1 {
			return setAutoFW(ctx, t.e, a.Mode)
		}
	}
	return fmt.Errorf("%w: %s", ErrBadCommand, a.Action)
}

// setAutoFW: remove the first enabled Shelly.Update schedule and, for stable
// or beta, create a new one at midnight every day (ScheduleManager.addFWUpdate).
func setAutoFW(ctx context.Context, e *entry, mode string) error {
	if mode != "stable" && mode != "beta" && mode != "none" {
		return fmt.Errorf("%w: auto update %q", ErrBadCommand, mode)
	}
	cur, err := autoFWUpdate(ctx, e)
	if err != nil {
		return err
	}
	if mode == "none" && cur == "" || strings.EqualFold(cur, mode) {
		return nil
	}
	jobs, err := scheduleJobs(ctx, e)
	if err != nil {
		return err
	}
	if len(jobs) > 0 {
		if err := g2call(ctx, e.conn, "Schedule.Delete", map[string]any{"id": jobs[0].id}); err != nil {
			return err
		}
	}
	if mode == "none" {
		return nil
	}
	return g2call(ctx, e.conn, "Schedule.Create", map[string]any{
		"timespec": "0 0 0 * * 0,1,2,3,4,5,6",
		"calls":    []any{map[string]any{"method": "Shelly.Update", "params": map[string]any{"stage": mode}, "origin": "shelly_service"}},
		"enable":   true,
	})
}
