package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/wimmme/shellylanman/internal/service"
)

// scriptRoutes: scripts and KVS of a Gen2+ device (Phase 8).
func (s *server) scriptRoutes(mux router, h func(http.HandlerFunc) http.HandlerFunc) {
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
	mux.HandleFunc("POST /api/v1/devices/{id}/scripts/log", h(s.scriptLogOn))
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
	var body scriptCreateBody
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
	var body scriptUpdateBody
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
	var body scriptCodeBody
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
	var body kvsBody
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

func (s *server) scriptLogOn(w http.ResponseWriter, r *http.Request) {
	if err := s.Devices.ScriptLogOn(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// backupScripts lists the scripts inside an uploaded .sbk (base64).
func (s *server) backupScripts(w http.ResponseWriter, r *http.Request) {
	var body uploadBody
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

// scheduleRoutes: the scheduler dialogs (Phase 8).
func (s *server) scheduleRoutes(mux router, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("POST /api/v1/devices/{id}/rpc", h(s.deviceRPC))
	mux.HandleFunc("GET /api/v1/devices/{id}/schedule/hints", h(s.scheduleHints))
	mux.HandleFunc("POST /api/v1/sbk/json", h(s.backupJSON))
	mux.HandleFunc("GET /api/v1/samples", h(s.getSamples))
	mux.HandleFunc("DELETE /api/v1/samples", h(s.clearSamples))
	mux.HandleFunc("GET /api/v1/devices/{id}/emdata", h(s.emData))
}

func (s *server) deviceRPC(w http.ResponseWriter, r *http.Request) {
	var body rpcBody
	if !readJSON(w, r, &body) {
		return
	}
	res, err := s.Devices.DeviceRPC(r.Context(), r.PathValue("id"), body.Method, body.Params)
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	if len(res) == 0 {
		res = json.RawMessage("null")
	}
	writeJSON(w, http.StatusOK, map[string]json.RawMessage{"result": res})
}

func (s *server) scheduleHints(w http.ResponseWriter, r *http.Request) {
	list, err := s.Devices.ScheduleHints(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) backupJSON(w http.ResponseWriter, r *http.Request) {
	var body uploadBody
	if !readBigJSON(w, r, &body) {
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Upload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload is not base64")
		return
	}
	files, err := service.BackupJSON(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *server) getSamples(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	writeJSON(w, http.StatusOK, s.Devices.Samples(idsParam(r), since))
}

func (s *server) clearSamples(w http.ResponseWriter, r *http.Request) {
	s.Devices.ClearSamples(idsParam(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) emData(w http.ResponseWriter, r *http.Request) {
	start, err1 := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
	end, err2 := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
	if err1 != nil || err2 != nil || start <= 0 || end <= start {
		writeError(w, http.StatusBadRequest, "start and end (unix seconds) needed")
		return
	}
	list, err := s.Devices.EMEnergy(r.Context(), r.PathValue("id"), start, end)
	if err != nil {
		writeError(w, scriptErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}
