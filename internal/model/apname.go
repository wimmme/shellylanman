package model

import (
	"regexp"
	"sort"
	"strings"
)

// The name of a device's own access point says what it is (DECISIONS P20-4,
// docs/phase-20-ap-wizards.md §1):
//
//	Gen2+: Shelly<App>-<MAC>   ShellyPlus2PM-A8032AB636EC   (the default name; it can be changed)
//	Gen1:  <slug>-<MAC>        shellyplug-s-80646F838136     (the host name, MAC of 6 or 12 hex digits)

// gen1Slugs maps the host-name slug of a Gen1 device to its type. Seen on real
// devices: shelly1, shelly1l, shellyuni, shellyix3, shellyrgbw2, shellyplug-s;
// the others are from Shelly's Gen1 documentation and not yet seen here. A slug
// that is missing or wrong only means "not recognised": the user then picks the model.
var gen1Slugs = map[string]string{
	"shelly1": "SHSW-1", "shelly1pm": "SHSW-PM", "shelly1l": "SHSW-L",
	"shellyswitch": "SHSW-21", "shellyswitch25": "SHSW-25",
	"shellyem": "SHEM", "shellyem3": "SHEM-3",
	"shellydimmer": "SHDM-1", "shellydimmer2": "SHDM-2",
	"shellyplug": "SHPLG-1", "shellyplug-s": "SHPLG-S", "shellyplug-u1": "SHPLG-U1", "shellyplug-e": "SHPLG2-1",
	"shellyrgbw2": "SHRGBW2", "shellyuni": "SHUNI-1", "shellyix3": "SHIX3-1",
	"shellybutton1": "SHBTN-2", "shellyht": "SHHT-1", "shellyflood": "SHWT-1",
	"shellydw": "SHDW-1", "shellydw2": "SHDW-2",
	"shellymotionsensor": "SHMOS-01", "shellymotion2": "SHMOS-02", "shellytrv": "SHTRV-01",
	"shellybulb": "SHBLB-1", "shellybulbduo": "SHBDUO-1", "shellycolorbulb": "SHCB-1",
}

var (
	mac12 = regexp.MustCompile(`^[0-9A-Fa-f]{12}$`)
	mac6  = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)
)

// APName is what a name of an access point says.
type APName struct {
	SSID string // as given
	Gen  string // "1", or "2" for Gen2 and newer (the firmware index's generations)
	Key  string // Gen1 type, or Gen2+ app: the key of the firmware index
	MAC  string // upper case, 12 digits; "" when the name carries only 6
	Name string // the model's name when the registry knows it, else ""
}

// ParseAPName reads the name of an access point as a phone shows it (or a host
// name). It reports false when the name has no MAC part or no model can be read from it.
func ParseAPName(ssid string) (APName, bool) {
	s := strings.TrimSpace(ssid)
	i := strings.LastIndex(s, "-")
	if i <= 0 || len(s) < 6 || !strings.HasPrefix(strings.ToLower(s), "shelly") {
		return APName{}, false
	}
	head, tail := s[:i], s[i+1:]
	out := APName{SSID: s}
	switch {
	case mac12.MatchString(tail):
		out.MAC = strings.ToUpper(tail)
	case mac6.MatchString(tail):
	default:
		return APName{}, false
	}
	if typ, ok := gen1Slugs[strings.ToLower(head)]; ok { // Gen1: the slug is the host name before the MAC
		out.Gen, out.Key = "1", typ
		out.Name = gen1[typ].name
		return out, true
	}
	if out.MAC == "" { // a Gen2+ name always has the whole MAC
		return APName{}, false
	}
	app := head[len("shelly"):]
	if key, name, ok := gen2App(app); ok { // the registry knows it: take its spelling
		out.Gen, out.Key, out.Name = "2", key, name
		return out, true
	}
	// Not in the registry (Gen4, new models): keep the app as written when the name has its
	// capitals; an all-lower-case host name cannot tell them, and the index is case-sensitive.
	if app == "" || app == strings.ToLower(app) {
		return APName{}, false
	}
	out.Gen, out.Key = "2", app
	return out, true
}

// gen2App finds an app in the Gen2 and Gen3 registries, ignoring case.
func gen2App(app string) (key, name string, ok bool) {
	for _, m := range []map[string]entry{gen2, gen3} {
		for k, e := range m {
			if strings.EqualFold(k, app) {
				return k, e.name, true
			}
		}
	}
	return "", "", false
}

// DefaultAPName is the name a device's access point has unless it was changed:
// the host name of a Gen1 device, Shelly<App>-<MAC> for Gen2 and newer.
func DefaultAPName(gen, hostname, app, mac string) string {
	if gen == "1" {
		return hostname
	}
	if app == "" || mac == "" {
		return ""
	}
	return "Shelly" + app + "-" + strings.ToUpper(mac)
}

// ModelChoice is a model the user can pick when an access point's name does not say.
type ModelChoice struct {
	Gen  string `json:"gen"` // "1", or "2" for Gen2 and newer
	Key  string `json:"key"` // Gen1 type or Gen2+ app
	Name string `json:"name"`
}

// ModelChoices lists the models the registry knows by the key of the firmware index
// (Gen1 types, Gen2 and Gen3 apps), by name.
func ModelChoices() []ModelChoice {
	var out []ModelChoice
	for k, e := range gen1 {
		out = append(out, ModelChoice{Gen: "1", Key: k, Name: e.name})
	}
	for _, m := range []map[string]entry{gen2, gen3} {
		for k, e := range m {
			if strings.HasSuffix(k, "ProAddon") { // the same firmware as the app without it
				continue
			}
			out = append(out, ModelChoice{Gen: "2", Key: k, Name: e.name})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name); a != b {
			return a < b
		}
		return out[i].Key < out[j].Key
	})
	return out
}
