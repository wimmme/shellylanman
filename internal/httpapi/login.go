package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/auth"
	"github.com/wimmme/shellylanman/internal/mcp"
	"github.com/wimmme/shellylanman/internal/store"
)

// The optional UI password (DECISIONS §24). Off while no password is set.
// With one: the UI, /api/v1 and /ws need a session cookie, except under Home
// Assistant ingress (P15-2); a valid MCP token also opens /api/v1 (P15-3);
// /healthz, /mcp (its own token), /api/v1/status and the login stay open.

// SessionsFile holds the sessions (hashed ids) so a restart keeps them.
const SessionsFile = "sessions.json"

const cookieName = "slm_session"

type guard struct {
	sessions *auth.Sessions
	limiter  *auth.Limiter
}

// sessionsFile is sessions.json on disk. Before 0.9.7 the file was the bare map.
type sessionsFile struct {
	Version  int                     `json:"version"`
	Sessions map[string]auth.Session `json:"sessions"`
}

// loadSessions reads the saved sessions. A file from a newer build is not misread: its
// sessions are dropped and everyone logs in again (as with a file that cannot be read).
func loadSessions(st *store.Store) (map[string]auth.Session, error) {
	var raw map[string]json.RawMessage
	if _, err := st.LoadVersioned(SessionsFile, store.SessionsVersion, &raw); err != nil || raw == nil {
		return nil, err
	}
	sessions, ok := raw["sessions"]
	if _, versioned := raw["version"]; !versioned { // the bare map of 0.9.6 and earlier: hashed id → session
		b, _ := json.Marshal(raw)
		sessions, ok = b, true
	}
	var m map[string]auth.Session
	if ok {
		if err := json.Unmarshal(sessions, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", SessionsFile, err)
		}
	}
	return m, nil
}

func saveSessions(st *store.Store, m map[string]auth.Session) error {
	return st.SaveJSON(SessionsFile, sessionsFile{Version: store.SessionsVersion, Sessions: m})
}

func newGuard(st *store.Store, log *slog.Logger) *guard {
	saved, err := loadSessions(st)
	if err != nil {
		log.Warn("sessions dropped", "err", err)
	}
	g := &guard{sessions: auth.NewSessions(saved), limiter: auth.NewLimiter()}
	g.sessions.Save = func(m map[string]auth.Session) {
		if err := saveSessions(st, m); err != nil {
			log.Warn("sessions", "err", err)
		}
	}
	return g
}

// ResetPassword removes the password and every session (SHELLYLANMAN_RESET_PASSWORD, P15-5).
func ResetPassword(st *store.Store) error {
	if err := st.SetSecret(auth.PasswordSecret, ""); err != nil {
		return err
	}
	return saveSessions(st, map[string]auth.Session{})
}

func (s *server) loginRoutes(mux router) {
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("PUT /api/v1/auth/password", s.setPassword)
}

func (s *server) passwordHash() string {
	h, _, err := s.Store.Secret(auth.PasswordSecret)
	if err != nil {
		s.Log.Error("UI password", "err", err)
		return "!" // cannot be read: nobody gets in rather than everybody
	}
	return h
}

func (s *server) authEnabled() bool { return s.passwordHash() != "" }

// loggedIn: the request carries a live session. A "Stay logged in" cookie is
// renewed when the UI starts (status), so it lasts 30 days since the last use.
func (s *server) loggedIn(w http.ResponseWriter, r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	ok, remember := s.guard.sessions.Valid(c.Value)
	if ok && remember && r.URL.Path == "/api/v1/status" {
		setSessionCookie(w, r, c.Value, true)
	}
	return ok
}

// tokenAllows: the MCP token opens /api/v1 for programs such as the Home
// Assistant integration; a read-only token only for reading.
func (s *server) tokenAllows(r *http.Request) bool {
	c := s.mcpConfig()
	if !c.Enabled || !mcp.Authorized(r, c.Token) {
		return false
	}
	return r.Method == http.MethodGet || c.Access != mcp.AccessRead
}

// requireLogin guards everything behind the password.
func (s *server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		api := strings.HasPrefix(p, "/api/")
		switch {
		case viaIngress(r), p == "/healthz", p == "/mcp", p == "/api/v1/status",
			p == "/api/v1/auth/login" && r.Method == http.MethodPost:
		case !api && p != "/ws" && !strings.HasPrefix(p, "/ws/"):
			// The page and its files (and the phones' firmware download): the app shows the login itself.
		case !s.authEnabled(), s.loggedIn(w, r), api && s.tokenAllows(r):
		default:
			writeError(w, http.StatusUnauthorized, "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, id string, remember bool) {
	c := &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isHTTPS(r)}
	if remember {
		c.MaxAge = int(auth.IdleLimit / time.Second)
	}
	http.SetCookie(w, c)
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: isHTTPS(r), MaxAge: -1})
}

func (s *server) tooMany(w http.ResponseWriter, wait time.Duration) {
	secs := int((wait + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeCoded(w, http.StatusTooManyRequests, apiError{Code: "too_many_tries", Message: "too many wrong passwords, wait", RetryAfter: secs})
}

// checkPassword verifies pw against the stored hash with the slowdown for
// wrong tries (P15-6). It answers the request itself when it returns false.
func (s *server) checkPassword(w http.ResponseWriter, r *http.Request, pw, hash string) bool {
	client := clientAddr(r)
	if wait := s.guard.limiter.Wait(client); wait > 0 {
		s.tooMany(w, wait)
		return false
	}
	if !auth.Verify(pw, hash) {
		n, wait := s.guard.limiter.Fail(client)
		s.Log.Warn("wrong UI password", "from", client, "tries", n)
		e := apiError{Code: "wrong_password", Message: auth.ErrWrong.Error()}
		if wait > 0 {
			e.RetryAfter = int((wait + time.Second - 1) / time.Second)
			w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfter))
		}
		writeCoded(w, http.StatusUnauthorized, e)
		return false
	}
	s.guard.limiter.Succeed(client)
	return true
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if !readJSON(w, r, &body) {
		return
	}
	hash := s.passwordHash()
	if hash == "" {
		writeCoded(w, http.StatusConflict, apiError{Code: "no_password", Message: "no password is set"})
		return
	}
	if !s.checkPassword(w, r, body.Password, hash) {
		return
	}
	id, err := s.guard.sessions.Create(body.Remember)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	setSessionCookie(w, r, id, body.Remember)
	s.Log.Info("logged in", "from", clientAddr(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.guard.sessions.End(c.Value)
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// setPassword sets, changes (current needed) or removes (empty password,
// current needed) the password. Under Home Assistant ingress the current
// password is not asked: the user is logged in to Home Assistant, and the app
// has no environment variable to reset a forgotten one (P15-9). Every session
// ends; the browser that made the change gets a new one, so it stays logged in.
func (s *server) setPassword(w http.ResponseWriter, r *http.Request) {
	var body passwordBody
	if !readJSON(w, r, &body) {
		return
	}
	if hash := s.passwordHash(); hash != "" && !viaIngress(r) && !s.checkPassword(w, r, body.Current, hash) {
		return
	}
	if body.Password == "" {
		if err := s.Store.SetSecret(auth.PasswordSecret, ""); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		s.guard.sessions.EndAll()
		clearSessionCookie(w, r)
		s.Log.Warn("UI password switched off", "from", clientAddr(r))
		writeJSON(w, http.StatusOK, map[string]bool{"authEnabled": false})
		return
	}
	if err := auth.CheckPolicy(body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	hash, err := auth.Hash(body.Password)
	if err == nil {
		err = s.Store.SetSecret(auth.PasswordSecret, hash)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.guard.sessions.EndAll()
	if !viaIngress(r) {
		if id, err := s.guard.sessions.Create(false); err == nil {
			setSessionCookie(w, r, id, false)
		}
	}
	s.Log.Info("UI password set", "from", clientAddr(r))
	writeJSON(w, http.StatusOK, map[string]bool{"authEnabled": true})
}
