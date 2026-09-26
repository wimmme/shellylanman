// Package fixture defines how recorded device payloads are named and scrubbed.
//
// A fixture set is a directory per device model under testdata/, holding one
// JSON file per request (see FileName). The repository is public, so every
// file is scrubbed before it is written: MAC addresses and IPv4 addresses are
// replaced by consistent fakes, and values of identifying keys are redacted.
package fixture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// FakeMACPrefix starts every replacement MAC; the fixture check accepts only
// MACs with this prefix.
const FakeMACPrefix = "AABBCC"

// FileName maps a request path to its fixture file: "/shelly" → "shelly.json",
// "/settings/actions" → "settings_actions.json", "/rpc/Shelly.GetStatus" →
// "rpc_Shelly.GetStatus.json". Query strings are ignored.
func FileName(requestPath string) string {
	p, _, _ := strings.Cut(requestPath, "?")
	p = strings.Trim(p, "/")
	return strings.ReplaceAll(p, "/", "_") + ".json"
}

// RPCFileName is the fixture file for a Gen2+ RPC method.
func RPCFileName(method string) string { return FileName("/rpc/" + method) }

var (
	macPlain = regexp.MustCompile(`(?i)\b[0-9a-f]{12}\b`)
	macColon = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?::[0-9a-f]{2}){5}\b`)
	ipv4     = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
)

// Keys whose values identify a person, place or network. Matched case-insensitively.
var redactKeys = map[string]string{
	"ssid": "REDACTED", "pass": "REDACTED", "password": "REDACTED", "key": "REDACTED",
	"user": "REDACTED", "username": "REDACTED", "server": "REDACTED", "peer": "REDACTED",
	"name": "Test", "lat": "0", "lng": "0", "lon": "0", "tz": "UTC", "timezone": "UTC",
}

// Scrubber replaces identifying data consistently across all files it sees,
// so the same real MAC becomes the same fake MAC everywhere.
type Scrubber struct {
	macs map[string]string // uppercase 12-hex real → uppercase 12-hex fake
	ips  map[string]string
}

// NewScrubber returns an empty scrubber.
func NewScrubber() *Scrubber {
	return &Scrubber{macs: map[string]string{}, ips: map[string]string{}}
}

// JSON scrubs a JSON document and returns it indented.
func (s *Scrubber) JSON(b []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("not JSON: %w", err)
	}
	v = redact(v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(s.Text(string(out))), '\n'), nil
}

func redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if repl, ok := redactKeys[strings.ToLower(k)]; ok {
				switch val.(type) {
				case string:
					if val != "" {
						t[k] = repl
					}
					continue
				case json.Number:
					t[k] = json.Number("0")
					continue
				}
			}
			t[k] = redact(val)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redact(t[i])
		}
		return t
	default:
		return v
	}
}

// Text replaces MAC and IPv4 addresses in free text.
func (s *Scrubber) Text(str string) string {
	str = macColon.ReplaceAllStringFunc(str, func(m string) string {
		fake := s.fakeMAC(strings.ReplaceAll(m, ":", ""))
		var parts []string
		for i := 0; i < 12; i += 2 {
			parts = append(parts, fake[i:i+2])
		}
		return matchCase(m, strings.Join(parts, ":"))
	})
	str = macPlain.ReplaceAllStringFunc(str, func(m string) string {
		return matchCase(m, s.fakeMAC(m))
	})
	return ipv4.ReplaceAllStringFunc(str, s.fakeIP)
}

func (s *Scrubber) fakeMAC(real string) string {
	up := strings.ToUpper(real)
	if strings.HasPrefix(up, FakeMACPrefix) {
		return up
	}
	if f, ok := s.macs[up]; ok {
		return f
	}
	f := fmt.Sprintf("%s%06X", FakeMACPrefix, len(s.macs)+1)
	s.macs[up] = f
	return f
}

func (s *Scrubber) fakeIP(ip string) string {
	if !IsPrivateIP(ip) {
		return ip // masks, 0.0.0.0, loopback, documentation and public addresses stay
	}
	if f, ok := s.ips[ip]; ok {
		return f
	}
	f := fmt.Sprintf("192.0.2.%d", len(s.ips)+1)
	s.ips[ip] = f
	return f
}

// IsPrivateIP reports whether ip is in an RFC 1918 or link-local range.
func IsPrivateIP(ip string) bool {
	var a, b, c, d int
	if n, _ := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d); n != 4 {
		return false
	}
	switch {
	case a == 10, a == 192 && b == 168, a == 172 && b >= 16 && b <= 31, a == 169 && b == 254:
		return true
	}
	return false
}

// FindUnscrubbed returns identifying tokens left in text: real-looking MACs and
// private IPv4 addresses. Used by the repository check.
func FindUnscrubbed(text string) []string {
	found := map[string]bool{}
	for _, m := range macColon.FindAllString(text, -1) {
		if !strings.HasPrefix(strings.ToUpper(strings.ReplaceAll(m, ":", "")), FakeMACPrefix) {
			found[m] = true
		}
	}
	for _, m := range macPlain.FindAllString(text, -1) {
		if !strings.HasPrefix(strings.ToUpper(m), FakeMACPrefix) && !isAllDigits(m) {
			found[m] = true
		}
	}
	for _, m := range ipv4.FindAllString(text, -1) {
		if IsPrivateIP(m) {
			found[m] = true
		}
	}
	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// matchCase returns repl in the case of orig (hostnames carry the MAC in lower case).
func matchCase(orig, repl string) string {
	if orig == strings.ToLower(orig) {
		return strings.ToLower(repl)
	}
	return repl
}
