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
	mux.HandleFunc("GET /api/v1/ap/phone", h(s.apPhone))
}

// apPhone: the link (and QR code) that opens the firmware wizard on a phone.
func (s *server) apPhone(w http.ResponseWriter, r *http.Request) {
	base, guessed := s.phoneBase(r)
	l, err := service.WizardLink(base, r.URL.Query().Get("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if guessed && l.Warning == "" {
		l.Warning = "ingress"
	}
	writeJSON(w, http.StatusOK, l)
}

// apGuide: the access point of a device (?id=) or of an access point name (?name=).
func (s *server) apGuide(w http.ResponseWriter, r *http.Request) {
	g, err := s.Devices.APGuide(r.Context(), r.URL.Query().Get("id"), r.URL.Query().Get("name"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, g)
	case errors.Is(err, service.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	case errors.Is(err, service.ErrBadCommand):
		writeErr(w, http.StatusConflict, err)
	case errors.Is(err, service.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err)
	default:
		writeErr(w, http.StatusInternalServerError, err)
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
	base, guessed := s.phoneBase(r)
	link, err := s.Devices.LocalDownloadModel(r.Context(), body.Gen, body.Key, base)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, localLink(link, guessed))
	case errors.Is(err, service.ErrInvalid):
		writeErr(w, http.StatusBadRequest, err)
	case errors.Is(err, firmware.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	default: // index or network problem
		writeErr(w, http.StatusBadGateway, err)
	}
}
