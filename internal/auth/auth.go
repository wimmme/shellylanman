// Package auth is the optional UI password (DECISIONS §24, P15-1..7): one
// password without a user name, stored as a PBKDF2 hash, browser sessions in a
// cookie, and a slowdown after wrong passwords. Standard library only.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// PasswordSecret names the password hash among the store's secrets.
const PasswordSecret = "ui.password"

// MinLength is the shortest password (P15-7).
const MinLength = 8

const (
	iterations = 600_000 // OWASP's PBKDF2-HMAC-SHA256 figure
	saltLen    = 16
	keyLen     = 32
	scheme     = "pbkdf2-sha256"
)

// Policy errors; the UI shows its own text for each.
var (
	ErrTooShort  = errors.New("the password needs at least 8 characters")
	ErrNoCapital = errors.New("the password needs at least one capital letter")
	ErrWrong     = errors.New("wrong password")
)

// CheckPolicy enforces P15-7: at least MinLength characters and a capital.
func CheckPolicy(pw string) error {
	if utf8.RuneCountInString(pw) < MinLength {
		return ErrTooShort
	}
	for _, r := range pw {
		if unicode.IsUpper(r) {
			return nil
		}
	}
	return ErrNoCapital
}

// Hash returns "pbkdf2-sha256$<iterations>$<salt>$<key>" (base64, no padding).
func Hash(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hashWith(pw, salt, iterations)
}

func hashWith(pw string, salt []byte, iter int) (string, error) {
	key, err := pbkdf2.Key(sha256.New, pw, salt, iter, keyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", scheme, iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify reports whether pw matches a hash made by Hash, in constant time.
func Verify(pw, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != scheme {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hashWith(pw, salt, iter)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(hash)) == 1
}

// ---- sessions ----

// IdleLimit ends a session that was not used for this long (P15-4).
const IdleLimit = 30 * 24 * time.Hour

// Session is one logged-in browser. Only the hash of its id is kept, so the
// file on disk cannot be used to log in.
type Session struct {
	LastUsed int64 `json:"lastUsed"` // unix seconds
	Remember bool  `json:"remember"` // "Stay logged in": a cookie that outlives the browser
}

// Sessions is safe for concurrent use. Save is called after every change
// that must survive a restart (nil: memory only).
type Sessions struct {
	mu   sync.Mutex
	m    map[string]Session // hashed id → session
	Now  func() time.Time
	Save func(map[string]Session)
}

// NewSessions starts from saved sessions (may be nil).
func NewSessions(saved map[string]Session) *Sessions {
	s := &Sessions{m: map[string]Session{}, Now: time.Now}
	for k, v := range saved {
		s.m[k] = v
	}
	return s
}

func hashID(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])
}

// Create starts a session and returns its id (for the cookie).
func (s *Sessions) Create(remember bool) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	s.m[hashID(id)] = Session{LastUsed: s.Now().Unix(), Remember: remember}
	s.pruneLocked()
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.save(snap)
	return id, nil
}

// Valid reports whether id is a live session and marks it used. The bool
// remember tells the caller whether to renew a persistent cookie.
func (s *Sessions) Valid(id string) (ok, remember bool) {
	if id == "" {
		return false, false
	}
	k := hashID(id)
	now := s.Now()
	s.mu.Lock()
	sess, found := s.m[k]
	if !found {
		s.mu.Unlock()
		return false, false
	}
	if now.Sub(time.Unix(sess.LastUsed, 0)) > IdleLimit {
		delete(s.m, k)
		snap := s.snapshotLocked()
		s.mu.Unlock()
		s.save(snap)
		return false, false
	}
	// Written to disk at most once an hour per session: enough for a 30-day limit.
	dirty := now.Unix()-sess.LastUsed > 3600
	sess.LastUsed = now.Unix()
	s.m[k] = sess
	var snap map[string]Session
	if dirty {
		snap = s.snapshotLocked()
	}
	s.mu.Unlock()
	if dirty {
		s.save(snap)
	}
	return true, sess.Remember
}

// End removes one session (log out).
func (s *Sessions) End(id string) {
	s.mu.Lock()
	delete(s.m, hashID(id))
	snap := s.snapshotLocked()
	s.mu.Unlock()
	s.save(snap)
}

// EndAll removes every session (password changed, switched off or reset).
func (s *Sessions) EndAll() {
	s.mu.Lock()
	s.m = map[string]Session{}
	s.mu.Unlock()
	s.save(map[string]Session{})
}

// Len is the number of live sessions (tests).
func (s *Sessions) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

func (s *Sessions) pruneLocked() {
	now := s.Now()
	for k, v := range s.m {
		if now.Sub(time.Unix(v.LastUsed, 0)) > IdleLimit {
			delete(s.m, k)
		}
	}
}

func (s *Sessions) snapshotLocked() map[string]Session {
	out := make(map[string]Session, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}

func (s *Sessions) save(snap map[string]Session) {
	if s.Save != nil {
		s.Save(snap)
	}
}

// ---- slowing down wrong passwords (P15-6) ----

// FreeTries wrong passwords per client address before the waits begin.
const FreeTries = 5

// MaxWait is the longest wait between two tries.
const MaxWait = 60 * time.Second

type attempt struct {
	fails int
	until time.Time
}

// Limiter is safe for concurrent use.
type Limiter struct {
	mu  sync.Mutex
	m   map[string]attempt
	Now func() time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter { return &Limiter{m: map[string]attempt{}, Now: time.Now} }

// Wait is how long client must still wait before the next try (0: may try).
func (l *Limiter) Wait(client string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if d := l.m[client].until.Sub(l.Now()); d > 0 {
		return d
	}
	return 0
}

// Fail records a wrong password and returns the number of wrong tries so far
// and the wait before the next one.
func (l *Limiter) Fail(client string) (int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	for k, a := range l.m { // forget addresses that have been quiet for a day
		if now.Sub(a.until) > 24*time.Hour {
			delete(l.m, k)
		}
	}
	a := l.m[client]
	a.fails++
	var wait time.Duration
	if a.fails >= FreeTries {
		wait = time.Second << min(a.fails-FreeTries, 6) // 1 s, 2 s, 4 s … 64 s
		wait = min(wait, MaxWait)
	}
	a.until = now.Add(wait)
	l.m[client] = a
	return a.fails, wait
}

// Succeed forgets the client's wrong tries.
func (l *Limiter) Succeed(client string) {
	l.mu.Lock()
	delete(l.m, client)
	l.mu.Unlock()
}
