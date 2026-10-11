package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/store"
)

// A deferred.json that cannot be read is moved aside and the queue starts empty; the next save
// writes a new file next to it (P22-3). One that could not even be read is never written over.
func TestDeferredFileUnreadable(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Dir(), store.DeferredFile)
	if err := os.WriteFile(path, []byte(`{"version": 1, "tasks": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewDevices(st, shelly.NewClient(), func(string, any) {}, nil)
	m.loadDeferred()
	if len(m.Deferred()) != 0 || m.deferred.keep {
		t.Fatalf("tasks %v keep %v", m.Deferred(), m.deferred.keep)
	}
	if moved, _ := filepath.Glob(path + ".broken-*"); len(moved) != 1 {
		t.Fatalf("moved aside: %v", moved)
	}
	m.saveDeferred()
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), `"version": 1`) {
		t.Fatalf("new file: %s %v", b, err)
	}

	// Not readable at all (here: a directory in its place): kept, never written over.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	m = NewDevices(st, shelly.NewClient(), func(string, any) {}, nil)
	m.loadDeferred()
	if !m.deferred.keep {
		t.Fatal("an unreadable deferred.json must not be written over")
	}
}

// Scenes and profiles go on empty after their file was moved aside.
func TestScenesAndProfilesUnreadable(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{store.ScenesFile, store.ProfilesFile} {
		if err := os.WriteFile(filepath.Join(st.Dir(), name), []byte(`{`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := NewDevices(st, shelly.NewClient(), func(string, any) {}, nil)
	if list, err := m.Profiles(); err != nil || len(list) != 0 {
		t.Fatalf("profiles %v %v", list, err)
	}
	if list, err := m.Scenes(); err != nil || len(list) != 0 {
		t.Fatalf("scenes %v %v", list, err)
	}
	for _, name := range []string{store.ScenesFile, store.ProfilesFile} {
		if moved, _ := filepath.Glob(filepath.Join(st.Dir(), name+".broken-*")); len(moved) != 1 {
			t.Fatalf("%s moved aside: %v", name, moved)
		}
	}
}
