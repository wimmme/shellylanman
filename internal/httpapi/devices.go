package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/wimmme/shellylanman/internal/discovery"
	"github.com/wimmme/shellylanman/internal/mcp"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// deviceRoutes registers the device API. Without a Devices service (tests of
// other endpoints) they answer 503.
func (s *server) deviceRoutes(mux router) {
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
	// BLU identification (DECISIONS P14-4): the gateways that can run it, and the run.
	mux.HandleFunc("GET /api/v1/blu/gateways", h(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Devices.BLUGateways())
	}))
	mux.HandleFunc("POST /api/v1/blu/identify", h(func(w http.ResponseWriter, r *http.Request) {
		var body identifyBLUBody
		if !readJSON(w, r, &body) {
			return
		}
		switch err := s.Devices.IdentifyBLU(body.Gateway, body.Duration); {
		case errors.Is(err, service.ErrIdentifyBusy):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case err != nil:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	mux.HandleFunc("GET /api/v1/devices/{id}/credentials", h(func(w http.ResponseWriter, r *http.Request) {
		// The LAN port has no login: device passwords only with the MCP token
		// at access level "configure" (DECISIONS P13-3).
		c := s.mcpConfig()
		if !c.Enabled || c.Access != mcp.AccessConfigure || !mcp.Authorized(r, c.Token) {
			writeError(w, http.StatusForbidden, "device credentials need the MCP token with access level configure")
			return
		}
		s.writeCredentials(w, r)
	}))
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
	mux.HandleFunc("GET /api/v1/devices/{id}/info", h(s.infoRequests))
	mux.HandleFunc("GET /api/v1/devices/{id}/info/{index}", h(s.infoResult))
	mux.HandleFunc("GET /api/v1/devices/{id}/log", h(s.logSnapshot))
	mux.HandleFunc("PUT /api/v1/devices/{id}/pause", h(s.pauseDevice))
	mux.HandleFunc("PUT /api/v1/devices/{id}/note", h(s.putNote))
	mux.HandleFunc("GET /ws/log/{id}", h(s.logStream))
	mux.HandleFunc("POST /api/v1/devices/{id}/command", h(s.command))
	mux.HandleFunc("POST /api/v1/devices/reboot", h(s.reboot))
	s.configRoutes(mux, h)
	s.backupRoutes(mux, h)
	s.firmwareRoutes(mux, h)
	s.apRoutes(mux, h)
	s.scriptRoutes(mux, h)
	s.scheduleRoutes(mux, h)
}

// command runs one action of the Command column (service.Command).
func (s *server) command(w http.ResponseWriter, r *http.Request) {
	var cmd service.Command
	if !readJSON(w, r, &cmd) {
		return
	}
	if err := s.Devices.Command(r.Context(), r.PathValue("id"), cmd); err != nil {
		code := deviceErrorCode(err)
		if errors.Is(err, service.ErrConfirm) {
			code = http.StatusPreconditionRequired
		}
		writeError(w, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// reboot restarts devices; destructive, so it needs confirm=true.
func (s *server) reboot(w http.ResponseWriter, r *http.Request) {
	var body idsConfirmBody
	if !readJSON(w, r, &body) {
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusPreconditionRequired, service.ErrConfirm.Error())
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no devices")
		return
	}
	if err := s.Devices.Reboot(body.IDs); err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// listDevices: all devices, or of one generation with ?gen= ("1".."4", "blu",
// "bth"), the API form of ShellyScanner's "-gen <n>" (CLIController.getFilter).
func (s *server) listDevices(w http.ResponseWriter, r *http.Request) {
	list := s.Devices.List()
	if gen := r.URL.Query().Get("gen"); gen != "" {
		out := list[:0:0]
		for _, d := range list {
			if d.Gen == gen {
				out = append(out, d)
			}
		}
		list = out
	}
	writeJSON(w, http.StatusOK, list)
}

// writeCredentials answers with the user and password ShellyLanMan uses for the
// device; callers have checked that the request may have them.
func (s *server) writeCredentials(w http.ResponseWriter, r *http.Request) {
	c, err := s.Devices.DeviceCredentials(r.PathValue("id"))
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrNoCredentials):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, c)
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
	var body idsBody
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

func (s *server) infoRequests(w http.ResponseWriter, r *http.Request) {
	reqs, err := s.Devices.InfoRequests(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, reqs)
}

func (s *server) infoResult(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "index must be a number")
		return
	}
	res, err := s.Devices.Info(r.Context(), r.PathValue("id"), idx)
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// deviceErrorCode maps service and device errors to HTTP status codes.
func deviceErrorCode(err error) int {
	var api *shelly.APIError
	switch {
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, shelly.ErrUnauthorized):
		return http.StatusForbidden
	case shelly.IsOffline(err), errors.Is(err, service.ErrNoConnection):
		return http.StatusGatewayTimeout
	case errors.As(err, &api):
		return http.StatusBadGateway
	}
	return http.StatusBadRequest
}

func (s *server) logSnapshot(w http.ResponseWriter, r *http.Request) {
	file, _ := strconv.Atoi(r.URL.Query().Get("file"))
	text, err := s.Devices.LogSnapshot(r.Context(), r.PathValue("id"), file)
	if err != nil {
		writeError(w, deviceErrorCode(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, text)
}

func (s *server) pauseDevice(w http.ResponseWriter, r *http.Request) {
	var b pauseBody
	if !readJSON(w, r, &b) {
		return
	}
	if err := s.Devices.SetPaused(r.PathValue("id"), b.Paused); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// logStream relays a Gen2+ device's live log to the browser. The browser only
// listens; closing the page closes the device connection.
func (s *server) logStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Devices.Get(id); !ok {
		writeError(w, http.StatusNotFound, service.ErrNotFound.Error())
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.Origins})
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := c.CloseRead(r.Context())
	err = s.Devices.LogStream(ctx, id, func(line json.RawMessage) {
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = c.Write(wctx, websocket.MessageText, line)
		cancel()
	})
	if err != nil {
		msg, _ := json.Marshal(map[string]string{"error": err.Error()})
		_ = c.Write(ctx, websocket.MessageText, msg)
		c.Close(websocket.StatusInternalError, "device log unavailable")
		return
	}
	c.Close(websocket.StatusNormalClosure, "")
}

func (s *server) putNote(w http.ResponseWriter, r *http.Request) {
	var body noteBody
	if !readJSON(w, r, &body) {
		return
	}
	err := s.Devices.SetNote(r.PathValue("id"), body.Note, body.Keyword)
	if err == nil && body.Name != nil {
		err = s.Devices.SetBLUName(r.PathValue("id"), *body.Name)
	}
	if err != nil {
		code := deviceErrorCode(err)
		if errors.Is(err, service.ErrNoArchive) {
			code = http.StatusConflict
		}
		writeError(w, code, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
