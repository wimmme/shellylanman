// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// g1/modules/Actions (fillSettings, Action.isActive).

package parse

import (
	"bytes"
	"encoding/json"
)

// gen1Actions reads /settings/actions into the events of each input index,
// keeping the order of the JSON object (the order of the buttons):
//
//	{"actions": {"shortpush_url": [{"index": 0, "enabled": true, "urls": ["http://…"]}, …], …}}
//
// An event is usable when enabled and it has URLs (Action.isActive).
func gen1Actions(b []byte) map[int][]InputEvent {
	out := map[int][]InputEvent{}
	if len(b) == 0 {
		return out
	}
	var top struct {
		Actions json.RawMessage `json:"actions"`
	}
	if json.Unmarshal(b, &top) != nil || len(top.Actions) == 0 {
		return out
	}
	dec := json.NewDecoder(bytes.NewReader(top.Actions))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return out
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return out
		}
		name, _ := t.(string)
		var entries []struct {
			Index   int               `json:"index"`
			Enabled bool              `json:"enabled"`
			URLs    []json.RawMessage `json:"urls"`
		}
		if err := dec.Decode(&entries); err != nil {
			return out
		}
		for _, e := range entries {
			ev := InputEvent{Event: name}
			for _, u := range e.URLs {
				var s string
				if json.Unmarshal(u, &s) == nil {
					ev.URLs = append(ev.URLs, s)
				} else {
					ev.URLs = append(ev.URLs, "") // {"url","int"} entries: Jackson's asString gives ""
				}
			}
			ev.Enabled = e.Enabled && len(ev.URLs) > 0
			out[e.Index] = append(out[e.Index], ev)
		}
	}
	return out
}
