package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"os"

	"github.com/wimmme/shellylanman/internal/service"
)

// firmwareRoutes: the FW Update panel (Phase 7).
func (s *server) firmwareRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/firmware", h(s.getFirmware))
	mux.HandleFunc("POST /api/v1/firmware/update", h(s.updateFirmware))
	mux.HandleFunc("GET /api/v1/firmware/index", h(s.firmwareIndex))
	mux.HandleFunc("POST /api/v1/firmware/{id}/local", h(s.localFirmware))
	// Reached by phones without a UI session: the token is the authorisation.
	mux.HandleFunc("GET /fw/{token}/{name}", h(s.firmwareFile))
}

func (s *server) firmwareIndex(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Devices.FirmwareIndex(r.Context(), idsParam(r))
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// phoneBase: the configured address for phones, else the one the browser used.
func (s *server) phoneBase(r *http.Request) string {
	if b := s.Store.Settings().PhoneBaseURL; b != "" {
		return b
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *server) localFirmware(w http.ResponseWriter, r *http.Request) {
	link, err := s.Devices.LocalDownload(r.Context(), r.PathValue("id"), s.phoneBase(r))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, link)
	case errors.Is(err, service.ErrUpToDate), errors.Is(err, service.ErrNoLocalFW):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default: // index or network problem
		writeError(w, http.StatusBadGateway, err.Error())
	}
}

func (s *server) firmwareFile(w http.ResponseWriter, r *http.Request) {
	path, name, err := s.Devices.FirmwareFile(r.Context(), r.PathValue("token"))
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, service.ErrLinkExpired) {
			code = http.StatusGone
		}
		http.Error(w, err.Error(), code)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

func (s *server) getFirmware(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Devices.Firmware(r.Context(), idsParam(r))
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	if rows == nil {
		rows = []service.FirmwareRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *server) updateFirmware(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items   []service.FirmwareRequest `json:"items"`
		Confirm bool                      `json:"confirm"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusPreconditionRequired, service.ErrConfirm.Error())
		return
	}
	lines, err := s.Devices.FirmwareUpdate(r.Context(), body.Items)
	if err != nil {
		writeError(w, configErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": lines})
}
