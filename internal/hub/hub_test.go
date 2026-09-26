package hub

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

func dial(t *testing.T, srv *httptest.Server, origin string) (*websocket.Conn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{}
	if origin != "" {
		hdr.Set("Origin", origin)
	}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), &websocket.DialOptions{HTTPHeader: hdr})
	return c, err
}

func read(t *testing.T, c *websocket.Conn) Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ev Event
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHelloAndBroadcast(t *testing.T) {
	h := New(nil, func() Event { return Event{Type: "hello", Data: map[string]string{"version": "test"}} }, nil)
	counts := make(chan int, 4)
	h.OnClientsChanged(func(n int) { counts <- n })
	srv := httptest.NewServer(h)
	defer srv.Close()

	c, err := dial(t, srv, "")
	if err != nil {
		t.Fatal(err)
	}
	if ev := read(t, c); ev.Type != "hello" {
		t.Fatalf("first event = %q, want hello", ev.Type)
	}
	if n := <-counts; n != 1 {
		t.Fatalf("client count = %d", n)
	}

	h.Broadcast(Event{Type: "device.updated", Data: 42})
	if ev := read(t, c); ev.Type != "device.updated" {
		t.Fatalf("event = %q", ev.Type)
	}

	c.Close(websocket.StatusNormalClosure, "")
	if n := <-counts; n != 0 {
		t.Fatalf("client count after close = %d", n)
	}
}

func TestCrossOriginRefused(t *testing.T) {
	h := New(nil, nil, nil)
	srv := httptest.NewServer(h)
	defer srv.Close()
	if _, err := dial(t, srv, "http://evil.example"); err == nil {
		t.Fatal("cross-origin websocket accepted")
	}
	if h.Clients() != 0 {
		t.Fatal("refused client was registered")
	}
}

func TestAllowedOriginPattern(t *testing.T) {
	h := New([]string{"shelly.example.net"}, nil, nil)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c, err := dial(t, srv, "https://shelly.example.net")
	if err != nil {
		t.Fatalf("configured origin refused: %v", err)
	}
	defer c.CloseNow()
	waitFor(t, func() bool { return h.Clients() == 1 })
}

func TestSlowClientIsDropped(t *testing.T) {
	h := New(nil, nil, nil)
	c := &client{send: make(chan []byte, 1)}
	h.add(c)
	h.Broadcast(Event{Type: "a"})
	h.Broadcast(Event{Type: "b"}) // buffer full → dropped
	if h.Clients() != 0 {
		t.Fatal("slow client not dropped")
	}
}
