package parse

// Meter types, the names of ShellyScanner's Meters.Type (the UI formats them
// with the METER_VAL_* rules of LabelsBundle.properties).
const (
	W     = "W"     // active power
	VA    = "VA"    // apparent power
	VAR   = "VAR"   // reactive power
	PF    = "PF"    // power factor
	V     = "V"     // voltage
	VL    = "VL"    // voltage, add-on voltmeter
	VX    = "VX"    // add-on custom expression
	I     = "I"     // current
	FREQ  = "FREQ"  // frequency
	T     = "T"     // temperature °C
	T1    = "T1"    //
	T2    = "T2"    //
	T3    = "T3"    //
	T4    = "T4"    //
	H     = "H"     // humidity % (integer)
	HD    = "HD"    // humidity % (one decimal)
	L     = "L"     // lux
	LE    = "LE"    // light level enum: 0 dark, 1 twilight, 2 bright
	LIGHT = "LIGHT" // boolean light
	EX    = "EX"    // external switch: 0 open, 1 closed
	PERC  = "PERC"  // 0–100
	NUM   = "NUM"   // counter
	DMM   = "DMM"   // distance mm
	RAIN  = "RAIN"  //
	VIB   = "VIB"   // vibration boolean
	ANG   = "ANG"   //
	ANG1  = "ANG1"  //
	ANG2  = "ANG2"  //
	CHAN  = "CHANNEL"
	BAT   = "BAT"  // battery %
	BATE  = "BATE" // battery low boolean
)

// MeterValue is one measurement.
type MeterValue struct {
	Type  string  `json:"type"`
	Value float64 `json:"value"`
	Name  string  `json:"name,omitempty"` // sensor name (add-on, BLU)
}

// MeterSet is one Meters object of ShellyScanner: a group of values with an
// optional label (LabelHolder), e.g. one energy-meter channel.
type MeterSet struct {
	Label  string       `json:"label,omitempty"`
	Values []MeterValue `json:"values"`
	Total  bool         `json:"total,omitempty"` // EMTotalMeters (Pro 3EM triphase total): left out of per-meter charts
}

func set(label string, kv ...any) MeterSet {
	s := MeterSet{Label: label}
	for i := 0; i+1 < len(kv); i += 2 {
		s.Values = append(s.Values, MeterValue{Type: kv[i].(string), Value: kv[i+1].(float64)})
	}
	return s
}

// Module kinds (ShellyScanner's DeviceModule implementations).
const (
	KindRelay      = "relay"
	KindLight      = "light" // white / dimmer
	KindRGB        = "rgb"
	KindRGBW       = "rgbw"
	KindCCT        = "cct"
	KindRGBCCT     = "rgbcct"
	KindCover      = "cover"
	KindInput      = "input"
	KindThermostat = "thermostat"
	KindBreaker    = "cb"
	KindCamera     = "camera"
	KindSensor     = "sensor" // read-only state: motion, flood, smoke, door/window, presence
)

// Module is the state of one controllable or observable part of a device, as
// shown in the "Command" column, plus what its controls need (ranges, input
// events). Key names the component a command goes to ("switch:0", "relay/0",
// "cover:1", "number:202", ...); the service maps (Kind, Key, action) to the
// Shelly call ShellyScanner makes.
type Module struct {
	Kind       string   `json:"kind"`
	Index      int      `json:"index"`
	Key        string   `json:"key,omitempty"`
	Label      string   `json:"label"`
	On         *bool    `json:"on,omitempty"`
	Brightness *int     `json:"brightness,omitempty"`
	Gain       *int     `json:"gain,omitempty"`     // RGB gain 0–100
	Position   *int     `json:"position,omitempty"` // cover, TRV valve
	Calibrated *bool    `json:"calibrated,omitempty"`
	Target     *float64 `json:"target,omitempty"` // thermostat target °C
	InputOn    *bool    `json:"inputOn,omitempty"`
	InputOn1   *bool    `json:"inputOn1,omitempty"` // cover: second ("down") input
	RGB        []int    `json:"rgb,omitempty"`
	White      *int     `json:"white,omitempty"`
	TempK      *int     `json:"tempK,omitempty"` // colour temperature
	ColorMode  *bool    `json:"colorMode,omitempty"`
	State      string   `json:"state,omitempty"` // cover state, sensor text
	Source     string   `json:"source,omitempty"`

	// Ranges: brightness (lights) or target temperature (thermostats).
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Div  int      `json:"div,omitempty"` // thermostat steps per °C (ThermostatInterface.getUnitDivision)
	TMin int      `json:"tMin,omitempty"`
	TMax int      `json:"tMax,omitempty"`

	Enabled  *bool        `json:"enabled,omitempty"`  // thermostat enabled; input in use
	Running  *bool        `json:"running,omitempty"`  // thermostat output active
	Schedule *bool        `json:"schedule,omitempty"` // Gen1 TRV: schedule active
	Locked   *bool        `json:"locked,omitempty"`   // circuit breaker safety lock
	Motion   *bool        `json:"motion,omitempty"`   // camera
	Events   []InputEvent `json:"events,omitempty"`
}

// Readings is everything parsed from one configuration + status pair.
type Readings struct {
	Name           string     `json:"-"`
	RSSI           int        `json:"rssi"`
	SSID           string     `json:"ssid"`
	CloudEnabled   bool       `json:"cloudEnabled"`
	CloudConnected bool       `json:"cloudConnected"`
	MQTTEnabled    bool       `json:"mqttEnabled"`
	MQTTConnected  bool       `json:"mqttConnected"`
	Uptime         int        `json:"uptime"`  // seconds; -1 unknown
	LogMode        string     `json:"logMode"` // NONE FILE MQTT SOCKET UDP UNDEFINED
	RebootRequired bool       `json:"rebootRequired"`
	RangeExtender  bool       `json:"-"`
	InternalTemp   *float64   `json:"internalTemp,omitempty"`
	Meters         []MeterSet `json:"meters,omitempty"`
	Modules        []Module   `json:"modules,omitempty"`
	Layout         string     `json:"layout,omitempty"`
	AddonType      string     `json:"-"` // Gen2+ sys.device.addon_type

	// UpdateAvailable: the device itself says a newer stable firmware exists
	// (Gen1 has_update, Gen2+ sys.available_updates.stable); UpdateVersion is that version.
	UpdateAvailable bool   `json:"updateAvailable"`
	UpdateVersion   string `json:"updateVersion,omitempty"`
}

func ptr[T any](v T) *T { return &v }
