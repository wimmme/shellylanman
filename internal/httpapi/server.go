package httpapi

import "net/http"

// Ports tells the settings page where ShellyLanMan listens and where that is
// set (DECISIONS P17-1): the page shows it, it does not change it.
type Ports struct {
	Source   string `json:"source"`             // package listen: default, env (SHELLYLANMAN_PORT), app
	App      bool   `json:"app"`                // running as the Home Assistant app
	Ingress  string `json:"ingress,omitempty"`  // the app's ingress listener (chosen by the Supervisor)
	MCPLocal string `json:"mcpLocal,omitempty"` // the app's token-less loopback listener
}

// ServerInfo is GET /api/v1/server.
type ServerInfo struct {
	Port int `json:"port"`
	Ports
}

func (s *server) serverRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/server", s.getServer)
}

func (s *server) getServer(w http.ResponseWriter, r *http.Request) {
	info := ServerInfo{Ports: s.Ports}
	if s.Port != nil {
		info.Port = s.Port()
	}
	if info.Source == "" {
		info.Source = "default"
	}
	writeJSON(w, http.StatusOK, info)
}
