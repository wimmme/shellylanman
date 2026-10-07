package httpapi

import (
	"net/http"
	"strconv"

	"github.com/wimmme/shellylanman/internal/logbuf"
)

// LogResponse is GET /api/v1/log: ShellyLanMan's own log (DECISIONS §27).
type LogResponse struct {
	Entries []logbuf.Entry `json:"entries"`
}

func (s *server) logRoutes(mux router) {
	mux.HandleFunc("GET /api/v1/log", s.getLog)
}

// getLog returns the entries after ?after=<seq> (all when absent), oldest first.
// New lines arrive on the WebSocket as "log.entry" events.
func (s *server) getLog(w http.ResponseWriter, r *http.Request) {
	var after uint64
	if v := r.URL.Query().Get("after"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "after must be a sequence number")
			return
		}
		after = n
	}
	res := LogResponse{Entries: []logbuf.Entry{}}
	if s.Logs != nil {
		res.Entries = s.Logs.Entries(after)
	}
	writeJSON(w, http.StatusOK, res)
}
