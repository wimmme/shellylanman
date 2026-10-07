package service

import (
	"errors"
	"testing"
)

func TestClassifyRPC(t *testing.T) {
	for method, want := range map[string]string{
		"Shelly.GetStatus":       RPCRead,
		"KVS.GetMany":            RPCRead,
		"Schedule.List":          RPCRead,
		"Shelly.CheckForUpdate":  RPCRead,
		"Switch.Set":             RPCWrite,
		"Switch.SetConfig":       RPCWrite,
		"KVS.Set":                RPCWrite,
		"Shelly.Reboot":          RPCRisky,
		"shelly.update":          RPCRisky,
		"Script.Eval":            RPCRisky,
		"Virtual.Delete":         RPCRisky,
		"Foo.Delete":             RPCRisky,
		"Shelly.SetAuth":         RPCRisky,
		"Shelly.FactoryReset":    RPCDataLoss,
		"Schedule.DeleteAll":     RPCDataLoss,
		"Shelly.ResetWiFiConfig": RPCDataLoss,
	} {
		if got := ClassifyRPC(method); got != want {
			t.Errorf("%s: %s, want %s", method, got, want)
		}
	}
}

func TestCheckRPC(t *testing.T) { // DECISIONS P19-5
	for _, c := range []struct {
		method  string
		confirm bool
		want    error
	}{
		{"Shelly.GetStatus", false, nil},
		{"Switch.Set", false, nil},
		{"Schedule.Create", false, nil},
		{"Schedule.Delete", false, nil}, // the scheduler's own editing
		{"Shelly.Reboot", false, ErrConfirm},
		{"Shelly.Reboot", true, nil},
		{"shelly.update", false, ErrConfirm},
		{"Script.Eval", false, ErrConfirm},
		{"Sys.SetConfig", false, ErrConfirm},
		{"Sys.SetConfig", true, nil},
		{"Foo.Delete", false, ErrConfirm},
		{"Shelly.FactoryReset", false, ErrRPCBlocked},
		{"Shelly.FactoryReset", true, ErrRPCBlocked}, // confirm does not unlock it
		{"Schedule.DeleteAll", true, ErrRPCBlocked},
	} {
		if got := CheckRPC(c.method, c.confirm); !errors.Is(got, c.want) {
			t.Errorf("CheckRPC(%s, %v) = %v, want %v", c.method, c.confirm, got, c.want)
		}
	}
}
