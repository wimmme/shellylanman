package httpapi

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"

	"github.com/wimmme/shellylanman/internal/sbk"
	"github.com/wimmme/shellylanman/internal/service"
)

// maxRestoreBody: a restore request may carry an uploaded .sbk (base64).
const maxRestoreBody = 24 << 20

// backupRoutes: backup and restore (Phase 6).
func (s *server) backupRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("POST /api/v1/backup", h(s.backup))
	mux.HandleFunc("GET /api/v1/backups", h(s.listBackups))
	mux.HandleFunc("GET /api/v1/devices/{id}/backups/{name}", h(s.downloadBackup))
	mux.HandleFunc("POST /api/v1/devices/{id}/restore/check", h(s.restoreCheck))
	mux.HandleFunc("POST /api/v1/devices/{id}/restore", h(s.restore))
	mux.HandleFunc("POST /api/v1/restore/multi", h(s.restoreMulti))
}

// readBigJSON is readJSON with a larger limit.
func readBigJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRestoreBody))
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

func (s *server) backup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	lines, err := s.Devices.Backup(r.Context(), body.IDs)
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": lines})
}

func (s *server) listBackups(w http.ResponseWriter, r *http.Request) {
	list := s.Devices.Backups(r.URL.Query().Get("id"))
	if list == nil {
		list = []service.BackupFile{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	b, err := s.Devices.BackupData(r.PathValue("id"), name)
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

func restoreErrorCode(err error) int {
	if errors.Is(err, service.ErrInvalid) {
		return http.StatusBadRequest
	}
	return deviceErrorCode(err)
}

func (s *server) restoreCheck(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source service.RestoreSource `json:"source"`
	}
	if !readBigJSON(w, r, &body) {
		return
	}
	plan, err := s.Devices.RestoreCheck(r.Context(), r.PathValue("id"), body.Source)
	if err != nil {
		writeError(w, restoreErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source  service.RestoreSource `json:"source"`
		Answers sbk.Answers           `json:"answers"`
		Confirm bool                  `json:"confirm"`
	}
	if !readBigJSON(w, r, &body) {
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusPreconditionRequired, service.ErrConfirm.Error())
		return
	}
	if body.Answers == nil {
		body.Answers = sbk.Answers{}
	}
	res, err := s.Devices.Restore(r.Context(), r.PathValue("id"), body.Source, body.Answers)
	if err != nil {
		writeError(w, restoreErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) restoreMulti(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs     []string `json:"ids"`
		Confirm bool     `json:"confirm"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusPreconditionRequired, service.ErrConfirm.Error())
		return
	}
	lines, err := s.Devices.RestoreMulti(r.Context(), body.IDs)
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": lines})
}
