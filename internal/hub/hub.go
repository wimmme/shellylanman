// Package hub fans server events out to connected browsers over WebSocket.
//
// The socket is one-way: server → browser. Browsers act through the REST API,
// so the API stays equally usable by curl, an MCP adapter or Home Assistant.
// The number of connected browsers is observable because the poller runs at
// full rate only while someone is watching (DECISIONS.md §8, Q8).
package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Event is one message to the browsers.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

const (
	sendBuffer   = 64
	writeTimeout = 10 * time.Second
	pingInterval = 30 * time.Second
)

// Hub is safe for concurrent use.
type Hub struct {
	originPatterns []string
	hello          func() Event
	log            *slog.Logger

	mu       sync.Mutex
	clients  map[*client]struct{}
	onChange func(n int)
}

type client struct {
	send chan []byte
}

// New creates a hub. originPatterns are extra allowed Origin hosts (the
// request's own host is always allowed). hello, if set, is sent first to each
// new client.
func New(originPatterns []string, hello func() Event, log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		originPatterns: originPatterns,
		hello:          hello,
		log:            log,
		clients:        map[*client]struct{}{},
	}
}

// OnClientsChanged registers fn to be called with the new count whenever a
// browser connects or disconnects.
func (h *Hub) OnClientsChanged(fn func(n int)) {
	h.mu.Lock()
	h.onChange = fn
	h.mu.Unlock()
}

// Clients is the number of connected browsers.
func (h *Hub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Broadcast queues ev for every client. A client whose buffer is full is
// disconnected rather than slowing everyone else down; it reconnects and
// reloads its state.
func (h *Hub) Broadcast(ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		h.log.Error("marshal event", "type", ev.Type, "err", err)
		return
	}
	h.mu.Lock()
	dropped := false
	for c := range h.clients {
		select {
		case c.send <- b:
		default:
			dropped = h.removeLocked(c) || dropped
		}
	}
	n, fn := len(h.clients), h.onChange
	h.mu.Unlock()
	if dropped && fn != nil {
		fn(n)
	}
}

func (h *Hub) add(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	n, fn := len(h.clients), h.onChange
	h.mu.Unlock()
	if fn != nil {
		fn(n)
	}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	removed := h.removeLocked(c)
	n, fn := len(h.clients), h.onChange
	h.mu.Unlock()
	if removed && fn != nil {
		fn(n)
	}
}

func (h *Hub) removeLocked(c *client) bool {
	if _, ok := h.clients[c]; !ok {
		return false
	}
	delete(h.clients, c)
	close(c.send)
	return true
}

// ServeHTTP upgrades the request. Cross-origin connections are refused unless
// the Origin host matches the request host or one of the configured patterns.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.originPatterns})
	if err != nil {
		h.log.Debug("websocket accept refused", "remote", r.RemoteAddr, "origin", r.Header.Get("Origin"), "err", err)
		return
	}
	defer conn.CloseNow()

	c := &client{send: make(chan []byte, sendBuffer)}
	if h.hello != nil {
		if b, err := json.Marshal(h.hello()); err == nil {
			c.send <- b
		}
	}
	h.add(c)
	defer h.remove(c)

	// Browsers send nothing; CloseRead handles control frames and tells us
	// when the peer goes away.
	ctx := conn.CloseRead(r.Context())
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-c.send:
			if !ok {
				conn.Close(websocket.StatusPolicyViolation, "too slow")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
