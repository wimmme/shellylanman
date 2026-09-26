// Package parse turns the status and configuration payloads of a device into
// what the device table shows: common fields (RSSI, SSID, cloud, MQTT, uptime,
// logs), internal temperature, measurements and modules.
//
// The per-model rules are ported from ShellyScanner's device classes; see the
// headers of the individual files.
package parse

import (
	"encoding/json"
	"sort"
	"strconv"
)

// node is a read-only view of decoded JSON with forgiving accessors: a missing
// key yields an empty node, like Jackson's path().
type node struct{ v any }

func decode(b []byte) node {
	var v any
	if len(b) > 0 {
		_ = json.Unmarshal(b, &v)
	}
	return node{v}
}

func (n node) Get(key string) node {
	if m, ok := n.v.(map[string]any); ok {
		return node{m[key]}
	}
	return node{}
}

// Path follows keys; "0", "1"… index arrays.
func (n node) Path(keys ...string) node {
	for _, k := range keys {
		if a, ok := n.v.([]any); ok {
			i, err := strconv.Atoi(k)
			if err != nil || i < 0 || i >= len(a) {
				return node{}
			}
			n = node{a[i]}
			continue
		}
		n = n.Get(k)
	}
	return n
}

func (n node) Idx(i int) node {
	if a, ok := n.v.([]any); ok && i >= 0 && i < len(a) {
		return node{a[i]}
	}
	return node{}
}

func (n node) Exists() bool { return n.v != nil }

func (n node) Len() int {
	switch t := n.v.(type) {
	case []any:
		return len(t)
	case map[string]any:
		return len(t)
	}
	return 0
}

func (n node) Has(key string) bool {
	m, ok := n.v.(map[string]any)
	if !ok {
		return false
	}
	_, has := m[key]
	return has
}

func (n node) Keys() []string {
	m, ok := n.v.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (n node) Float() float64 {
	switch t := n.v.(type) {
	case float64:
		return t
	case bool:
		if t {
			return 1
		}
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func (n node) Int() int { return int(n.Float()) }

func (n node) Bool() bool {
	switch t := n.v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t == "true"
	}
	return false
}

// Str returns the string value, or def when absent or not a string.
func (n node) Str(def string) string {
	if s, ok := n.v.(string); ok {
		return s
	}
	return def
}

// IsNull reports an explicit JSON null (present, but null).
func (n node) IsNull() bool { return n.v == nil }

// Node is the forgiving JSON reader, for other packages.
type Node = node

// Decode parses b into a Node (a missing or invalid document reads as empty).
func Decode(b []byte) Node { return decode(b) }
