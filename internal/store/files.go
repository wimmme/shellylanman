package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DeferredFile holds the queued actions for offline devices (DECISIONS Q10).
const DeferredFile = "deferred.json"

// LoadJSON reads name from the data directory into v; a missing file leaves v
// untouched and returns false.
func (s *Store) LoadJSON(name string, v any) (bool, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return true, nil
}

// SaveJSON writes v to name in the data directory atomically (mode 0600).
func (s *Store) SaveJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeFileAtomic(filepath.Join(s.dir, name), append(b, '\n'), 0o600)
}

// Seal encrypts value for storage in another file; context binds it to its
// place (used as additional authenticated data), so it cannot be moved.
func (s *Store) Seal(context, value string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(value), []byte(context))), nil
}

// Unseal decrypts a value made by Seal with the same context.
func (s *Store) Unseal(context, sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	n := s.aead.NonceSize()
	if len(raw) < n {
		return "", errors.New("sealed value too short")
	}
	plain, err := s.aead.Open(nil, raw[:n], raw[n:], []byte(context))
	if err != nil {
		return "", errors.New("cannot decrypt (wrong key?)")
	}
	return string(plain), nil
}
