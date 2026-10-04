package service

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestScriptsAndKVS(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, map[string]string{
		"rpc_Script.List.json":      `{"scripts":[{"id":1,"name":"blink","enable":true,"running":false},{"id":2,"name":"ble","enable":false,"running":true}]}`,
		"rpc_Script.GetConfig.json": `{"id":3,"name":"script_3","enable":false}`,
		"rpc_Script.Create.json":    `{"id":3}`,
		"rpc_Script.GetCode.json":   `{"data":"let a = 1;\r\nprint(a);"}`,
		"_behaviour.json":           `{"/rpc/Script.GetCode?id=2":{"status":500}}`,
		"rpc_KVS.GetMany.json":      `{"items":[{"key":"k1","etag":"e1","value":"v1"},{"key":"k2","etag":"e2","value":"v2"}],"offset":0,"total":2}`,
		"rpc_KVS.Set.json":          `{"etag":"new","rev":7}`,
	}, nil)
	const id = "AABBCC000002"

	v, err := m.Scripts(ctx, id)
	if err != nil || len(v.Scripts) != 2 || v.Scripts[1].Name != "ble" || !v.Scripts[1].Running || len(v.KVS) != 2 || v.KVS[1].Value != "v2" {
		t.Fatalf("view %+v %v", v, err)
	}
	s, err := m.ScriptCreate(ctx, id, "")
	if err != nil || s.ID != 3 || s.Name != "script_3" || !hasCall(g2, "RPC Script.Create {}") {
		t.Fatalf("create %+v %v %v", s, err, g2.Calls())
	}
	name, on := "renamed", true
	if err := m.ScriptUpdate(ctx, id, 1, &name, &on); err != nil || !hasCall(g2, `RPC Script.SetConfig {"config":{"enable":true,"name":"renamed"},"id":1}`) {
		t.Fatalf("update %v %v", err, g2.Calls())
	}
	if err := m.ScriptRun(ctx, id, 1, true, true); err != nil || !callPrefix(g2, "RPC Sys.SetConfig ") || !hasCall(g2, "GET /rpc/Script.Start?id=1") {
		t.Fatalf("run with log %v %v", err, g2.Calls())
	}
	if code, err := m.ScriptCode(ctx, id, 1); err != nil || code != "let a = 1;\nprint(a);" {
		t.Fatalf("code %q %v", code, err)
	}
	// A device that answers with an error: an error, not "" (an empty editor
	// whose upload would wipe the script — FEATURE_PARITY O34).
	if code, err := m.ScriptCode(ctx, id, 2); err == nil || code != "" {
		t.Fatalf("failing Script.GetCode: %q %v", code, err)
	}
	long := strings.Repeat("x", 2500)
	if err := m.ScriptPutCode(ctx, id, 1, long); err != nil {
		t.Fatal(err)
	}
	var puts []string
	for _, c := range g2.Calls() {
		if strings.HasPrefix(c, "RPC Script.PutCode") {
			puts = append(puts, c)
		}
	}
	if len(puts) != 3 || strings.Contains(puts[0], "append") || !strings.Contains(puts[1], `"append":true`) {
		t.Fatalf("put code in 1024-character segments: %d %v", len(puts), puts)
	}
	it, err := m.KVSSet(ctx, id, "k3", "hello")
	if err != nil || it.Etag != "new" || !hasCall(g2, `RPC KVS.Set {"key":"k3","value":"hello"}`) {
		t.Fatalf("kvs set %+v %v", it, err)
	}
	if err := m.KVSDelete(ctx, id, "a b"); err != nil || !hasCall(g2, "GET /rpc/KVS.Delete?key=a+b") {
		t.Fatalf("kvs delete %v %v", err, g2.Calls())
	}
	if err := m.ScriptDelete(ctx, id, 2); err != nil || !hasCall(g2, "GET /rpc/Script.Delete?id=2") {
		t.Fatalf("delete %v", err)
	}
	if _, err := m.KVSSet(ctx, id, "", "x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty key: %v", err)
	}
}

func TestScriptsNotForGen1(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plugS(t, m, ctx, nil, "AABBCC000001")
	if _, err := m.Scripts(ctx, "AABBCC000001"); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("gen1: %v", err)
	}
}

func TestScriptsInBackup(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range map[string]string{"Shelly.GetConfig.json": "{}", "b.mjs": "print(2);\r\n", "a.mjs": "print(1);"} {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	list, err := ScriptsInBackup(buf.Bytes())
	if err != nil || len(list) != 2 || list[0].Name != "a" || list[1].Code != "print(2);\n" {
		t.Fatalf("%+v %v", list, err)
	}
	if _, err := ScriptsInBackup([]byte("x")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("not a zip: %v", err)
	}
}
