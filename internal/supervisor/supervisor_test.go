package supervisor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromEnvOnlyWithToken(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "")
	if FromEnv() != nil {
		t.Fatal("client without SUPERVISOR_TOKEN")
	}
	t.Setenv("SUPERVISOR_TOKEN", "abc")
	if c := FromEnv(); c == nil || c.Token != "abc" || c.Base != "http://supervisor" {
		t.Fatalf("client %+v", c)
	}
}

func TestAnnounce(t *testing.T) {
	var got struct {
		Service string         `json:"service"`
		Config  map[string]any `json:"config"`
	}
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/discovery" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"result":"ok","data":{"uuid":"x"}}`))
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Token: "tok", HTTP: srv.Client()}
	if err := c.Announce(context.Background(), ServiceMCP, map[string]any{"url": "http://127.0.0.1:8097/mcp"}); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer tok" || got.Service != "mcp" || got.Config["url"] != "http://127.0.0.1:8097/mcp" {
		t.Fatalf("sent %q %+v", auth, got)
	}
	c.Base = srv.URL + "/nothing"
	if err := c.Announce(context.Background(), ServiceShellyLanMan, nil); err == nil {
		t.Fatal("error status not reported")
	}
}
