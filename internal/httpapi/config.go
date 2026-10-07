package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wimmme/shellylanman/internal/service"
)

// configRoutes: the settings dialog, the checklist and the deferred tasks (Phase 5).
func (s *server) configRoutes(mux router, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/config/{section}", h(s.getConfig))
	mux.HandleFunc("POST /api/v1/config/{section}", h(s.applyConfig))
	mux.HandleFunc("GET /api/v1/checklist", h(s.getChecklist))
	mux.HandleFunc("POST /api/v1/checklist/action", h(s.checklistAction))
	mux.HandleFunc("GET /api/v1/deferred", h(s.listDeferred))
	mux.HandleFunc("DELETE /api/v1/deferred/{id}", h(s.cancelDeferred))
}

func idsParam(r *http.Request) []string {
	var ids []string
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func configErrorCode(err error) int {
	switch {
	case errors.Is(err, service.ErrConfirm):
		return http.StatusPreconditionRequired
	case errors.Is(err, service.ErrAllExcluded):
		return http.StatusConflict
	case errors.Is(err, service.ErrInvalid):
		return http.StatusBadRequest
	}
	return deviceErrorCode(err)
}

func (s *server) getConfig(w http.ResponseWriter, r *http.Request) {
	ids := idsParam(r)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	f, err := s.Devices.ConfigForm(r.Context(), ids, r.PathValue("section"))
	if err != nil {
		writeError(w, configErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *server) applyConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var sel struct {
		IDs []string `json:"ids"`
	}
	if json.Unmarshal(body, &sel) != nil || len(sel.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	section := r.PathValue("section")
	var apply any
	var derr error
	switch section {
	case service.SectionWiFi1, service.SectionWiFi2:
		var a service.WiFiApply
		derr = json.Unmarshal(body, &a)
		apply = a
	case service.SectionLogin:
		var a service.LoginApply
		derr = json.Unmarshal(body, &a)
		apply = a
	case service.SectionMQTT:
		var a service.MQTTApply
		derr = json.Unmarshal(body, &a)
		a.Variant, a.Multi = "", false // decided by the service
		apply = a
	case service.SectionOthers:
		var a service.OthersApply
		derr = json.Unmarshal(body, &a)
		apply = a
	default:
		writeError(w, http.StatusNotFound, "unknown section")
		return
	}
	if derr != nil {
		writeError(w, http.StatusBadRequest, derr.Error())
		return
	}
	lines, err := s.Devices.ConfigApply(r.Context(), sel.IDs, section, apply)
	if err != nil {
		writeError(w, configErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": lines})
}

func (s *server) getChecklist(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Devices.Checklist(r.Context(), idsParam(r)))
}

func (s *server) checklistAction(w http.ResponseWriter, r *http.Request) {
	var a service.ChecklistAction
	if !readJSON(w, r, &a) {
		return
	}
	if len(a.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	lines, rows, err := s.Devices.ChecklistApply(r.Context(), a)
	if err != nil {
		writeError(w, configErrorCode(err), err.Error())
		return
	}
	if lines == nil {
		lines = []service.ResultLine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"errors": lines, "rows": rows})
}

func (s *server) listDeferred(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Devices.Deferred())
}

func (s *server) cancelDeferred(w http.ResponseWriter, r *http.Request) {
	if err := s.Devices.CancelDeferred(r.PathValue("id")); err != nil {
		code := http.StatusConflict
		if errors.Is(err, service.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeError(w, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
