// Package httpapi is the HTTP adapter over the service layer: REST under
// /api/v1, events on /ws, /healthz, and the embedded frontend.
//
// Handlers stay thin. Anything a user can do belongs in the service layer, so
// a future MCP server or Home Assistant integration can call the same code
// (ARCHITECTURE.md §2.1).
package httpapi

import (
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
	"github.com/wimmme/shellylanman/internal/version"
)

const maxBody = 64 << 10

// Config wires the server.
type Config struct {
	Store   *store.Store
	Hub     *hub.Hub
	Devices *service.Devices
	// Static is the built frontend (index.html at its root).
	Static fs.FS
	// Origins are extra allowed Origin hosts for state-changing requests and
	// the WebSocket, e.g. a reverse proxy's public name.
	Origins []string
	Log     *slog.Logger
}

type server struct {
	Config
}

// New returns the complete HTTP handler.
func New(cfg Config) http.Handler {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	s := &server{Config: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/about", s.about)
	mux.HandleFunc("GET /api/v1/status", s.status)
	mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	mux.HandleFunc("PUT /api/v1/settings", s.putSettings)
	s.deviceRoutes(mux)
	mux.Handle("GET /ws", cfg.Hub)
	mux.HandleFunc("GET /", s.static)
	return securityHeaders(s.sameOrigin(mux))
}

// health is unauthenticated by design and says nothing about the version.
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// About is what the About page shows.
type About struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Commit   string   `json:"commit,omitempty"`
	License  string   `json:"license"`
	Source   string   `json:"source"`
	BasedOn  Credit   `json:"basedOn"`
	Credits  []Credit `json:"credits"`
	Notice   string   `json:"notice"`
	Language []string `json:"languages"`
}

// Credit names a project ShellyLanMan builds on.
type Credit struct {
	Name    string `json:"name"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	License string `json:"license"`
	What    string `json:"what"`
}

func (s *server) about(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, About{
		Name:    "ShellyLanMan",
		Version: version.Version,
		Commit:  version.Commit,
		License: "GPL-3.0-or-later",
		Source:  "https://github.com/wimmme/shellylanman",
		BasedOn: Credit{
			Name:    "ShellyScanner",
			Author:  "Antonio Flaccomio (usnasoft)",
			URL:     "https://github.com/usnasoft/shellyscanner",
			License: "GPL-3.0",
			What:    "functional and code reference",
		},
		Credits: []Credit{
			{Name: "MikroDash", Author: "MikroDash contributors", URL: "https://github.com/SecOps-7/MikroDash", License: "MIT", What: "design tokens, palettes and appearance settings"},
			{Name: "Bundled fonts", Author: "see OFL.txt", URL: "/fonts/OFL.txt", License: "OFL-1.1", What: "Inter, Oxanium, IBM Plex Sans, Nunito, Roboto, JetBrains Mono"},
		},
		Notice:   "ShellyLanMan is an independent project. It is not ShellyScanner and is not affiliated with or endorsed by Shelly Group. Shelly is a trademark of its owner.",
		Language: store.Languages,
	})
}

// Status tells the UI what it needs before anything else.
type Status struct {
	FirstRunDone bool `json:"firstRunDone"`
	AuthEnabled  bool `json:"authEnabled"`
	Clients      int  `json:"clients"`
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Status{
		FirstRunDone: s.Store.Settings().FirstRunDone,
		AuthEnabled:  false, // optional UI password: not built yet
		Clients:      s.Hub.Clients(),
	})
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
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Hub.Broadcast(hub.Event{Type: "settings.changed", Data: next})
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
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
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
