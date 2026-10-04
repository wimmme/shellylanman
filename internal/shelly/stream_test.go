package shelly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A protected device answers the call with 401 and a challenge; the call is
// repeated with the JSON-RPC auth object, then notifications arrive.
func TestNotificationsWithAuth(t *testing.T) {
	gotAuth := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		_, b, _ := c.Read(ctx)
		var req map[string]any
		_ = json.Unmarshal(b, &req)
		challenge, _ := json.Marshal(map[string]any{"auth_type": "digest", "nonce": 1700000000, "realm": "shellyplus1-x", "algorithm": "SHA-256"})
		reply, _ := json.Marshal(map[string]any{"id": req["id"], "error": map[string]any{"code": 401, "message": string(challenge)}})
		_ = c.Write(ctx, websocket.MessageText, reply)
		_, b, _ = c.Read(ctx)
		_ = json.Unmarshal(b, &req)
		auth, _ := req["auth"].(map[string]any)
		gotAuth <- auth
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"id":2,"result":null}`))
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"method":"NotifyEvent","params":{"events":[{"component":"bthome","event":"discovery_done"}]}}`))
		<-ctx.Done()
	}))
	defer srv.Close()

	conn := NewClient().Conn(strings.TrimPrefix(srv.URL, "http://"), false)
	conn.SetCredentials(&Credentials{Password: "s3cret"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var events []string
	err := conn.Notifications(ctx, "BTHome.StartDeviceDiscovery", map[string]any{"duration": 30}, func(method string, params json.RawMessage) {
		events = append(events, method+" "+string(params))
		cancel()
	})
	if err != nil {
		t.Fatal(err)
	}
	auth := <-gotAuth
	if auth["username"] != "admin" || auth["realm"] != "shellyplus1-x" || auth["response"] == "" {
		t.Fatalf("auth %v", auth)
	}
	if len(events) != 1 || !strings.Contains(events[0], "discovery_done") {
		t.Fatalf("events %v", events)
	}
}
