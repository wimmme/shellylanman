package service

import (
	"encoding/json"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Credentials: a global default plus optional per-device credentials, all
// encrypted in the store (DECISIONS Q9). ShellyScanner keeps one "last used"
// set (DevicesFactory.setCredential) and prompts when a protected device is
// found; a server cannot prompt, so protected devices show as "not logged"
// until credentials are set (FEATURE_PARITY D18).

const (
	secretGlobal    = "cred:global"
	secretDevicePfx = "cred:device:"
)

// CredentialsInfo tells the UI what is configured, never the password.
type CredentialsInfo struct {
	GlobalSet  bool   `json:"globalSet"`
	GlobalUser string `json:"globalUser"`
}

func (m *Devices) readCred(name string) *shelly.Credentials {
	v, ok, err := m.store.Secret(name)
	if err != nil || !ok {
		return nil
	}
	var c shelly.Credentials
	if json.Unmarshal([]byte(v), &c) != nil || c.Password == "" {
		return nil
	}
	return &c
}

// credentialsFor returns the device's own credentials, else the global ones.
func (m *Devices) credentialsFor(id string) *shelly.Credentials {
	if c := m.readCred(secretDevicePfx + id); c != nil {
		return c
	}
	return m.readCred(secretGlobal)
}

// Credentials reports whether global credentials are set.
func (m *Devices) Credentials() CredentialsInfo {
	c := m.readCred(secretGlobal)
	if c == nil {
		return CredentialsInfo{}
	}
	return CredentialsInfo{GlobalSet: true, GlobalUser: c.User}
}

func (m *Devices) writeCred(name string, c shelly.Credentials) error {
	if c.Password == "" {
		return m.store.SetSecret(name, "")
	}
	b, _ := json.Marshal(c)
	return m.store.SetSecret(name, string(b))
}

// SetGlobalCredentials stores (or with an empty password clears) the default
// credentials and reloads every device that is not logged in.
func (m *Devices) SetGlobalCredentials(c shelly.Credentials) error {
	if err := m.writeCred(secretGlobal, c); err != nil {
		return err
	}
	for _, d := range m.List() {
		if d.Status == model.StatusLogin {
			m.Reload(d.ID)
		}
	}
	return nil
}

// SetDeviceCredentials stores credentials for one device and reloads it.
func (m *Devices) SetDeviceCredentials(id string, c shelly.Credentials) error {
	if _, ok := m.Get(id); !ok {
		return ErrNotFound
	}
	if err := m.writeCred(secretDevicePfx+id, c); err != nil {
		return err
	}
	m.Reload(id)
	return nil
}
