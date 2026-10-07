package service

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// RPC method classes, for the MCP's RPC tools and for scenes.
const (
	RPCRead     = "read"      // Get*, List*, Check*: changes nothing
	RPCWrite    = "write"     // changes state or configuration
	RPCRisky    = "risky"     // restarts, updates, deletes, code, credentials: never in a scene
	RPCDataLoss = "data-loss" // factory reset, Wi-Fi reset, delete-all: needs an extra flag
)

var rpcReadRe = regexp.MustCompile(`^[A-Za-z0-9]+\.(Get[A-Za-z]*|List[A-Za-z]*|Check[A-Za-z]*)$`)

// dataLoss: methods that wipe the device or a whole category of its data.
var dataLoss = map[string]bool{
	"shelly.factoryreset": true, "shelly.resetwificonfig": true,
	"schedule.deleteall": true, "webhook.deleteall": true, "kvs.deleteall": true,
}

// risky: methods that restart, update, remove, run code or set credentials.
var risky = map[string]bool{
	"shelly.reboot": true, "shelly.update": true, "shelly.setauth": true,
	"script.putcode": true, "script.eval": true, "script.delete": true,
	"schedule.delete": true, "webhook.delete": true, "kvs.delete": true,
	"virtual.delete": true, "bthome.deletedevice": true, "bthome.deletesensor": true,
	"sys.setconfig": true, "wifi.setconfig": true, "mqtt.setconfig": true, "cloud.setconfig": true,
}

// ClassifyRPC tells what calling method does (method names are case-insensitive on the device).
func ClassifyRPC(method string) string {
	lower := strings.ToLower(method)
	switch {
	case rpcReadRe.MatchString(method):
		return RPCRead
	case dataLoss[lower]:
		return RPCDataLoss
	case risky[lower] || strings.HasSuffix(lower, ".delete"):
		return RPCRisky
	}
	return RPCWrite
}

// ErrRPCBlocked: the method may not be called through the raw RPC endpoint.
var ErrRPCBlocked = errors.New("this method is not available here")

// CheckRPC decides whether the raw RPC endpoint (the scheduler's calls and its
// "test method" button) passes a method on (DECISIONS P19-5): reads and ordinary
// changes always; restarts, updates, deletes, code, credentials and settings
// blocks only with confirm; a factory reset, a Wi-Fi reset and the delete-all
// methods never (they have their own, confirmed flows in the UI).
func CheckRPC(method string, confirm bool) error {
	switch ClassifyRPC(method) {
	case RPCDataLoss:
		return fmt.Errorf("%w: %s wipes the device", ErrRPCBlocked, method)
	case RPCRisky:
		// The scheduler removes its own entries like any edit: no question.
		if !confirm && !strings.EqualFold(method, "Schedule.Delete") {
			return ErrConfirm
		}
	}
	return nil
}
