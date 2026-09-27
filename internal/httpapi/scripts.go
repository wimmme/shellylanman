package httpapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"

	"github.com/wimmme/shellylanman/internal/service"
)

// scriptRoutes: scripts and KVS of a Gen2+ device (Phase 8).
func (s *server) scriptRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/devices/{id}/scripts", h(s.getScripts))
	mux.HandleFunc("POST /api/v1/devices/{id}/scripts", h(s.createScript))
	mux.HandleFunc("PATCH /api/v1/devices/{id}/scripts/{sid}", h(s.updateScript))
	mux.HandleFunc("DELETE /api/v1/devices/{id}/scripts/{sid}", h(s.deleteScript))
	mux.HandleFunc("POST /api/v1/devices/{id}/scripts/{sid}/start", h(s.runScript(true)))
	mux.HandleFunc("POST /api/v1/devices/{id}/scripts/{sid}/stop", h(s.runScript(false)))
	mux.HandleFunc("GET /api/v1/devices/{id}/scripts/{sid}/code", h(s.getScriptCode))
	mux.HandleFunc("PUT /api/v1/devices/{id}/scripts/{sid}/code", h(s.putScriptCode))
	mux.HandleFunc("POST /api/v1/devices/{id}/kvs", h(s.setKVS))
	mux.HandleFunc("DELETE /api/v1/devices/{id}/kvs", h(s.deleteKVS))
	mux.HandleFunc("POST /api/v1/sbk/scripts", h(s.backupScripts))
}

func scriptErrorCode(err error) int {
	switch {
	case errors.Is(err, service.ErrNoScripts), errors.Is(err, service.ErrBadCommand):
		return http.StatusConflict
	case errors.Is(err, service.ErrInvalid):
		return http.StatusBadRequest
	}
	return deviceErrorCode(err)
}

func sid(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("sid"))
	if err != nil || n < 0 {
		writeError(w, http.StatusBadRequest, "bad script id")
		return 0, false
	}
	return n, true
}

func (s *server) getScripts(w http.ResponseWriter, r *http.Request) {
	v, err := s.Devices.Scripts(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) createScript(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &body) {
		return
	}
	sc, err := s.Devices.ScriptCreate(r.Context(), r.PathValue("id"), body.Name)
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (s *server) updateScript(w http.ResponseWriter, r *http.Request) {
	n, ok := sid(w, r)
	if !ok {
		return
	}
	var body struct {
		Name   *string `json:"name"`
		Enable *bool   `json:"enable"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.Devices.ScriptUpdate(r.Context(), r.PathValue("id"), n, body.Name, body.Enable); err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteScript(w http.ResponseWriter, r *http.Request) {
	n, ok := sid(w, r)
	if !ok {
		return
	}
	if err := s.Devices.ScriptDelete(r.Context(), r.PathValue("id"), n); err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) runScript(run bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, ok := sid(w, r)
		if !ok {
			return
		}
		withLog := r.URL.Query().Get("log") == "true"
		if err := s.Devices.ScriptRun(r.Context(), r.PathValue("id"), n, run, withLog); err != nil {
			writeError(w, scriptErrorCode(err), err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) getScriptCode(w http.ResponseWriter, r *http.Request) {
	n, ok := sid(w, r)
	if !ok {
		return
	}
	code, err := s.Devices.ScriptCode(r.Context(), r.PathValue("id"), n)
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (s *server) putScriptCode(w http.ResponseWriter, r *http.Request) {
	n, ok := sid(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !readBigJSON(w, r, &body) {
		return
	}
	if err := s.Devices.ScriptPutCode(r.Context(), r.PathValue("id"), n, body.Code); err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) setKVS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	it, err := s.Devices.KVSSet(r.Context(), r.PathValue("id"), body.Key, body.Value)
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *server) deleteKVS(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "no key")
		return
	}
	if err := s.Devices.KVSDelete(r.Context(), r.PathValue("id"), key); err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// backupScripts lists the scripts inside an uploaded .sbk (base64).
func (s *server) backupScripts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Upload string `json:"upload"`
	}
	if !readBigJSON(w, r, &body) {
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Upload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload is not base64")
		return
	}
	list, err := service.ScriptsInBackup(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}
