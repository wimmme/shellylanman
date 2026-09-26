// Package sim is a simulated Shelly device that answers from a fixture set
// (see package fixture). It backs integration tests and cmd/shellysim.
//
// Phase 1 scope: read-only and stateless. GET requests return the recorded
// file for their path; POST /rpc returns the recorded result for the method in
// a Gen2+ RPC envelope. Optional authentication (Basic / SHA-256 Digest)
// since Phase 2. Stateful writes, WebSocket events and
// mDNS announcement are added in the phases that need them.
package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/coder/websocket"

	"github.com/wimmme/shellylanman/internal/fixture"
)

// Device serves one fixture directory.
type Device struct {
	dir  string
	id   string // Gen2+ "id" from shelly.json, used as "src" in RPC replies
	gen1 bool

	// Password, when set, protects everything except /shelly: Gen1 with Basic
	// auth (User/Password), Gen2+ with SHA-256 Digest for user "admin".
	User, Password string
}

// New loads the device in dir. dir must contain at least shelly.json.
func New(dir string) (*Device, error) {
	b, err := os.ReadFile(filepath.Join(dir, fixture.FileName("/shelly")))
	if err != nil {
		return nil, err
	}
	var info struct {
		ID  string `json:"id"`
		Gen int    `json:"gen"`
	}
	_ = json.Unmarshal(b, &info)
	return &Device{dir: dir, id: info.ID, gen1: info.Gen == 0}, nil
}

func (d *Device) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if d.Password != "" && r.URL.Path != "/shelly" && !d.authorized(w, r) {
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/debug/log" && !d.gen1 && strings.EqualFold(r.Header.Get("Upgrade"), "websocket"):
		d.serveLog(w, r)
	case r.Method == http.MethodGet:
		d.serveFile(w, fixture.FileName(r.URL.Path))
	case r.Method == http.MethodPost && r.URL.Path == "/rpc":
		d.serveRPC(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

const simNonce = "1790000000"

// authorized checks credentials the way the real devices do, answering 401
// (with a Digest challenge for Gen2+) when they are missing or wrong.
func (d *Device) authorized(w http.ResponseWriter, r *http.Request) bool {
	if d.gen1 {
		if u, p, ok := r.BasicAuth(); ok && u == d.User && p == d.Password {
			return true
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="`+d.id+`"`)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Digest ") {
		p := parseDigest(strings.TrimPrefix(h, "Digest "))
		realm := d.id
		want := digestResponse(p["username"], realm, d.Password, r.Method, p["uri"], simNonce, p["nc"], p["cnonce"], p["qop"])
		if p["username"] == "admin" && p["nonce"] == simNonce && p["uri"] == r.URL.RequestURI() && p["response"] == want {
			return true
		}
	}
	w.Header().Set("WWW-Authenticate", `Digest qop="auth", realm="`+d.id+`", nonce="`+simNonce+`", algorithm=SHA-256`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"code":401,"message":"unauthorized"}`))
	return false
}

func parseDigest(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[strings.ToLower(k)] = strings.Trim(v, `"`)
		}
	}
	return out
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// digestResponse is RFC 7616 with SHA-256, written independently of the
// client's implementation so the two check each other.
func digestResponse(user, realm, pass, method, uri, nonce, nc, cnonce, qop string) string {
	return sha256hex(sha256hex(user+":"+realm+":"+pass) + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + sha256hex(method+":"+uri))
}

func (d *Device) serveFile(w http.ResponseWriter, name string) {
	b, err := os.ReadFile(filepath.Join(d.dir, name))
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

type rpcRequest struct {
	ID     any    `json:"id"`
	Method string `json:"method"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (d *Device) serveRPC(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err := json.Unmarshal(body, &req); err != nil || req.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"id": req.ID, "src": d.id, "error": rpcError{-103, "invalid request"}})
		return
	}
	b, err := os.ReadFile(filepath.Join(d.dir, fixture.RPCFileName(req.Method)))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"id": req.ID, "src": d.id, "error": rpcError{404, "No handler for " + req.Method}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": req.ID, "src": d.id, "result": json.RawMessage(b)})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// LogLines are sent to each /debug/log WebSocket client (Gen2+).
var LogLines = []string{`{"ts":1790000000.1,"level":2,"data":"shelly_notification:163 Status change of switch:0: {\"output\":true}","fd":1}`}

func (d *Device) serveLog(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := c.CloseRead(r.Context())
	for _, l := range LogLines {
		if c.Write(ctx, websocket.MessageText, []byte(l)) != nil {
			return
		}
	}
	<-ctx.Done()
}
