package httpapi

import (
	"errors"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/wimmme/shellylanman/internal/service"
)

// firmwareRoutes: the FW Update panel (Phase 7).
func (s *server) firmwareRoutes(mux router, h func(http.HandlerFunc) http.HandlerFunc) {
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
		writeErr(w, deviceErrorCode(err), err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// phoneBase: the configured address for phones, else the one the browser used.
// Under Home Assistant ingress the browser used Home Assistant, which does not
// serve /fw/: then ShellyLanMan's own port on the same host (as Settings → MCP).
// guessed: that port on Home Assistant's address may not reach ShellyLanMan
// (Home Assistant behind a proxy or a name): the page then shows a tip.
func (s *server) phoneBase(r *http.Request) (base string, guessed bool) {
	if b := s.Store.Settings().PhoneBaseURL; b != "" {
		return b, false
	}
	if viaIngress(r) && s.Port != nil {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(s.Port())), true
	}
	// baseURL: https also behind a TLS proxy; an http link from an https page is an
	// insecure download that browsers block, a PWA without a word.
	return baseURL(r), false
}

// localLink adds the ingress tip to a link whose address phoneBase guessed.
func localLink(link *service.LocalLink, guessed bool) *service.LocalLink {
	if guessed && link.Warning == "" {
		link.Warning = "ingress"
	}
	return link
}

func (s *server) localFirmware(w http.ResponseWriter, r *http.Request) {
	base, guessed := s.phoneBase(r)
	link, err := s.Devices.LocalDownload(r.Context(), r.PathValue("id"), base)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, localLink(link, guessed))
	case errors.Is(err, service.ErrUpToDate), errors.Is(err, service.ErrNoLocalFW):
		writeErr(w, http.StatusConflict, err)
	case errors.Is(err, service.ErrNotFound):
		writeErr(w, http.StatusNotFound, err)
	default: // index or network problem
		writeErr(w, http.StatusBadGateway, err)
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
		writeErr(w, deviceErrorCode(err), err)
		return
	}
	if rows == nil {
		rows = []service.FirmwareRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *server) updateFirmware(w http.ResponseWriter, r *http.Request) {
	var body firmwareUpdateBody
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	if !body.Confirm {
		writeErr(w, http.StatusPreconditionRequired, service.ErrConfirm)
		return
	}
	lines, err := s.Devices.FirmwareUpdate(r.Context(), body.Items)
	if err != nil {
		writeErr(w, configErrorCode(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": lines})
}
