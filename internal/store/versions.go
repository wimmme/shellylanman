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

// Every JSON file under /data carries a version (roadmap §4, DECISIONS §31). A build that finds a
// newer one than it understands does not start, instead of misreading the file and later writing
// over it: an older image started on newer data stops with a message naming the file. Only the
// sessions are an exception: they are dropped (everyone logs in again).
//
// A file that cannot be read at all (broken JSON, a version that is no number) is moved aside as
// <name>.broken-<unix time> and the loader starts empty, so the next save does not overwrite what
// the user may still want back (DECISIONS P22-3). settings.json is not moved: a broken one stops
// the start, as before.

// The versions this build writes and understands.
const (
	ProfilesVersion = 1
	DeferredVersion = 1
	ScenesVersion   = 1
	SessionsVersion = 1
)

// ScenesFile holds the scenes (DECISIONS P11-3).
const ScenesFile = "scenes.json"

// versioned is every file Open checks besides settings.json: its name, the field that holds the
// version and the highest version this build understands.
var versioned = []struct {
	name, field string
	max         int
}{
	{archiveFile, "ver", archiveVersion},
	{DeferredFile, "version", DeferredVersion},
	{ScenesFile, "version", ScenesVersion},
	{ProfilesFile, "version", ProfilesVersion},
}

var (
	// ErrNewerFile: a file was written by a newer ShellyLanMan.
	ErrNewerFile = errors.New("written by a newer ShellyLanMan")
	// ErrUnreadable: a file could not be read and was moved aside; the loader starts empty.
	ErrUnreadable = errors.New("unreadable")
)

func newerFile(name string, have, max int) error {
	return fmt.Errorf("%s is version %d, this build understands up to %d (%w)", name, have, max, ErrNewerFile)
}

// versionOf reads the version field of a JSON object; a file without one (written before the
// field existed) is version 0.
func versionOf(b []byte, field string) (int, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, err
	}
	v := 0
	if raw, ok := m[field]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return 0, fmt.Errorf("%s: %w", field, err)
		}
	}
	return v, nil
}

// checkVersions refuses a data directory with a file newer than this build. A file that cannot be
// parsed is left to its loader, which moves it aside.
func (s *Store) checkVersions() error {
	for _, f := range versioned {
		b, err := os.ReadFile(filepath.Join(s.dir, f.name))
		if err != nil {
			continue
		}
		if v, err := versionOf(b, f.field); err == nil && v > f.max {
			return newerFile(f.name, v, f.max)
		}
	}
	return nil
}

// LoadVersioned reads name, which has a "version" field, into v: false when the file is not there,
// ErrNewerFile for a newer version (v untouched, the file stays), ErrUnreadable when it could not be
// parsed (moved aside, v untouched).
func (s *Store) LoadVersioned(name string, max int, v any) (bool, error) {
	return s.loadVersioned(name, "version", max, v)
}

func (s *Store) loadVersioned(name, field string, max int, v any) (bool, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	have, err := versionOf(b, field)
	if err == nil && have > max {
		return false, newerFile(name, have, max)
	}
	if err == nil {
		err = json.Unmarshal(b, v)
	}
	if err != nil {
		return false, s.quarantine(name, err)
	}
	return true, nil
}

// quarantine moves a file that could not be parsed aside and says where to.
func (s *Store) quarantine(name string, cause error) error {
	to := fmt.Sprintf("%s.broken-%d", name, time.Now().Unix())
	s.mu.Lock()
	err := os.Rename(filepath.Join(s.dir, name), filepath.Join(s.dir, to))
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("parse %s: %v; moving it aside failed: %w", name, cause, err)
	}
	return fmt.Errorf("%s could not be read (%v), moved aside to %s: %w", name, cause, to, ErrUnreadable)
}
