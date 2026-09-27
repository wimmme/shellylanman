// Package sim is a simulated Shelly device that answers from a fixture set
// (see package fixture). It backs integration tests and cmd/shellysim.
//
// GET requests return the recorded file for their path; POST /rpc returns the
// recorded result for the method in a Gen2+ RPC envelope. Optional
// authentication (Basic / SHA-256 Digest, JSON-RPC auth object for POST).
// Since Phase 4 every request is logged (Calls) and relay commands change the
// served status (Gen1 /relay/N?turn=, Gen2+ Switch.Set / Switch.Toggle);
// other commands are accepted and answered with an empty result.
package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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

	mu     sync.Mutex
	calls  []string                 // "GET /relay/0?turn=on", "RPC Switch.Set {"id":0,"on":true}"
	status map[string]any           // served status, changed by relay commands
	down   bool                     // drop every connection: the device looks off line
	rpcWS  map[chan []byte]struct{} // RPC WebSocket clients (notifications)
}

// SetDown makes the device unreachable (connections are closed unanswered).
func (d *Device) SetDown(down bool) {
	d.mu.Lock()
	d.down = down
	d.mu.Unlock()
}

// Calls returns the requests received so far (except /shelly), oldest first.
func (d *Device) Calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

func (d *Device) logCall(c string) {
	d.mu.Lock()
	if len(d.calls) >= 1000 {
		d.calls = d.calls[1:]
	}
	d.calls = append(d.calls, c)
	d.mu.Unlock()
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
	d.mu.Lock()
	down := d.down
	d.mu.Unlock()
	if down {
		if hj, ok := w.(http.Hijacker); ok {
			if c, _, err := hj.Hijack(); err == nil {
				c.Close()
				return
			}
		}
		panic(http.ErrAbortHandler)
	}
	rpcPost := r.Method == http.MethodPost && r.URL.Path == "/rpc"
	var body []byte
	if rpcPost {
		body, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
	}
	if d.Password != "" && r.URL.Path != "/shelly" {
		if rpcPost && r.Header.Get("Authorization") == "" {
			if !d.rpcAuthorized(w, body) {
				return
			}
		} else if !d.authorized(w, r) {
			return
		}
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/debug/log" && !d.gen1 && strings.EqualFold(r.Header.Get("Upgrade"), "websocket"):
		d.serveLog(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/rpc" && !d.gen1 && strings.EqualFold(r.Header.Get("Upgrade"), "websocket"):
		d.serveRPCWS(w, r)
	case r.Method == http.MethodGet:
		if r.URL.Path != "/shelly" {
			d.logCall("GET " + r.URL.RequestURI())
		}
		d.serveGet(w, r)
	case rpcPost:
		d.serveRPC(w, body)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// statusPath is the file whose content relay commands change.
func (d *Device) statusPath() string {
	if d.gen1 {
		return "/status"
	}
	return "/rpc/Shelly.GetStatus"
}

// loadStatus reads the status fixture once; callers hold d.mu.
func (d *Device) loadStatus() {
	if d.status != nil {
		return
	}
	d.status = map[string]any{}
	if b, err := os.ReadFile(filepath.Join(d.dir, fixture.FileName(d.statusPath()))); err == nil {
		_ = json.Unmarshal(b, &d.status)
	}
}

func (d *Device) serveGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case r.URL.Path == d.statusPath():
		d.mu.Lock()
		d.loadStatus()
		b, _ := json.Marshal(d.status)
		d.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	case d.gen1 && strings.HasPrefix(r.URL.Path, "/relay/") && q.Get("turn") != "":
		idx, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/relay/"))
		writeJSON(w, http.StatusOK, d.setRelay(idx, q.Get("turn")))
	case !d.gen1 && (r.URL.Path == "/rpc/Switch.Set" || r.URL.Path == "/rpc/Switch.Toggle"):
		idx, _ := strconv.Atoi(q.Get("id"))
		turn := "toggle"
		if r.URL.Path == "/rpc/Switch.Set" {
			turn = map[bool]string{true: "on", false: "off"}[q.Get("on") == "true"]
		}
		writeJSON(w, http.StatusOK, d.setRelay(idx, turn))
	case isCommand(r):
		writeJSON(w, http.StatusOK, map[string]any{})
	default:
		d.serveFile(w, fixture.FileName(r.URL.Path))
	}
}

// isCommand: a request that changes something, answered with {} when no
// fixture exists for it.
func isCommand(r *http.Request) bool {
	p := r.URL.Path
	if p == "/reboot" || p == "/rpc/Shelly.Reboot" {
		return true
	}
	for _, pre := range []string{"/relay/", "/roller/", "/light/", "/white/", "/color/", "/settings"} {
		if strings.HasPrefix(p, pre) && r.URL.RawQuery != "" {
			return true
		}
	}
	return strings.HasPrefix(p, "/rpc/") && isCommandMethod(strings.TrimPrefix(p, "/rpc/"))
}

// isCommandMethod: RPC methods that change something.
func isCommandMethod(m string) bool {
	_, verb, ok := strings.Cut(m, ".")
	if !ok {
		return false
	}
	if i := strings.LastIndex(verb, "."); i >= 0 { // Thermostat.Schedule.AddProfile
		verb = verb[i+1:]
	}
	for _, pre := range []string{"Set", "Add", "Delete", "Create", "Put", "Remove", "Update", "Toggle", "Call", "call", "Reboot"} {
		if strings.HasPrefix(verb, pre) {
			return true
		}
	}
	return false
}

// setRelay applies on/off/toggle to relay idx and returns the relay state
// (Gen1: the /relay/N answer; Gen2+: the Switch.Set/Toggle "was_on" result).
func (d *Device) setRelay(idx int, turn string) map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loadStatus()
	var rel map[string]any
	var key string
	if d.gen1 {
		key = "ison"
		if list, ok := d.status["relays"].([]any); ok && idx < len(list) {
			rel, _ = list[idx].(map[string]any)
		}
	} else {
		key = "output"
		rel, _ = d.status[fmt.Sprintf("switch:%d", idx)].(map[string]any)
	}
	if rel == nil {
		return map[string]any{}
	}
	was, _ := rel[key].(bool)
	now := turn == "on" || (turn == "toggle" && !was)
	rel[key] = now
	rel["source"] = "http"
	if d.gen1 {
		return rel
	}
	return map[string]any{"was_on": was}
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
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Auth   map[string]any  `json:"auth"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (d *Device) serveRPC(w http.ResponseWriter, body []byte) {
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"id": req.ID, "src": d.id, "error": rpcError{-103, "invalid request"}})
		return
	}
	params := string(req.Params)
	if params == "" {
		params = "{}"
	}
	d.logCall("RPC " + req.Method + " " + params)
	reply := func(result any) {
		writeJSON(w, http.StatusOK, map[string]any{"id": req.ID, "src": d.id, "result": result})
	}
	if req.Method == "Switch.Set" || req.Method == "Switch.Toggle" {
		var p struct {
			ID int  `json:"id"`
			On bool `json:"on"`
		}
		_ = json.Unmarshal(req.Params, &p)
		turn := "toggle"
		if req.Method == "Switch.Set" {
			turn = map[bool]string{true: "on", false: "off"}[p.On]
		}
		reply(d.setRelay(p.ID, turn))
		return
	}
	b, err := os.ReadFile(filepath.Join(d.dir, fixture.RPCFileName(req.Method)))
	if err != nil {
		if isCommandMethod(req.Method) {
			reply(map[string]any{})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"id": req.ID, "src": d.id, "error": rpcError{404, "No handler for " + req.Method}})
		return
	}
	reply(json.RawMessage(b))
}

// rpcAuthorized checks the JSON-RPC "auth" object of a POST /rpc and answers
// 401 with the challenge in "message" (a JSON string) like the devices do.
func (d *Device) rpcAuthorized(w http.ResponseWriter, body []byte) bool {
	var req rpcRequest
	_ = json.Unmarshal(body, &req)
	if a := req.Auth; a != nil {
		str := func(k string) string {
			switch v := a[k].(type) {
			case string:
				return v
			case float64:
				return strconv.FormatFloat(v, 'f', -1, 64)
			}
			return ""
		}
		nc := str("nc")
		if nc == "" {
			nc = "1"
		}
		ha1 := sha256hex("admin:" + d.id + ":" + d.Password)
		want := sha256hex(ha1 + ":" + simNonce + ":" + nc + ":" + str("cnonce") + ":auth:" + sha256hex("dummy_method:dummy_uri"))
		if str("username") == "admin" && str("nonce") == simNonce && str("response") == want {
			return true
		}
	}
	ch, _ := json.Marshal(map[string]any{"auth_type": "digest", "nonce": 1790000000, "nc": 1, "realm": d.id, "algorithm": "SHA-256"})
	writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "message": string(ch)})
	return false
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

// serveRPCWS: the device's RPC WebSocket. After the client's first request
// it receives the notifications sent with Notify.
func (d *Device) serveRPCWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	if _, _, err := c.Read(r.Context()); err != nil {
		return
	}
	ch := make(chan []byte, 16)
	d.mu.Lock()
	if d.rpcWS == nil {
		d.rpcWS = map[chan []byte]struct{}{}
	}
	d.rpcWS[ch] = struct{}{}
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.rpcWS, ch)
		d.mu.Unlock()
	}()
	ctx := c.CloseRead(r.Context())
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-ch:
			if c.Write(ctx, websocket.MessageText, b) != nil {
				return
			}
		}
	}
}

// RPCClients is the number of connected RPC WebSocket clients.
func (d *Device) RPCClients() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.rpcWS)
}

// Notify sends a NotifyEvent with the given events to the RPC WebSocket clients.
func (d *Device) Notify(events ...map[string]any) {
	b, _ := json.Marshal(map[string]any{"src": d.id, "dst": "shellylanman", "method": "NotifyEvent", "params": map[string]any{"events": events}})
	d.mu.Lock()
	defer d.mu.Unlock()
	for ch := range d.rpcWS {
		select {
		case ch <- b:
		default:
		}
	}
}
