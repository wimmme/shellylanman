package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// logLines captures the service's log (slog text lines).
type logLines struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logLines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// find returns the first line holding all parts, or "".
func (l *logLines) find(parts ...string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range strings.Split(l.buf.String(), "\n") {
		ok := true
		for _, p := range parts {
			if !strings.Contains(line, p) {
				ok = false
				break
			}
		}
		if ok && line != "" {
			return line
		}
	}
	return ""
}

func (l *logLines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// testLogs: the log of each service newService made.
var testLogs sync.Map

// captureLog returns m's log from now on (what came before is dropped).
func captureLog(m *Devices) *logLines {
	v, _ := testLogs.Load(m)
	l := v.(*logLines)
	l.mu.Lock()
	l.buf.Reset()
	l.mu.Unlock()
	return l
}

func TestActionLine(t *testing.T) {
	m, _, _ := newService(t, nil)
	l := captureLog(m)
	web := WithActor(context.Background(), Actor{Source: SourceWeb, Addr: "192.168.0.5"})
	m.action(web, "reboot", DeviceRef{ID: "AABBCC000001", Name: "Pump"}, nil)
	m.action(context.Background(), "reboot", DeviceRef{ID: "AABBCC000002"}, errors.New("timeout"))
	m.actionResult(web, "backup", DeviceRef{ID: "AABBCC000003"}, ResultQueued, "")

	if got := l.find(`msg="action reboot"`, "device=AABBCC000001"); !strings.Contains(got, "level=INFO") ||
		!strings.Contains(got, "name=Pump result=ok source=web addr=192.168.0.5") {
		t.Errorf("ok line: %q", got)
	}
	if got := l.find("device=AABBCC000002"); !strings.Contains(got, "level=WARN") ||
		!strings.Contains(got, "result=fail error=timeout source=system") || strings.Contains(got, "addr=") {
		t.Errorf("failed line, no actor: %q", got)
	}
	if got := l.find(`msg="action backup"`); !strings.Contains(got, "level=INFO") || !strings.Contains(got, "result=queued") {
		t.Errorf("queued is not a warning: %q", got)
	}
}

func TestActionsKeepSecretsOut(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plus1(t, m, ctx, map[string]string{
		"rpc_KVS.Set.json":        `{"etag":"new","rev":7}`,
		"rpc_Script.PutCode.json": `{"len":10}`,
	}, nil)
	l := captureLog(m)
	const id = "AABBCC000002"
	mcp := WithActor(ctx, Actor{Source: SourceMCP, Addr: "127.0.0.1"})
	if _, err := m.KVSSet(mcp, id, "token", "s3cret-value"); err != nil {
		t.Fatal(err)
	}
	if err := m.ScriptPutCode(mcp, id, 1, "let password = 's3cret-code';"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DeviceRPC(mcp, id, "Sys.GetStatus", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.DeviceRPC(mcp, id, "Switch.Set", []byte(`{"id":0,"on":true}`)); err != nil {
		t.Fatal(err)
	}
	out := l.String()
	if strings.Contains(out, "s3cret") {
		t.Fatalf("a secret in the log:\n%s", out)
	}
	if l.find(`msg="action kvs.set"`, "key=token", "result=ok", "source=mcp") == "" ||
		l.find(`msg="action script.code"`, "script=1", "bytes=29") == "" {
		t.Fatalf("kvs/script lines missing:\n%s", out)
	}
	if l.find(`msg="action rpc"`, "method=Sys.GetStatus") != "" || l.find(`msg="action rpc"`, "method=Switch.Set", "result=ok") == "" {
		t.Fatalf("rpc: only writes are actions:\n%s", out)
	}
}

func TestActionsOfCommandAndChecklist(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plugS(t, m, ctx, nil, "AABBCC000001")
	l := captureLog(m)
	in := WithActor(ctx, Actor{Source: SourceIngress, Addr: "192.168.0.7"})
	if err := m.Command(in, "AABBCC000001", Command{Key: "relay/0", Action: ActionOn}); err != nil {
		t.Fatal(err)
	}
	if got := l.find(`msg="action command"`); !strings.Contains(got, "key=relay/0 do=on result=ok source=ingress addr=192.168.0.7") {
		t.Errorf("command line: %q", got)
	}
	if _, _, err := m.ChecklistApply(in, ChecklistAction{IDs: []string{"AABBCC000001"}, Action: CheckEco, Value: true}); err != nil {
		t.Fatal(err)
	}
	if got := l.find(`msg="action checklist.eco"`); !strings.Contains(got, "device=AABBCC000001") || !strings.Contains(got, "value=true") {
		t.Errorf("checklist line: %q\n%s", got, l)
	}
}
