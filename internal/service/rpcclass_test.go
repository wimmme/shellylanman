package service

import "testing"

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
