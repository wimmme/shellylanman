package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/wimmme/shellylanman/internal/mcp"
	"github.com/wimmme/shellylanman/internal/store"
	"github.com/wimmme/shellylanman/internal/version"
)

// mcpRoutes: the MCP endpoint itself (/mcp, bearer token, package mcp) and the
// settings API the UI uses to switch it on and make a token.
func (s *server) mcpRoutes(mux *http.ServeMux) {
	srv := &mcp.Server{Config: s.mcpConfig, Version: version.Version, Log: s.Log}
	if s.Devices != nil { // a nil *service.Devices must not become a non-nil interface
		srv.Service = s.Devices
	}
	endpoint := func(w http.ResponseWriter, r *http.Request) {
		if srv.Service == nil {
			writeError(w, http.StatusServiceUnavailable, "device service not running")
			return
		}
		srv.ServeHTTP(w, r)
	}
	// Per method: a bare "/mcp" would conflict with "GET /" (the SPA).
	for _, m := range []string{"POST", "GET", "DELETE"} {
		mux.HandleFunc(m+" /mcp", endpoint)
	}
	mux.HandleFunc("GET /api/v1/mcp", s.getMCP)
	mux.HandleFunc("PUT /api/v1/mcp", s.putMCP)
	mux.HandleFunc("POST /api/v1/mcp/token", s.newMCPToken)
}

func (s *server) mcpConfig() mcp.Config {
	st := s.Store.Settings().MCP
	token, _, _ := s.Store.Secret(store.MCPTokenSecret)
	access := st.Access
	if access == "" {
		access = mcp.AccessRead
	}
	return mcp.Config{Enabled: st.Enabled, Access: access, Token: token}
}

// MCPInfo is what the settings page shows; the token itself only once, when made.
type MCPInfo struct {
	Enabled  bool   `json:"enabled"`
	Access   string `json:"access"`
	HasToken bool   `json:"hasToken"`
	Token    string `json:"token,omitempty"`
}

func (s *server) mcpInfo() MCPInfo {
	c := s.mcpConfig()
	return MCPInfo{Enabled: c.Enabled, Access: c.Access, HasToken: c.Token != ""}
}

func (s *server) getMCP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mcpInfo())
}

func (s *server) putMCP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool   `json:"enabled"`
		Access  *string `json:"access"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	_, err := s.Store.Update(func(st *store.Settings) {
		if body.Enabled != nil {
			st.MCP.Enabled = *body.Enabled
		}
		if body.Access != nil {
			st.MCP.Access = *body.Access
		}
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Switching on without a token would refuse every client: make one.
	info := s.mcpInfo()
	if info.Enabled && !info.HasToken {
		tok, err := s.makeMCPToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		info.HasToken, info.Token = true, tok
	}
	writeJSON(w, http.StatusOK, info)
}

// newMCPToken replaces the token; the old one stops working at once.
func (s *server) newMCPToken(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !readJSON(w, r, &body) { // like every state-changing call: JSON only (CSRF)
		return
	}
	tok, err := s.makeMCPToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	info := s.mcpInfo()
	info.Token = tok
	writeJSON(w, http.StatusOK, info)
}

func (s *server) makeMCPToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := "slm_" + base64.RawURLEncoding.EncodeToString(b)
	return tok, s.Store.SetSecret(store.MCPTokenSecret, tok)
}
