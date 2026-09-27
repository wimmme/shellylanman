package httpapi

import (
	"net/http"

	"github.com/wimmme/shellylanman/internal/service"
)

// firmwareRoutes: the FW Update panel (Phase 7).
func (s *server) firmwareRoutes(mux *http.ServeMux, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/firmware", h(s.getFirmware))
	mux.HandleFunc("POST /api/v1/firmware/update", h(s.updateFirmware))
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
