package service

import (
	"errors"
	"strings"
	"testing"
)

func TestDeviceRPCAndHints(t *testing.T) {
	m, _, ctx := newService(t, nil)
	g2 := plus1(t, m, ctx, map[string]string{
		"rpc_Schedule.List.json":        `{"jobs":[{"id":1,"enable":true,"timespec":"0 0 7 * * 1,2,3,4,5","calls":[{"method":"switch.set","params":{"id":0,"on":true}}]}],"rev":3}`,
		"rpc_Schedule.Create.json":      `{"id":2,"rev":4}`,
		"rpc_Shelly.GetComponents.json": `{"components":[{"key":"switch:0","config":{"id":0,"name":"Pump"}},{"key":"cover:0","config":{"id":0,"name":null,"slat":{"enable":true}}},{"key":"sys","config":{}}],"offset":0,"total":3}`,
	}, nil)
	const id = "AABBCC000002"
	res, err := m.DeviceRPC(ctx, id, "Schedule.List", nil)
	if err != nil || !strings.Contains(string(res), `"timespec":"0 0 7 * * 1,2,3,4,5"`) {
		t.Fatalf("list %s %v", res, err)
	}
	res, err = m.DeviceRPC(ctx, id, "Schedule.Create", []byte(`{"timespec":"0 0 8 * * *","calls":[{"method":"switch.toggle","params":{"id":0}}],"enable":true}`))
	if err != nil || !strings.Contains(string(res), `"id":2`) || !callPrefix(g2, "RPC Schedule.Create ") {
		t.Fatalf("create %s %v", res, err)
	}
	if _, err := m.DeviceRPC(ctx, id, " ", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("no method: %v", err)
	}
	hints, err := m.ScheduleHints(ctx, id)
	if err != nil || len(hints) != 3+5 || hints[0].Name != "Pump - On" || hints[0].Params != `"id":0,"on":true` || hints[7].Name != "cover:0 - Go 50%, Slat 50%" {
		t.Fatalf("hints %+v %v", hints, err)
	}
}

func TestDeviceRPCNotForGen1(t *testing.T) {
	m, _, ctx := newService(t, nil)
	plugS(t, m, ctx, nil, "AABBCC000001")
	if _, err := m.DeviceRPC(ctx, "AABBCC000001", "Shelly.GetStatus", nil); !errors.Is(err, ErrBadCommand) {
		t.Fatalf("gen1: %v", err)
	}
}
