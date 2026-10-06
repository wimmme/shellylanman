// Package httpapi is the HTTP adapter over the service layer: REST under
// /api/v1, events on /ws, /healthz, and the embedded frontend.
//
// Handlers stay thin. Anything a user can do belongs in the service layer, so
// a future MCP server or Home Assistant integration can call the same code
// (ARCHITECTURE.md §2.1).
package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/store"
	"github.com/wimmme/shellylanman/internal/update"
)

const maxBody = 64 << 10

// Config wires the server.
type Config struct {
	Store   *store.Store
	Hub     *hub.Hub
	Devices *service.Devices
	Updates *update.Checker // release check (nil in tests)
	// Port is the web UI's port, Ports where it and the app's other listeners
	// are set (shown on the settings page; nil and empty in most tests).
	Port  func() int
	Ports Ports
	// Static is the built frontend (index.html at its root).
	Static fs.FS
	// Origins are extra allowed Origin hosts for state-changing requests and
	// the WebSocket, e.g. a reverse proxy's public name.
	Origins []string
	Log     *slog.Logger
}

type server struct {
	Config
	guard *guard
}

// New returns the complete HTTP handler.
func New(cfg Config) http.Handler {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	s := &server{Config: cfg, guard: newGuard(cfg.Store, cfg.Log)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	s.loginRoutes(mux)
	s.aboutRoutes(mux)
	mux.HandleFunc("GET /api/v1/status", s.status)
	mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	mux.HandleFunc("PUT /api/v1/settings", s.putSettings)
	mux.HandleFunc("GET /api/v1/update", s.getUpdate)
	s.deviceRoutes(mux)
	s.serverRoutes(mux)
	s.mcpRoutes(mux)
	mux.Handle("GET /ws", cfg.Hub)
	mux.HandleFunc("GET /", s.static)
	return securityHeaders(s.sameOrigin(s.requireLogin(mux)))
}

// health is unauthenticated by design and says nothing about the version.
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Status tells the UI what it needs before anything else.
type Status struct {
	FirstRunDone bool `json:"firstRunDone"`
	AuthEnabled  bool `json:"authEnabled"` // a UI password is set (DECISIONS §24)
	LoggedIn     bool `json:"loggedIn"`    // this browser may use the UI (also: no password, or ingress)
	Clients      int  `json:"clients"`
	// Ingress: this request came through the Home Assistant ingress (the
	// user is logged in to Home Assistant; the page is under a path prefix).
	Ingress bool `json:"ingress"`
	// LocalURL: the Home Assistant app's token-less loopback listener, so the
	// integration next to the app finds it also when a password is set
	// (a loopback address: tells nothing to anyone else).
	LocalURL string `json:"localUrl,omitempty"`
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	enabled := s.authEnabled()
	st := Status{AuthEnabled: enabled, Ingress: viaIngress(r), LocalURL: localURL(s.Ports.MCPLocal)}
	st.LoggedIn = !enabled || st.Ingress || s.loggedIn(w, r) || s.tokenAllows(r)
	if st.LoggedIn { // nothing else for a browser that still has to log in
		st.FirstRunDone = s.Store.Settings().FirstRunDone
		st.Clients = s.Hub.Clients()
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Settings())
}

// putSettings accepts a partial update: only fields present in the body change.
func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var patch struct {
		FirstRunDone *bool                  `json:"firstRunDone"`
		Language     *string                `json:"language"`
		Scan         *store.ScanSettings    `json:"scan"`
		Archive      *store.ArchiveSettings `json:"archive"`
		MQTTSlow     *int                   `json:"mqttSlow"`
		BackupKeep   *int                   `json:"backupKeep"`
		PhoneBaseURL *string                `json:"phoneBaseURL"`
		UpdateCheck  *string                `json:"updateCheck"`
		SkipVersion  *string                `json:"skipVersion"`
	}
	if !readJSON(w, r, &patch) {
		return
	}
	next, err := s.Store.Update(func(st *store.Settings) {
		if patch.FirstRunDone != nil {
			st.FirstRunDone = *patch.FirstRunDone
		}
		if patch.Language != nil {
			st.Language = *patch.Language
		}
		if patch.Scan != nil {
			st.Scan = *patch.Scan
		}
		if patch.Archive != nil {
			st.Archive = *patch.Archive
		}
		if patch.MQTTSlow != nil {
			st.MQTTSlow = *patch.MQTTSlow
		}
		if patch.BackupKeep != nil {
			st.BackupKeep = *patch.BackupKeep
		}
		if patch.UpdateCheck != nil {
			st.UpdateCheck = *patch.UpdateCheck
		}
		if patch.SkipVersion != nil {
			st.SkipVersion = *patch.SkipVersion
		}
		if patch.PhoneBaseURL != nil {
			st.PhoneBaseURL = strings.TrimRight(strings.TrimSpace(*patch.PhoneBaseURL), "/")
		}
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Hub.Broadcast(hub.Event{Type: "settings.changed", Data: next})
	if s.Updates != nil {
		s.Updates.Wake() // the release check follows its setting at once
	}
	// ShellyScanner applies a new scan mode at the next start; a server applies
	// it at once with a rescan.
	if (patch.Scan != nil || patch.Archive != nil) && s.Devices != nil {
		go s.Devices.Rescan()
	}
	writeJSON(w, http.StatusOK, next)
}

// static serves the SPA: existing files as-is, every other path gets
// index.html so client-side routes survive a reload.
func (s *server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "no such endpoint")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if st, err := fs.Stat(s.Static, name); err != nil || st.IsDir() {
		name = "index.html"
	}
	b, err := fs.ReadFile(s.Static, name)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "ShellyLanMan: the frontend is not built into this binary. The API is available under /api/v1.\n")
		return
	}
	if name == "index.html" {
		nonce, _ := r.Context().Value(nonceKey{}).(string)
		b = bytes.ReplaceAll(b, []byte(noncePlaceholder), []byte(nonce))
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// Revalidate every file: after an upgrade the browser must not keep an old
	// app.js next to a new index.html.
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

// sameOrigin refuses state-changing requests from other origins (CSRF). A
// request without an Origin header is not from a browser page and is allowed.
func (s *server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if o := r.Header.Get("Origin"); o != "" && !s.originAllowed(o, r.Host) {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) originAllowed(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, host) {
		return true
	}
	for _, p := range s.Origins {
		if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(u.Host)); ok {
			return true
		}
	}
	return false
}

// nonceKey carries the request's CSP nonce to static(), which writes it into
// index.html.
type nonceKey struct{}

// noncePlaceholder in index.html is replaced by the request's nonce.
const noncePlaceholder = "__CSP_NONCE__"

// cspNonce: 16 random bytes, base64.
func cspNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Inline styles only with this response's nonce: the script editor
		// (CodeMirror) injects its stylesheet as a <style> element carrying it.
		// Everything else stays blocked; style attributes are set through the
		// CSSOM, which the policy does not restrict.
		nonce := cspNonce()
		r = r.WithContext(context.WithValue(r.Context(), nonceKey{}, nonce))
		// Home Assistant shows ingress apps in an iframe of its own (same) origin.
		frame, xfo := "'none'", "DENY"
		if viaIngress(r) {
			frame, xfo = "'self'", "SAMEORIGIN"
		}
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'nonce-"+nonce+"'; img-src 'self' data:; connect-src 'self'; frame-ancestors "+frame+"; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", xfo)
		next.ServeHTTP(w, r)
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// getUpdate: the last release check (never checked when the setting is off).
func (s *server) getUpdate(w http.ResponseWriter, r *http.Request) {
	if s.Updates == nil {
		writeJSON(w, http.StatusOK, update.Status{Mode: update.Never})
		return
	}
	writeJSON(w, http.StatusOK, s.Updates.Status())
}
