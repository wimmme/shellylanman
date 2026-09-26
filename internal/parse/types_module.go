package parse

// Command layouts: the array type a ShellyScanner model returns from
// getModules(), which decides how DevicesCommandCellRenderer and
// DevicesCommandCellEditor draw the "Command" cell.
const (
	LayoutRelay      = "relay"      // RelayInterface[]: one ON/OFF row per relay
	LayoutRoller     = "roller"     // RollerInterface[]: ▲ ■ ▼ and position slider
	LayoutRGBCCT     = "rgbcct"     // RGBCCTInterface[]: the first module, full panel
	LayoutRGBW       = "rgbw"       // RGBWInterface[]: gain and white sliders
	LayoutRGB        = "rgb"        // RGBInterface[]: gain slider
	LayoutThermostat = "thermostat" // ThermostatInterface[]: target slider, ▲ ▼, enable
	LayoutTRVG1      = "trvg1"      // ShellyTRV (Gen1): profile, target slider, ▲ ▼
	LayoutBreaker    = "cb"         // CBreakerInterface[]: toggle with confirmation
	LayoutMixed      = "mixed"      // any other DeviceModule[]
)

// InputEvent is one action configured on an input (Gen1 action URLs, Gen2+
// webhooks). ShellyScanner performs the URLs itself when the event button is
// used; the URLs stay on the server.
type InputEvent struct {
	Event   string   `json:"event"` // e.g. "shortpush_url", "input.button_push", "bthomedevice.single_push"
	Enabled bool     `json:"enabled"`
	URLs    []string `json:"-"`
}

func fptr(v float64) *float64 { return &v }

// layoutOf gives the layout of a model whose modules are all of one kind
// (the typed arrays of the Java classes).
func layoutOf(mods []Module) string {
	if len(mods) == 0 {
		return ""
	}
	k := mods[0].Kind
	for _, m := range mods[1:] {
		if m.Kind != k {
			return LayoutMixed
		}
	}
	switch k {
	case KindRelay:
		return LayoutRelay
	case KindCover:
		return LayoutRoller
	case KindRGBCCT:
		return LayoutRGBCCT
	case KindRGBW:
		return LayoutRGBW
	case KindRGB:
		return LayoutRGB
	case KindThermostat:
		return LayoutThermostat
	case KindBreaker:
		return LayoutBreaker
	}
	return LayoutMixed
}
