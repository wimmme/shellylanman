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
// shown in the "Command" column. Controls arrive in Phase 4; here it is state.
type Module struct {
	Kind       string   `json:"kind"`
	Index      int      `json:"index"`
	Label      string   `json:"label"`
	On         *bool    `json:"on,omitempty"`
	Brightness *int     `json:"brightness,omitempty"`
	Position   *int     `json:"position,omitempty"` // cover, TRV valve
	Calibrated *bool    `json:"calibrated,omitempty"`
	Target     *float64 `json:"target,omitempty"` // thermostat target °C
	InputOn    *bool    `json:"inputOn,omitempty"`
	RGB        []int    `json:"rgb,omitempty"`
	White      *int     `json:"white,omitempty"`
	TempK      *int     `json:"tempK,omitempty"` // colour temperature
	State      string   `json:"state,omitempty"` // cover state, sensor text
	Source     string   `json:"source,omitempty"`
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
	AddonType      string     `json:"-"` // Gen2+ sys.device.addon_type
}

func ptr[T any](v T) *T { return &v }
