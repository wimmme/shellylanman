// Package model holds the device model shared by discovery, the service layer
// and the API: what the device table shows, independent of generation.
package model

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/parse"
)

// Status mirrors ShellyScanner's ShellyAbstractDevice.Status.
type Status string

const (
	StatusOnline  Status = "online"  // ON_LINE
	StatusOffline Status = "offline" // OFF_LINE
	StatusLogin   Status = "login"   // NOT_LOOGGED: needs credentials
	StatusReading Status = "reading" // READING: refresh or reboot in progress
	StatusError   Status = "error"   // ERROR
	StatusGhost   Status = "ghost"   // GHOST: known from the archive, not seen this session
	// StatusSearching: listed before a rescan and not found again yet (ShellyLanMan
	// only, DECISIONS P12-7); becomes ghost, or leaves the list, after the search.
	StatusSearching Status = "searching"
)

// Generation values; Gen1–Gen4 are "1".."4" like ShellyScanner's getGeneration().
const (
	GenBTHome  = "bth" // BTHomeDevice.GENERATION
	GenBLU     = "blu" // AbstractBTHomeDevice.GENERATION (BLU TRV)
	GenGeneric = "-"   // ShellyGenericUnmanagedImpl
)

// Device is one row of the device table.
type Device struct {
	ID       string `json:"id"`  // MAC, upper case, no separators; the identity (ShellyAbstractDevice.equals)
	MAC      string `json:"mac"` // as the device reports it
	Gen      string `json:"gen"`
	TypeID   string `json:"typeId"`
	TypeName string `json:"typeName"`
	Hostname string `json:"hostname"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Status   Status `json:"status"`
	Managed  bool   `json:"managed"` // false: unknown type or failed to initialise ("unmanaged")
	Battery  bool   `json:"battery"`
	// Protected: the device asks for a password (GET /shelly auth / auth_en).
	Protected bool   `json:"protected,omitempty"`
	Error     string `json:"error,omitempty"`
	LastSeen  int64  `json:"lastSeen"` // unix milliseconds of the last successful contact
	SSID      string `json:"ssid,omitempty"`

	// Read-only information (Phase 3): the remaining device-table columns.
	RSSI           int              `json:"rssi"`
	CloudEnabled   bool             `json:"cloudEnabled"`
	CloudConnected bool             `json:"cloudConnected"`
	MQTTEnabled    bool             `json:"mqttEnabled"`
	MQTTConnected  bool             `json:"mqttConnected"`
	Uptime         int              `json:"uptime"`  // seconds, -1 unknown
	LogMode        string           `json:"logMode"` // NONE FILE MQTT SOCKET UDP UNDEFINED
	InternalTemp   *float64         `json:"internalTemp,omitempty"`
	Meters         []parse.MeterSet `json:"meters,omitempty"`
	Modules        []parse.Module   `json:"modules,omitempty"`
	Layout         string           `json:"layout,omitempty"` // how the Command cell draws the modules (parse.Layout*)
	Paused         bool             `json:"paused,omitempty"` // refresh paused (logs dialog)

	RebootRequired bool `json:"rebootRequired"`

	// The device says a newer stable firmware exists (DECISIONS P19-1).
	UpdateAvailable bool   `json:"updateAvailable"`
	UpdateVersion   string `json:"updateVersion,omitempty"`

	// BLU devices live behind a Gen2+ gateway.
	Parent  string   `json:"parent,omitempty"`  // gateway device ID
	Parents []string `json:"parents,omitempty"` // other gateways that see it (hostnames)
	// Relay: a BLU device the gateway only relays to the Shelly Cloud
	// (BLE.CloudRelay), read only (DECISIONS P14-1).
	Relay bool `json:"relay,omitempty"`

	// From the archive (notes editor).
	Note    string `json:"note,omitempty"`
	Keyword string `json:"keyword,omitempty"`
}

// Address is "ip:port".
func (d Device) Address() string {
	return net.JoinHostPort(d.IP, strconv.Itoa(d.Port))
}

var macRe = regexp.MustCompile(`^[0-9A-F]{12}$`)

// NormalizeMAC turns "aa:bb:cc:00:00:01" or "aabbcc000001" into "AABBCC000001".
func NormalizeMAC(mac string) string {
	return strings.ToUpper(strings.NewReplacer(":", "", "-", "").Replace(mac))
}

// MACFromHostname extracts the MAC ShellyScanner takes from the last 12
// characters of a host name ("shellyplus1-a8032abeb248"), or "" if they are
// not hexadecimal (ShellyGenericUnmanagedImpl).
func MACFromHostname(host string) string {
	if len(host) <= 12 {
		return ""
	}
	tail := strings.ToUpper(host[len(host)-12:])
	if macRe.MatchString(tail) {
		return tail
	}
	return ""
}

// ApplyReadings copies parsed readings onto the device.
func (d *Device) ApplyReadings(r parse.Readings) {
	if r.Name != "" || d.Gen == "1" || d.Gen == "2" || d.Gen == "3" || d.Gen == "4" {
		d.Name = r.Name
	}
	if r.SSID != "" {
		d.SSID = r.SSID
	}
	d.RSSI, d.CloudEnabled, d.CloudConnected = r.RSSI, r.CloudEnabled, r.CloudConnected
	d.MQTTEnabled, d.MQTTConnected = r.MQTTEnabled, r.MQTTConnected
	d.Uptime, d.LogMode, d.RebootRequired = r.Uptime, r.LogMode, r.RebootRequired
	d.UpdateAvailable, d.UpdateVersion = r.UpdateAvailable, r.UpdateVersion
	d.InternalTemp, d.Meters, d.Modules, d.Layout = r.InternalTemp, r.Meters, r.Modules, r.Layout
}
