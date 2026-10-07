package store

// Profiles (DECISIONS §29, docs/phase-20-ap-wizards.md): what a new Shelly gets when it
// has joined the network, set up once in ShellyLanMan and used for every new device.
// The passwords of a profile are not in this file: they are secrets of the store.

// ProfilesFile holds the profiles.
const ProfilesFile = "profiles.json"

// Profile is a set of settings. A field that is absent (nil, empty) is left alone on the device.
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NamePattern string `json:"namePattern,omitempty"` // the device's name: {model}, {mac}, {mac4}, {gen}, {host}
	// WiFiSSID is a reminder the wizard shows when the user enters the home network on the device.
	// The password of that network is never kept (DECISIONS P20-6).
	WiFiSSID string        `json:"wifiSSID,omitempty"`
	Login    *ProfileLogin `json:"login,omitempty"`
	MQTT     *ProfileMQTT  `json:"mqtt,omitempty"`
	NTP      string        `json:"ntp,omitempty"` // the time server
	Cloud    *bool         `json:"cloud,omitempty"`
	// The checklist's settings, as its columns show them.
	Eco     *bool  `json:"eco,omitempty"`
	LEDOff  *bool  `json:"ledOff,omitempty"` // Gen1: the status LED is off
	AP      *bool  `json:"ap,omitempty"`     // Gen2+: the device's own access point is on
	Roaming *bool  `json:"roaming,omitempty"`
	AutoFW  string `json:"autoFW,omitempty"` // Gen2+: stable, beta or none
}

// ProfileLogin is the login the device gets; its password is a secret.
type ProfileLogin struct {
	Enabled bool   `json:"enabled"`
	User    string `json:"user,omitempty"` // Gen1; Gen2+ always use "admin"
}

// ProfileMQTT is the MQTT setting the device gets; its password is a secret.
type ProfileMQTT struct {
	Enabled    bool   `json:"enabled"`
	Server     string `json:"server,omitempty"`
	User       string `json:"user,omitempty"`
	NoPassword bool   `json:"noPassword,omitempty"`
}

type profilesFile struct {
	Profiles []Profile `json:"profiles"`
}

// LoadProfiles reads the profiles (none when the file is not there).
func (s *Store) LoadProfiles() ([]Profile, error) {
	var f profilesFile
	if _, err := s.LoadJSON(ProfilesFile, &f); err != nil {
		return nil, err
	}
	return f.Profiles, nil
}

// SaveProfiles writes the profiles.
func (s *Store) SaveProfiles(list []Profile) error {
	return s.SaveJSON(ProfilesFile, profilesFile{Profiles: list})
}
