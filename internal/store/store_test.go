package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenCreatesKeyAndDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(filepath.Join(dir, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != keySize {
		t.Fatalf("key has %d bytes", len(key))
	}
	if got := s.Settings(); !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("settings = %+v, want defaults", got)
	}
}

func TestKeyIsReusedAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.SetSecret("device:AABBCC000001", "p@ss"); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, ok, err := s2.Secret("device:AABBCC000001")
	if err != nil || !ok || v != "p@ss" {
		t.Fatalf("Secret = %q, %v, %v", v, ok, err)
	}
}

func TestWrongKeyLengthIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, keyFile), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("Open accepted a truncated key")
	}
}

func TestSettingsPersistAndValidate(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	if _, err := s.Update(func(st *Settings) { st.Language = "nl"; st.FirstRunDone = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(func(st *Settings) { st.Language = "xx" }); err == nil {
		t.Fatal("invalid language accepted")
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Settings(); got.Language != "nl" || !got.FirstRunDone {
		t.Fatalf("reloaded settings = %+v", got)
	}
}

func TestSecretsAreNotStoredInClear(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	if err := s.SetSecret("global", "very-secret-value"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, settingsFile))
	if strings.Contains(string(b), "very-secret-value") {
		t.Fatal("secret found in clear text in settings.json")
	}
}

func TestSecretBoundToItsName(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_ = s.SetSecret("a", "one")
	// Move the ciphertext to another name: decryption must fail.
	s.data.Secrets["b"] = s.data.Secrets["a"]
	if _, _, err := s.Secret("b"); err == nil {
		t.Fatal("ciphertext accepted under a different name")
	}
}

func TestEmptySecretDeletes(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.SetSecret("a", "one")
	_ = s.SetSecret("a", "")
	if _, ok, _ := s.Secret("a"); ok {
		t.Fatal("secret still present after delete")
	}
}

func TestNewerFileVersionIsRefused(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, settingsFile), []byte(`{"version":99}`), 0o600)
	if _, err := Open(dir); err == nil {
		t.Fatal("future settings version accepted")
	}
}

func TestInstanceIDStablePerDataDir(t *testing.T) {
	dir := t.TempDir()
	a, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.InstanceID()) != 16 || a.InstanceID() != b.InstanceID() || a.InstanceID() == c.InstanceID() {
		t.Fatalf("ids %q %q %q", a.InstanceID(), b.InstanceID(), c.InstanceID())
	}
}
