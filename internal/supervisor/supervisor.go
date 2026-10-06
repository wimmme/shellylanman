// Package supervisor announces ShellyLanMan to Home Assistant when it runs as a
// Home Assistant app (docs/phase-11-ha-mcp.md §6.3): the Supervisor passes a
// discovery message to Home Assistant Core, which then offers the matching
// integration ("shellylanman", "mcp") with one click.
//
// Only used when the Supervisor started the process: it sets SUPERVISOR_TOKEN.
package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Discovery services announced by the app; they must be listed under
// "discovery" in the app's config.yaml.
const (
	ServiceShellyLanMan = "shellylanman"
	ServiceMCP          = "mcp"
)

// Client talks to the Supervisor API.
type Client struct {
	Base  string // "http://supervisor"
	Token string
	HTTP  *http.Client
}

// FromEnv returns a client when running as a Home Assistant app, else nil.
func FromEnv() *Client {
	tok := os.Getenv("SUPERVISOR_TOKEN")
	if tok == "" {
		return nil
	}
	return &Client{Base: "http://supervisor", Token: tok, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Announce sends one discovery message. Home Assistant matches repeated
// messages of the same app and service, so announcing again (e.g. after a
// port change) updates the existing entry instead of adding one.
func (c *Client) Announce(ctx context.Context, service string, config map[string]any) error {
	body, err := json.Marshal(map[string]any{"service": service, "config": config})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/discovery", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery %s: HTTP %d", service, resp.StatusCode)
	}
	return nil
}

// IngressPort asks the Supervisor which port it chose for this app's ingress
// (config.yaml "ingress_port: 0": a free port, so it never clashes with another
// app on the host network).
func (c *Client) IngressPort(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/addons/self/info", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("app info: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			IngressPort int `json:"ingress_port"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("app info: %w", err)
	}
	if body.Data.IngressPort < 1 || body.Data.IngressPort > 65535 {
		return 0, fmt.Errorf("app info: no ingress port (%d)", body.Data.IngressPort)
	}
	return body.Data.IngressPort, nil
}
