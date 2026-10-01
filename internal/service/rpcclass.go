package service

import (
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
