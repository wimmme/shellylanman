package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An older build started on newer data stops with the file's name instead of misreading it.
func TestOpenRefusesNewerFiles(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{settingsFile, `{"version": 2}`},
		{archiveFile, `{"ver": 2, "dev": []}`},
		{DeferredFile, `{"version": 2, "tasks": []}`},
		{ScenesFile, `{"version": 2, "scenes": []}`},
		{ProfilesFile, `{"version": 2, "profiles": []}`},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, c.name), []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Open(dir)
		if !errors.Is(err, ErrNewerFile) || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s: err = %v, want ErrNewerFile naming the file", c.name, err)
		}
	}
}

// Files this build understands, older ones without the field and broken ones (their loader
// reports those) do not stop the start.
func TestOpenAcceptsKnownVersions(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		archiveFile:  `{"ver": 1, "dev": []}`,
		DeferredFile: `{"version": 1, "tasks": []}`,
		ScenesFile:   `{"scenes": []}`,
		ProfilesFile: `{"profiles": [{"id": "p1", "name": "Old"}]}`,
		"other.json": `not json`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.LoadProfiles()
	if err != nil || len(list) != 1 || list[0].Name != "Old" {
		t.Fatalf("profiles without a version: %v %v", list, err)
	}
	if err := s.SaveProfiles(list); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ProfilesFile))
	if !strings.Contains(string(b), `"version": 1`) {
		t.Fatalf("profiles.json is written without its version:\n%s", b)
	}
}

func TestLoadVersioned(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Version int    `json:"version"`
		X       string `json:"x"`
	}
	if ok, err := s.LoadVersioned("f.json", 1, &v); ok || err != nil {
		t.Fatalf("missing file: %v %v", ok, err)
	}
	if err := s.SaveJSON("f.json", map[string]any{"version": 3, "x": "new"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadVersioned("f.json", 2, &v); !errors.Is(err, ErrNewerFile) || v.X != "" {
		t.Fatalf("newer file: %v, v = %+v", err, v)
	}
	if ok, err := s.LoadVersioned("f.json", 3, &v); !ok || err != nil || v.X != "new" {
		t.Fatalf("known version: %v %v %+v", ok, err, v)
	}
}
