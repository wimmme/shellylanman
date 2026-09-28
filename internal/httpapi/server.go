package httpapi

import (
	"errors"
	"net/http"

	"github.com/wimmme/shellylanman/internal/listen"
	"github.com/wimmme/shellylanman/internal/service"
)

// Listener is the web server's own socket (package listen).
type Listener interface {
	Info() listen.Info
	Move(port int) error
}

func (s *server) serverRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/server", s.getServer)
	mux.HandleFunc("PUT /api/v1/server", s.putServer)
}

func (s *server) getServer(w http.ResponseWriter, r *http.Request) {
	if s.Listener == nil {
		writeJSON(w, http.StatusOK, listen.Info{Fixed: true})
		return
	}
	writeJSON(w, http.StatusOK, s.Listener.Info())
}

// putServer moves the web server to another port. The old port keeps working
// for a few seconds, so this response still arrives; the browser then goes to
// the new port. It needs confirm, like other actions that cut the UI off.
func (s *server) putServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port    int  `json:"port"`
		Confirm bool `json:"confirm"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusPreconditionRequired, service.ErrConfirm.Error())
		return
	}
	if s.Listener == nil {
		writeError(w, http.StatusConflict, listen.ErrFixed.Error())
		return
	}
	if err := s.Listener.Move(body.Port); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, listen.ErrFixed) {
			code = http.StatusConflict
		}
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Listener.Info())
}
