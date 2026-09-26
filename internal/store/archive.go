package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	archiveFile    = "archive.json"
	archiveVersion = 1
)

// ArchivedDevice is one device in the archive: the last known identity and
// address, plus the user's note and keyword. Field names follow ShellyScanner's
// ShellyStore.arc (model/DevicesStore.java) so the format is recognisable,
// but ShellyLanMan does not read or write .arc files (DECISIONS.md Q11).
type ArchivedDevice struct {
	TypeID   string `json:"tid"`
	TypeName string `json:"tn"`
	Host     string `json:"host"`
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Name     string `json:"name"`
	SSID     string `json:"ssid"`
	Last     int64  `json:"last"` // unix milliseconds
	Battery  bool   `json:"bat"`
	Gen      string `json:"gen"`
	Note     string `json:"note"`
	Keyword  string `json:"keyword"`
}

type archiveFileFormat struct {
	Version int              `json:"ver"`
	Time    int64            `json:"time"`
	Devices []ArchivedDevice `json:"dev"`
}

// LoadArchive returns the archived devices; a missing file is an empty archive.
func (s *Store) LoadArchive() ([]ArchivedDevice, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, archiveFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	var a archiveFileFormat
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("parse %s: %w", archiveFile, err)
	}
	if a.Version > archiveVersion {
		return nil, fmt.Errorf("%s is version %d, this build understands up to %d", archiveFile, a.Version, archiveVersion)
	}
	return a.Devices, nil
}

// SaveArchive replaces the archive atomically.
func (s *Store) SaveArchive(devs []ArchivedDevice) error {
	b, err := json.MarshalIndent(archiveFileFormat{Version: archiveVersion, Time: time.Now().UnixMilli(), Devices: devs}, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeFileAtomic(filepath.Join(s.dir, archiveFile), append(b, '\n'), 0o600)
}

// ClearArchive deletes the archive (Settings → Archive → Clear).
func (s *Store) ClearArchive() error {
	err := os.Remove(filepath.Join(s.dir, archiveFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
