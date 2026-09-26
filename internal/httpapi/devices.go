package httpapi

import (
	"errors"
	"net/http"

	"github.com/wimmme/shellylanman/internal/discovery"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// deviceRoutes registers the device API. Without a Devices service (tests of
// other endpoints) they answer 503.
func (s *server) deviceRoutes(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if s.Devices == nil {
				writeError(w, http.StatusServiceUnavailable, "device service not running")
				return
			}
			fn(w, r)
		}
	}
	mux.HandleFunc("GET /api/v1/devices", h(s.listDevices))
	mux.HandleFunc("GET /api/v1/devices/{id}", h(s.getDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{id}", h(s.removeDevice))
	mux.HandleFunc("POST /api/v1/devices/refresh", h(s.refreshDevices))
	mux.HandleFunc("POST /api/v1/devices/{id}/reload", h(s.reloadDevice))
	mux.HandleFunc("PUT /api/v1/devices/{id}/credentials", h(s.putDeviceCredentials))
	mux.HandleFunc("GET /api/v1/credentials", h(s.getCredentials))
	mux.HandleFunc("PUT /api/v1/credentials", h(s.putCredentials))
	mux.HandleFunc("GET /api/v1/scan", h(s.getScan))
	mux.HandleFunc("POST /api/v1/scan", h(s.rescan))
	mux.HandleFunc("DELETE /api/v1/archive", h(s.clearArchive))
	mux.HandleFunc("GET /api/v1/network/interfaces", s.interfaces)
}

func (s *server) listDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Devices.List())
}

func (s *server) getDevice(w http.ResponseWriter, r *http.Request) {
	d, ok := s.Devices.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, service.ErrNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) removeDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.Devices.Remove(r.PathValue("id")); err != nil {
		code := http.StatusConflict
		if errors.Is(err, service.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeError(w, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) refreshDevices(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &body) {
		return
	}
	s.Devices.Refresh(body.IDs)
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) reloadDevice(w http.ResponseWriter, r *http.Request) {
	if !s.Devices.Reload(r.PathValue("id")) {
		writeError(w, http.StatusNotFound, service.ErrNotFound.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type credentialsBody struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

func (s *server) putDeviceCredentials(w http.ResponseWriter, r *http.Request) {
	var b credentialsBody
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Devices.SetDeviceCredentials(r.PathValue("id"), shelly.Credentials{User: b.User, Password: b.Password}); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, service.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeError(w, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Passwords are write-only: GET tells whether they are set, never what they are.
func (s *server) getCredentials(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Devices.Credentials())
}

func (s *server) putCredentials(w http.ResponseWriter, r *http.Request) {
	var b credentialsBody
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Devices.SetGlobalCredentials(shelly.Credentials{User: b.User, Password: b.Password}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Devices.Credentials())
}

func (s *server) getScan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Devices.ScanState())
}

func (s *server) rescan(w http.ResponseWriter, r *http.Request) {
	go s.Devices.Rescan()
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) clearArchive(w http.ResponseWriter, r *http.Request) {
	if err := s.Devices.ClearArchive(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) interfaces(w http.ResponseWriter, r *http.Request) {
	ifs, err := discovery.Interfaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ifs)
}
