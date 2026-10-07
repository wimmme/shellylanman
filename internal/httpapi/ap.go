package httpapi

import (
	"errors"
	"net/http"

	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/service"
)

// apRoutes: what the access-point wizards need (DECISIONS §29).
func (s *server) apRoutes(mux router, h func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET /api/v1/ap/guide", h(s.apGuide))
	mux.HandleFunc("GET /api/v1/ap/models", s.apModels)
	mux.HandleFunc("POST /api/v1/firmware/local", h(s.localFirmwareModel))
}

// apGuide: the access point of a device (?id=) or of an access point name (?name=).
func (s *server) apGuide(w http.ResponseWriter, r *http.Request) {
	g, err := s.Devices.APGuide(r.Context(), r.URL.Query().Get("id"), r.URL.Query().Get("name"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, g)
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrBadCommand):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// apModels: the models the user can pick when an access point's name does not say.
func (s *server) apModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ModelChoices())
}

// localFirmwareModel: the download link and QR code of a model, for a device that is not in the list.
func (s *server) localFirmwareModel(w http.ResponseWriter, r *http.Request) {
	var body modelBody
	if !readJSON(w, r, &body) {
		return
	}
	link, err := s.Devices.LocalDownloadModel(r.Context(), body.Gen, body.Key, s.phoneBase(r))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, link)
	case errors.Is(err, service.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, firmware.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default: // index or network problem
		writeError(w, http.StatusBadGateway, err.Error())
	}
}
