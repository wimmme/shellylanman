// Package store owns everything ShellyLanMan writes under its data directory
// (/data in the container). See ARCHITECTURE.md §2.6 for the full list.
//
// Files handled here:
//
//	secret.key     32 random bytes, created on first start (mode 0600)
//	settings.json  application settings; secret values encrypted with AES-256-GCM
//
// The key lives next to the data it protects. That protects a copy of
// settings.json on its own (a support request, a partial backup), not someone
// who has the whole volume. SECURITY.md says so.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/wimmme/shellylanman/internal/discovery"
)

const (
	keyFile      = "secret.key"
	settingsFile = "settings.json"
	keySize      = 32
	fileVersion  = 1
)

// Languages the UI ships with.
var Languages = []string{"en", "nl"}

// Settings are the application settings visible to the UI. Secrets are kept
// apart and never leave the store in clear text except through Secret.
type Settings struct {
	FirstRunDone bool            `json:"firstRunDone"`
	Language     string          `json:"language"` // default UI language for new browsers
	Scan         ScanSettings    `json:"scan"`
	Archive      ArchiveSettings `json:"archive"`
	// MQTTSlow is an extra pause, in tenths of a second, after each device
	// when MQTT settings are applied to several devices (ShellyScanner's
	// "-slow" command-line option, MQTT_SLOW).
	MQTTSlow int `json:"mqttSlow"`
}

// Scan modes (ShellyScanner: setting SCAN_MODE, dialog "Network scan mode").
const (
	ScanFull    = "full"    // mDNS on all interfaces ("Full mDNS scan")
	ScanLocal   = "local"   // mDNS on one chosen interface ("Local mDNS scan")
	ScanIP      = "ip"      // IP ranges ("IP scan")
	ScanOffline = "offline" // archive only ("Offline")
)

// ScanSettings are ShellyScanner's Network tab.
type ScanSettings struct {
	Mode      string            `json:"mode"`
	Interface string            `json:"interface,omitempty"` // for ScanLocal
	Ranges    []discovery.Range `json:"ranges,omitempty"`    // for ScanIP, at most discovery.MaxRanges
	// RefreshSeconds is the status refresh interval while a browser is
	// connected (REFRESH_INTERVAL, default 2); ConfigTics is how many status
	// refreshes run per configuration refresh (REFRESH_SETTINGS, default 5).
	RefreshSeconds int `json:"refreshSeconds"`
	ConfigTics     int `json:"configTics"`
}

// ArchiveSettings are ShellyScanner's Archive tab (USE_ARCHIVE, AUTORELOAD).
type ArchiveSettings struct {
	Use        bool `json:"use"`
	AutoReload bool `json:"autoReload"`
}

// Defaults are the settings of a fresh installation (ScannerProperties defaults).
func Defaults() Settings {
	return Settings{
		Language: "en",
		Scan:     ScanSettings{Mode: ScanFull, RefreshSeconds: 2, ConfigTics: 5},
		Archive:  ArchiveSettings{Use: true, AutoReload: true},
	}
}

// Validate reports whether the settings can be stored.
func (s Settings) Validate() error {
	if !slices.Contains(Languages, s.Language) {
		return fmt.Errorf("unsupported language %q", s.Language)
	}
	switch s.Scan.Mode {
	case ScanFull, ScanOffline:
	case ScanLocal:
		if s.Scan.Interface == "" {
			return errors.New("local scan needs a network interface")
		}
	case ScanIP:
		if len(s.Scan.Ranges) == 0 {
			return errors.New("IP scan needs at least one range")
		}
	default:
		return fmt.Errorf("unknown scan mode %q", s.Scan.Mode)
	}
	if len(s.Scan.Ranges) > discovery.MaxRanges {
		return fmt.Errorf("at most %d IP ranges", discovery.MaxRanges)
	}
	for _, r := range s.Scan.Ranges {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	if s.Scan.RefreshSeconds < 1 || s.Scan.RefreshSeconds > 3600 {
		return errors.New("status refresh must be 1–3600 seconds")
	}
	if s.MQTTSlow < 0 || s.MQTTSlow > 600 {
		return errors.New("MQTT delay must be 0–600 tenths of a second")
	}
	if s.Scan.ConfigTics < 1 || s.Scan.ConfigTics > 1000 {
		return errors.New("configuration refresh must be 1–1000 status refreshes")
	}
	return nil
}

// fileFormat is settings.json on disk.
type fileFormat struct {
	Version int `json:"version"`
	Settings
	// Secrets maps a name to base64(nonce || ciphertext). The name is used as
	// additional authenticated data, so a value cannot be moved to another name.
	Secrets map[string]string `json:"secrets,omitempty"`
}

// Store is safe for concurrent use.
type Store struct {
	dir  string
	aead cipher.AEAD

	mu   sync.Mutex
	data fileFormat
}

// Open prepares dir, creating the key and default settings on first start.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("data directory: %w", err)
	}
	key, err := loadOrCreateKey(filepath.Join(dir, keyFile))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, aead: aead}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir is the data directory.
func (s *Store) Dir() string { return s.dir }

func loadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(key) != keySize {
			return nil, fmt.Errorf("%s has %d bytes, want %d; refusing to guess", path, len(key), keySize)
		}
		return key, nil
	case errors.Is(err, fs.ErrNotExist):
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		// O_EXCL: never overwrite a key another process just wrote.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create key: %w", err)
		}
		if _, err := f.Write(key); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		return key, nil
	default:
		return nil, fmt.Errorf("read key: %w", err)
	}
}

func (s *Store) load() error {
	s.data = fileFormat{Version: fileVersion, Settings: Defaults()}
	b, err := os.ReadFile(filepath.Join(s.dir, settingsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return fmt.Errorf("parse %s: %w", settingsFile, err)
	}
	if s.data.Version > fileVersion {
		return fmt.Errorf("%s is version %d, this build understands up to %d", settingsFile, s.data.Version, fileVersion)
	}
	s.data.Version = fileVersion
	return nil
}

// Settings returns a copy of the current settings.
func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings.clone()
}

func (s Settings) clone() Settings {
	s.Scan.Ranges = slices.Clone(s.Scan.Ranges)
	return s
}

// Update changes the settings through fn and saves them if they validate.
func (s *Store) Update(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data.Settings.clone()
	fn(&next)
	if err := next.Validate(); err != nil {
		return s.data.Settings, err
	}
	prev := s.data.Settings
	s.data.Settings = next
	if err := s.save(); err != nil {
		s.data.Settings = prev
		return prev, err
	}
	return next, nil
}

// SetSecret stores value encrypted under name. An empty value deletes it.
func (s *Store) SetSecret(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.data.Secrets[name]
	if value == "" {
		delete(s.data.Secrets, name)
	} else {
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		sealed := s.aead.Seal(nonce, nonce, []byte(value), []byte(name))
		if s.data.Secrets == nil {
			s.data.Secrets = map[string]string{}
		}
		s.data.Secrets[name] = base64.StdEncoding.EncodeToString(sealed)
	}
	if err := s.save(); err != nil {
		if had {
			s.data.Secrets[name] = prev
		} else {
			delete(s.data.Secrets, name)
		}
		return err
	}
	return nil
}

// Secret returns the decrypted value stored under name.
func (s *Store) Secret(name string) (string, bool, error) {
	s.mu.Lock()
	enc, ok := s.data.Secrets[name]
	s.mu.Unlock()
	if !ok {
		return "", false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", true, fmt.Errorf("secret %q: %w", name, err)
	}
	n := s.aead.NonceSize()
	if len(raw) < n {
		return "", true, fmt.Errorf("secret %q: too short", name)
	}
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], []byte(name))
	if err != nil {
		return "", true, fmt.Errorf("secret %q: cannot decrypt (wrong key?)", name)
	}
	return string(plain), true, nil
}

// save writes settings.json atomically. Callers hold s.mu.
func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.dir, settingsFile), append(b, '\n'), 0o600)
}

// writeFileAtomic writes to a temporary file in the same directory and renames
// it over path, so a crash never leaves a half-written file behind.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}
