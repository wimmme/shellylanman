// Package model holds the device model shared by discovery, the service layer
// and the API: what the device table shows, independent of generation.
package model

import (
	"net"
	"regexp"
	"strconv"
	"strings"
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
	Error    string `json:"error,omitempty"`
	LastSeen int64  `json:"lastSeen"` // unix milliseconds of the last successful contact
	SSID     string `json:"ssid,omitempty"`

	RebootRequired bool `json:"rebootRequired"`

	// BLU devices live behind a Gen2+ gateway.
	Parent  string   `json:"parent,omitempty"`  // gateway device ID
	Parents []string `json:"parents,omitempty"` // other gateways that see it (hostnames)

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
