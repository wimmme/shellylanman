// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// g2/modules/Script, g2/modules/KVS, view/scripts/DialogDeviceScripts,
// ScriptsPanel.loadCodeFromFile and ScriptFrame.activateLogConnection.

package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/ojson"
	"github.com/wimmme/shellylanman/internal/sbk"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// ErrNoScripts: the device offers neither scripts nor KVS (msgScriptKVSNotSupported).
var ErrNoScripts = errors.New("scripts and KVS are not supported by this device")

// ScriptInfo is one row of the Scripts tab.
type ScriptInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Enable  bool   `json:"enable"`
	Running bool   `json:"running"`
}

// KVItem is one row of the KVS tab.
type KVItem struct {
	Key   string `json:"key"`
	Etag  string `json:"etag"`
	Value string `json:"value"`
}

// ScriptsView is what the dialog shows: nil means the tab is not offered.
type ScriptsView struct {
	Scripts []ScriptInfo `json:"scripts"`
	KVS     []KVItem     `json:"kvs"`
}

// g2conn: a Gen2+ device (not BLU) that is reachable.
func (m *Devices) g2conn(id string) (*shelly.Conn, model.Device, error) {
	e, err := m.entryFor(id)
	if err != nil {
		return nil, model.Device{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d := e.dev
	if d.Gen == "1" || d.Gen == model.GenBLU || d.Gen == model.GenBTHome || !d.Managed {
		return nil, d, fmt.Errorf("%w: Gen2+ devices only", ErrBadCommand)
	}
	if e.conn == nil || d.Status == model.StatusGhost {
		return nil, d, ErrNoConnection
	}
	return e.conn, d, nil
}

func (m *Devices) rpcGet(ctx context.Context, c *shelly.Conn, path string) (*ojson.Value, error) {
	b, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	return ojson.Parse(b)
}

// Scripts reads both tabs (DialogDeviceScripts): battery devices have no
// scripts; a tab whose request fails is left out.
func (m *Devices) Scripts(ctx context.Context, id string) (*ScriptsView, error) {
	c, d, err := m.g2conn(id)
	if err != nil {
		return nil, err
	}
	v := &ScriptsView{}
	var firstErr error
	if !d.Battery {
		if list, err := m.scriptList(ctx, c); err == nil {
			v.Scripts = list
		} else {
			firstErr = err
		}
	}
	if kvs, err := m.kvsList(ctx, c); err == nil {
		v.KVS = kvs
	} else if firstErr == nil {
		firstErr = err
	}
	if v.Scripts == nil && v.KVS == nil {
		if shelly.IsOffline(firstErr) {
			return nil, firstErr
		}
		return nil, ErrNoScripts
	}
	return v, nil
}

func (m *Devices) scriptList(ctx context.Context, c *shelly.Conn) ([]ScriptInfo, error) {
	v, err := m.rpcGet(ctx, c, "/rpc/Script.List")
	if err != nil {
		return nil, err
	}
	out := []ScriptInfo{}
	for _, s := range v.Get("scripts").Items() {
		out = append(out, scriptOf(s))
	}
	return out, nil
}

func scriptOf(s *ojson.Value) ScriptInfo {
	return ScriptInfo{ID: s.Get("id").Int(), Name: s.Get("name").Text(), Enable: s.Get("enable").Bool(), Running: s.Get("running").Bool()}
}

// ScriptCreate: Script.Create (no name: the device picks one), then its config.
func (m *Devices) ScriptCreate(ctx context.Context, id, name string) (ScriptInfo, error) {
	c, _, err := m.g2conn(id)
	if err != nil {
		return ScriptInfo{}, err
	}
	params := map[string]any{}
	if name != "" {
		params["name"] = name
	}
	b, err := c.Call(ctx, "Script.Create", params)
	if err != nil {
		return ScriptInfo{}, err
	}
	res, _ := ojson.Parse(b)
	cfg, err := m.rpcGet(ctx, c, "/rpc/Script.GetConfig?id="+strconv.Itoa(res.Get("id").Int()))
	if err != nil {
		return ScriptInfo{}, err
	}
	return scriptOf(cfg), nil
}

// ScriptUpdate renames and/or enables a script (Script.SetConfig).
func (m *Devices) ScriptUpdate(ctx context.Context, id string, sid int, name *string, enable *bool) error {
	c, _, err := m.g2conn(id)
	if err != nil {
		return err
	}
	cfg := map[string]any{}
	if name != nil {
		cfg["name"] = *name
	}
	if enable != nil {
		cfg["enable"] = *enable
	}
	if len(cfg) == 0 {
		return invalid("nothing to change")
	}
	return g2call(ctx, c, "Script.SetConfig", map[string]any{"id": sid, "config": cfg})
}

// ScriptDelete: Script.Delete.
func (m *Devices) ScriptDelete(ctx context.Context, id string, sid int) error {
	c, _, err := m.g2conn(id)
	if err != nil {
		return err
	}
	_, err = c.Get(ctx, "/rpc/Script.Delete?id="+strconv.Itoa(sid))
	return err
}

// ScriptRun starts or stops a script. withLog (the editor) first switches
// the device's websocket debug log on, like ScriptFrame.activateLogConnection.
func (m *Devices) ScriptRun(ctx context.Context, id string, sid int, run, withLog bool) error {
	c, d, err := m.g2conn(id)
	if err != nil {
		return err
	}
	if run && withLog && d.LogMode != "SOCKET" {
		_ = g2call(ctx, c, "Sys.SetConfig", map[string]any{"config": map[string]any{"debug": map[string]any{"websocket": map[string]any{"enable": true}}}})
	}
	method := "Script.Stop"
	if run {
		method = "Script.Start"
	}
	_, err = c.Get(ctx, "/rpc/"+method+"?id="+strconv.Itoa(sid))
	return err
}

var crlfRe = regexp.MustCompile(`\r+\n`)

// ScriptCode: Script.GetCode ("" when the device has none, an error only when off line).
func (m *Devices) ScriptCode(ctx context.Context, id string, sid int) (string, error) {
	c, _, err := m.g2conn(id)
	if err != nil {
		return "", err
	}
	v, err := m.rpcGet(ctx, c, "/rpc/Script.GetCode?id="+strconv.Itoa(sid))
	if err != nil {
		if shelly.IsOffline(err) {
			return "", err
		}
		return "", nil
	}
	return crlfRe.ReplaceAllString(v.Get("data").Text(), "\n"), nil
}

// ScriptPutCode: Script.PutCode in segments of 1024 characters (append after the first).
func (m *Devices) ScriptPutCode(ctx context.Context, id string, sid int, code string) error {
	c, _, err := m.g2conn(id)
	if err != nil {
		return err
	}
	r := []rune(crlfRe.ReplaceAllString(code, "\n"))
	if len(r) == 0 {
		return g2call(ctx, c, "Script.PutCode", map[string]any{"id": sid, "code": ""})
	}
	for start := 0; start < len(r); start += 1024 {
		p := map[string]any{"id": sid, "code": string(r[start:min(start+1024, len(r))])}
		if start > 0 {
			p["append"] = true
		}
		if err := g2call(ctx, c, "Script.PutCode", p); err != nil {
			return err
		}
	}
	return nil
}

// BackupScript is a script found in a .sbk.
type BackupScript struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

// ScriptsInBackup lists the scripts of a .sbk (ScriptsPanel.loadCodeFromFile).
func ScriptsInBackup(data []byte) ([]BackupScript, error) {
	files, err := sbk.Read(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	out := []BackupScript{}
	for name, v := range files {
		if base, ok := strings.CutSuffix(name, ".mjs.json"); ok {
			out = append(out, BackupScript{Name: base, Code: crlfRe.ReplaceAllString(v.Get("code").Text(), "\n")})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ---- KVS -----------------------------------------------------------------------------

func (m *Devices) kvsList(ctx context.Context, c *shelly.Conn) ([]KVItem, error) {
	v, err := m.rpcGet(ctx, c, "/rpc/KVS.GetMany")
	if err != nil {
		return nil, err
	}
	out := []KVItem{}
	items := v.Get("items")
	if items.Kind() != ojson.Array { // firmware < 1.5.0: an object keyed by name
		for _, k := range items.Keys() {
			it := items.Get(k)
			out = append(out, KVItem{Key: k, Etag: it.Get("etag").Text(), Value: it.Get("value").Text()})
		}
		return out, nil
	}
	last := -1
	for {
		for _, it := range items.Items() {
			out = append(out, KVItem{Key: it.Get("key").Text(), Etag: it.Get("etag").Text(), Value: it.Get("value").Text()})
		}
		total := v.Get("total").Int()
		offset := v.Get("offset").Int() + items.Len()
		if total <= offset || items.Len() == 0 || offset <= last {
			return out, nil
		}
		last = offset
		if v, err = m.rpcGet(ctx, c, "/rpc/KVS.GetMany?offset="+strconv.Itoa(offset)); err != nil {
			return nil, err
		}
		items = v.Get("items")
	}
}

// KVSSet adds or edits an item (KVS.Set) and returns it with its new etag.
func (m *Devices) KVSSet(ctx context.Context, id, key, value string) (KVItem, error) {
	if key == "" {
		return KVItem{}, invalid("key")
	}
	c, _, err := m.g2conn(id)
	if err != nil {
		return KVItem{}, err
	}
	b, err := c.Call(ctx, "KVS.Set", map[string]string{"key": key, "value": value})
	if err != nil {
		return KVItem{}, err
	}
	res, _ := ojson.Parse(b)
	return KVItem{Key: key, Etag: res.Get("etag").Text(), Value: value}, nil
}

// KVSDelete: KVS.Delete.
func (m *Devices) KVSDelete(ctx context.Context, id, key string) error {
	c, _, err := m.g2conn(id)
	if err != nil {
		return err
	}
	_, err = c.Get(ctx, "/rpc/KVS.Delete?key="+url.QueryEscape(key))
	return err
}
