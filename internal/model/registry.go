// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0:
// the identification rules of model/DevicesFactory.java and the type IDs and
// type names of the model/device/g1..g4 and blu classes.

package model

import "github.com/wimmme/shellylanman/internal/shelly"

// Model is what the registry knows about a device type.
type Model struct {
	TypeID   string // ShellyScanner's getTypeID(): Gen1 type, Gen2/3 app, Gen4 model
	TypeName string // ShellyScanner's getTypeName(), shown in the "Type" column
	Battery  bool   // BatteryDeviceInterface: sleeps, keeps stored data
	Pro      bool   // AbstractProDevice: scanned for BLU devices like Gen3+
	Known    bool   // false: shown as "Generic Gn" (unmanaged)
}

type entry struct {
	name    string
	battery bool
	pro     bool
}

// Gen1: by "type" (DevicesFactory.createG1).
var gen1 = map[string]entry{
	"SHBTN-2": {"Button 1", true, false}, "SHSW-1": {"Shelly 1", false, false},
	"SHSW-L": {"Shelly 1L", false, false}, "SHSW-PM": {"Shelly 1PM", false, false},
	"SHSW-21": {"Shelly 2", false, false}, "SHSW-25": {"Shelly 2.5", false, false},
	"SHEM-3": {"Shelly 3EM", false, false}, "SHBLB-1": {"Shelly Bulb", false, false},
	"SHBDUO-1": {"DUO", false, false}, "SHCB-1": {"DUO RGB", false, false},
	"SHDW-1": {"Shelly DW", true, false}, "SHDW-2": {"Shelly DW2", true, false},
	"SHDM-1": {"Shelly Dimmer", false, false}, "SHDM-2": {"Shelly Dimmer 2", false, false},
	"SHEM": {"Shelly EM", false, false}, "SHWT-1": {"Shelly Flood", true, false},
	"SHHT-1": {"Shelly H&T", true, false}, "SHIX3-1": {"Shelly I3", false, false},
	"SHMOS-01": {"Motion 1", false, false}, "SHMOS-02": {"Motion", false, false},
	"SHPLG-1": {"Plug", false, false}, "SHPLG2-1": {"Plug E", false, false},
	"SHPLG-S": {"PlugS", false, false}, "SHPLG-U1": {"Plug US", false, false},
	"SHRGBW2": {"Shelly RGBW2", false, false}, "SHTRV-01": {"TRV", false, false},
	"SHUNI-1": {"Shelly UNI", false, false},
}

// Gen2: by "app" (DevicesFactory.createG2). Pro4PM and ProDimmerx are split
// by "model" in gen2ByModel.
var gen2 = map[string]entry{
	"Plus1": {"Shelly +1", false, false}, "Plus1PM": {"Shelly +1PM", false, false},
	"Plus2PM": {"Shelly +2PM", false, false}, "PlusI4": {"Shelly +i4", false, false},
	"Plus1Mini": {"Shelly Mini 1", false, false}, "Plus1PMMini": {"Shelly Mini 1PM", false, false},
	"PlusPMMini": {"Shelly Mini PM", false, false}, "PlusPlugS": {"Plug +S", false, false},
	"PlusPlugUK": {"Plug +UK", false, false}, "PlusPlugIT": {"Plug +IT", false, false},
	"PlugUS": {"Plug +US", false, false}, "PlusWallDimmer": {"Wall Dimmer", false, false},
	"PlusRGBWPM": {"Shelly +RGBW", false, false}, "Plus10V": {"Shelly +Dimmer 0-10V", false, false},
	"BluGw": {"Shelly BLU Gateway", false, false}, "WallDisplay": {"Wall Display", false, false},
	"WallDisplayV2": {"Wall Display X2i", false, false}, "PlusUni": {"Shelly +UNI", false, false},
	"PlusHT": {"Shelly +H&T", true, false}, "PlusSmoke": {"Shelly Smoke", true, false},
	"Pro1PM": {"Shelly Pro 1PM", false, true}, "Pro1PMProAddon": {"Shelly Pro 1PM", false, true},
	"Pro1": {"Shelly Pro 1", false, true}, "Pro1ProAddon": {"Shelly Pro 1", false, true},
	"Pro2PM": {"Shelly Pro 2PM", false, true}, "Pro2PMProAddon": {"Shelly Pro 2PM", false, true},
	"Pro2": {"Shelly Pro 2", false, true}, "Pro2ProAddon": {"Shelly Pro 2", false, true},
	"Pro3":  {"Shelly Pro 3", false, true},
	"ProEM": {"Shelly Pro EM-50", false, true}, "ProEMProAddon": {"Shelly Pro EM-50", false, true},
	"Pro3EM": {"Shelly Pro 3EM", false, true}, "Pro3EMProAddon": {"Shelly Pro 3EM", false, true},
	"ProRGBWWPM": {"Shelly Pro RGBWW PM", false, true},
	"ProCB":      {"Shelly Pro 2CB", false, true}, // "based on an obsolete prototype" (FEATURE_PARITY O9)
}

// Gen2 apps whose class depends on "model".
var gen2ByModel = map[string]map[string]entry{
	"Pro4PM": {
		"SPSH-002PE16EU": {"Shelly Pro Dual Cover", false, true},
		"":               {"Shelly Pro 4PM", false, true},
	},
	"ProDimmerx": {
		"SPDM-002PE01EU": {"Shelly Pro Dimmer 2PM", false, true},
		"":               {"Shelly Pro Dimmer 1PM", false, true},
	},
	"ProDimmerxProAddon": {
		"SPDM-002PE01EU": {"Shelly Pro Dimmer 2PM", false, true},
		"":               {"Shelly Pro Dimmer 1PM", false, true},
	},
}

// Gen3: by "app" (DevicesFactory.createG3). XT1 is split by svc0.type.
var gen3 = map[string]entry{
	"S1G3": {"Shelly 1 G3", false, false}, "S1PMG3": {"Shelly 1PM G3", false, false},
	"S2PMG3": {"Shelly 2PM G3", false, false}, "S2PMG3Shutter": {"Shelly Shutter G3", false, false},
	"Dimmer0110VPMG3": {"Shelly Dimmer 0/1-10V G3", false, false}, "I4G3": {"Shelly i4 G3", false, false},
	"Mini1G3": {"Shelly Mini 1 G3", false, false}, "Mini1PMG3": {"Shelly Mini 1PM G3", false, false},
	"MiniPMG3": {"Shelly Mini PM G3", false, false}, "PlugSG3": {"Plug S G3", false, false},
	"PlugPMG3": {"Plug PM G3", false, false}, "PlugMG3": {"Plug M G3", false, false},
	"OutdoorPlugSG3": {"Outdoor Plug S G3", false, false}, "HTG3": {"Shelly H&T G3", true, false},
	"DimmerG3": {"Shelly Dimmer G3", false, false}, "S3EMG3": {"Shelly 3EM-63", false, false},
	"S1LG3": {"Shelly 1L G3", false, false}, "S2LG3": {"Shelly 2L G3", false, false},
	"EMG3": {"Shelly EM G3", false, false}, "BluGwG3": {"Shelly BLU Gateway G3", false, false},
	"DuoBulbG3": {"Shelly Duo bulb G3", false, false}, "RGBCCTBulbG3": {"Shelly RGB bulb G3", false, false},
	"Camera": {"Shelly Camera", false, false}, "XMOD1": {"Shelly X MOD1", false, false},
	"Ogemray25": {"Ogemray SW40", false, false},
}

var xt1BySvc0 = map[string]entry{
	"linkedgo-st1820-floor-thermostat": {"LinkedGo ST1820", false, false},
	"linkedgo-st-802-hvac":             {"LinkedGo ST802", false, false},
	"":                                 {"XT1", false, false},
}

// Gen4: by "model" (DevicesFactory.createG4).
var gen4 = map[string]entry{
	"S4SW-001X16EU": {"Shelly 1 G4", false, false}, "S4SW-001P16EU": {"Shelly 1PM G4", false, false},
	"S4SW-002P16EU": {"Shelly 2PM G4", false, false}, "S4SW-001X8EU": {"Shelly Mini 1 G4", false, false},
	"S4SW-001P8EU": {"Shelly Mini 1PM G4", false, false}, "S4EM-001PXCEU16": {"Shelly Mini EM G4", false, false},
	"S4DM-0A101WWL": {"Shelly Dimmer G4", false, false}, "S4DM-0010WW": {"Shelly Dimmer 0/1-10V G4", false, false},
	"S4PL-00416EU": {"Shelly Power Strip G4", false, false}, "S4EM-002CXCEU": {"Shelly EM G4", false, false},
	"S4SW-0A1X1EUL": {"Shelly 1L G4", false, false}, "S4SW-0A2X4EUL": {"Shelly 2L G4", false, false},
	"S4SN-0U61X": {"Presence G4", false, false},
	"S4SN-0071Z": {"Shelly Flood S G4", true, false}, "S4SN-0071A": {"Shelly Flood G4", true, false},
}

// Lookup identifies a device from its /shelly answer.
func Lookup(info shelly.Info) Model {
	var (
		id string
		e  entry
		ok bool
	)
	switch info.Gen {
	case 0:
		id = info.Type
		e, ok = gen1[id]
	case 2:
		id = info.App
		if byModel, split := gen2ByModel[id]; split {
			if e, ok = byModel[info.Model]; !ok {
				e, ok = byModel[""]
			}
		} else {
			e, ok = gen2[id]
		}
	case 3:
		id = info.App
		if id == "XT1" {
			if e, ok = xt1BySvc0[info.Svc0Type]; !ok {
				e, ok = xt1BySvc0[""]
			}
		} else {
			e, ok = gen3[id]
		}
	case 4:
		id = info.Model
		e, ok = gen4[id]
	}
	if !ok {
		return Model{TypeID: id, TypeName: GenericName(info.Generation()), Known: false}
	}
	return Model{TypeID: id, TypeName: e.name, Battery: e.battery, Pro: e.pro, Known: true}
}

// GenericName is the type name of an unmanaged device of generation gen
// (Shelly{G1..G4}Unmanaged, ShellyGenericUnmanagedImpl).
func GenericName(gen string) string {
	switch gen {
	case "1", "2", "3", "4":
		return "Generic G" + gen
	}
	return "Generic"
}

// BLUTypeName maps a BTHome model_id (attrs.model_id) to ShellyScanner's type
// name (blu/BTHomeDevice constructor).
func BLUTypeName(modelID int) string {
	switch modelID {
	case 0x01:
		return "Blu Button"
	case 0x02:
		return "Blu Door Window"
	case 0x03:
		return "Blu H&T"
	case 0x05:
		return "Blu Motion"
	case 0x06:
		return "Blu Wall Switch 4"
	case 0x07:
		return "Blu RC Button 4"
	case 0x08:
		return "Blu TRV"
	case 0x09:
		return "Blu Remote"
	case 0x0A:
		return "Blu Distance"
	case 0x0B:
		return "Weather Station"
	case 0x0C:
		return "Blu H&T Display ZB"
	case 0x11:
		return "Blu H&T ZB"
	case 0x13:
		return "Blu Motion ZB"
	case 0x14:
		return "Blu Door Window ZB"
	case 0x15:
		return "Blu Wall Switch 4 ZB"
	case 0x16:
		return "Blu RC Button 4 ZB"
	case 0x17:
		return "Blu Button Tough 1 ZB"
	case 0x20:
		return "Blu 1"
	case 0x21:
		return "Blu 2"
	case 0x203A:
		return "Blu 3"
	}
	return "Generic BTHome"
}
