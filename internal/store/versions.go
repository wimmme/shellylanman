package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Every JSON file under /data carries a version (roadmap §4, DECISIONS §31). A build that finds a
// newer one than it understands does not start, instead of misreading the file and later writing
// over it: an older image started on newer data stops with a message naming the file. Only the
// sessions are an exception: they are dropped (everyone logs in again).

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

// ErrNewerFile: a file was written by a newer ShellyLanMan.
var ErrNewerFile = errors.New("written by a newer ShellyLanMan")

// CheckVersion is the error for a file of version have when this build understands up to max.
func CheckVersion(name string, have, max int) error {
	if have > max {
		return newerFile(name, have, max)
	}
	return nil
}

func newerFile(name string, have, max int) error {
	return fmt.Errorf("%s is version %d, this build understands up to %d (%w)", name, have, max, ErrNewerFile)
}

// fileVersionOf reads the version field of a JSON object; a file without one (written before
// the field existed) is version 0.
func fileVersionOf(b []byte, field string) (int, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, err
	}
	v := 0
	if raw, ok := m[field]; ok {
		if err := json.Unmarshal(raw, &v); err != nil {
			return 0, err
		}
	}
	return v, nil
}

// checkVersions refuses a data directory with a file newer than this build. A file that cannot be
// read or parsed is left to its own loader, which reports it as before.
func (s *Store) checkVersions() error {
	for _, f := range versioned {
		b, err := os.ReadFile(filepath.Join(s.dir, f.name))
		if err != nil {
			continue
		}
		if v, err := fileVersionOf(b, f.field); err == nil && v > f.max {
			return newerFile(f.name, v, f.max)
		}
	}
	return nil
}

// LoadVersioned is LoadJSON for a file with a "version" field: a version above max is an
// ErrNewerFile and leaves v untouched.
func (s *Store) LoadVersioned(name string, max int, v any) (bool, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	have, err := fileVersionOf(b, "version")
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	if have > max {
		return false, newerFile(name, have, max)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return true, nil
}
