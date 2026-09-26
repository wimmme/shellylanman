// Package shelly talks to Shelly devices over their local HTTP APIs.
//
// Gen1 is REST over HTTP GET with Basic authentication. Gen2+ is RPC; reads
// use the GET form /rpc/<Method>?<params>, and protected devices answer 401
// with an HTTP Digest challenge (SHA-256, user "admin") — see
// https://shelly-api-docs.shelly.cloud/gen2/General/Authentication.
//
// Every request to one device goes through that device's Conn, which keeps
// one request in flight and ~59 ms between requests, like ShellyScanner
// (Devices.MULTI_QUERY_DELAY): "too many calls disturb some devices,
// especially Gen1".
package shelly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Pacing between two requests to the same device.
const Pacing = 59 * time.Millisecond

var (
	// ErrUnauthorized: the device needs (other) credentials.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrNotShelly: the address answered, but not like a Shelly.
	ErrNotShelly = errors.New("not a Shelly device")
)

// OfflineError wraps a network failure (timeout, refused, unreachable).
type OfflineError struct{ Err error }

func (e *OfflineError) Error() string { return "offline: " + e.Err.Error() }
func (e *OfflineError) Unwrap() error { return e.Err }

// APIError is an error answer from the device (HTTP status, Gen2 code/message).
type APIError struct {
	HTTPStatus int
	Code       int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("device error %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("device HTTP %d", e.HTTPStatus)
}

// IsOffline reports whether err means the device could not be reached.
func IsOffline(err error) bool {
	var o *OfflineError
	return errors.As(err, &o)
}

// Credentials for a protected device. Gen2+ ignore User (always "admin").
type Credentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// Info is the answer of GET /shelly, which every generation serves without
// authentication.
type Info struct {
	Gen      int    `json:"gen"`   // 0 for Gen1 (field absent)
	Type     string `json:"type"`  // Gen1 model, e.g. "SHPLG-S"
	App      string `json:"app"`   // Gen2+ application, e.g. "Plus1"
	Model    string `json:"model"` // Gen2+ hardware model, e.g. "SNSW-001X16EU"
	MAC      string `json:"mac"`
	ID       string `json:"id"`      // Gen2+ device id, e.g. "shellyplus1-aabbcc000001"
	Name     string `json:"name"`    // Gen2+ device name, if set
	AuthG1   bool   `json:"auth"`    // Gen1: login enabled
	AuthG2   bool   `json:"auth_en"` // Gen2+: authentication enabled
	FW       string `json:"fw"`      // Gen1 firmware build
	FWID     string `json:"fw_id"`   // Gen2+ firmware build
	Ver      string `json:"ver"`
	Svc0Type string `json:"-"` // Gen3 XT1: svc0.type
}

// Generation as ShellyScanner reports it: "1".."4".
func (i Info) Generation() string {
	if i.Gen == 0 {
		return "1"
	}
	return strconv.Itoa(i.Gen)
}

// AuthEnabled reports whether the device requires credentials.
func (i Info) AuthEnabled() bool { return i.AuthG1 || i.AuthG2 }

// ParseInfo decodes a /shelly answer. A Shelly is recognised by "mac", which
// every generation includes (ShellyScanner: Devices.isShelly).
func ParseInfo(b []byte) (Info, error) {
	var raw struct {
		Info
		MAC  *string `json:"mac"`
		Svc0 struct {
			Type string `json:"type"`
		} `json:"svc0"`
	}
	if err := json.Unmarshal(b, &raw); err != nil || raw.MAC == nil {
		return Info{}, ErrNotShelly
	}
	info := raw.Info
	info.MAC = *raw.MAC
	info.Svc0Type = raw.Svc0.Type
	return info, nil
}

// Client holds the HTTP client shared by all devices.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a client with sensible LAN timeouts.
func NewClient() *Client {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     5 * time.Minute, // ShellyScanner: setDestinationIdleTimeout(300_000)
		DisableCompression:  true,
	}
	return &Client{HTTP: &http.Client{Transport: tr, Timeout: 15 * time.Second}}
}

// Probe asks addr ("ip:port") for /shelly without authentication or pacing.
// Used by discovery; returns ErrNotShelly or an OfflineError when appropriate.
func (c *Client) Probe(ctx context.Context, addr string, timeout time.Duration) (Info, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/shelly", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Info{}, nil, &OfflineError{err}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Info{}, nil, &OfflineError{err}
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, nil, ErrNotShelly
	}
	info, err := ParseInfo(b)
	return info, b, err
}

// Conn is the paced, authenticated connection to one device.
type Conn struct {
	client *Client
	addr   string // "ip:port"
	gen1   bool

	mu     sync.Mutex // one request in flight
	last   time.Time
	cred   *Credentials
	digest *digestState
}

// Conn returns a connection to the device at addr. gen1 selects Basic auth.
func (c *Client) Conn(addr string, gen1 bool) *Conn {
	return &Conn{client: c, addr: addr, gen1: gen1}
}

// Addr is the device address ("ip:port").
func (d *Conn) Addr() string { return d.addr }

// SetCredentials sets or clears (nil) the credentials used from now on.
func (d *Conn) SetCredentials(c *Credentials) {
	d.mu.Lock()
	d.cred = c
	d.digest = nil
	d.mu.Unlock()
}

// Get requests path (e.g. "/status" or "/rpc/Shelly.GetStatus") and returns the body.
func (d *Conn) Get(ctx context.Context, path string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if wait := Pacing - time.Since(d.last); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() { d.last = time.Now() }()

	b, status, hdr, err := d.do(ctx, path, d.authHeader(path))
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized && !d.gen1 && d.cred != nil {
		// Digest: answer the challenge (or a stale nonce) once.
		if ds, ok := parseChallenge(hdr.Get("WWW-Authenticate")); ok {
			d.digest = ds
			time.Sleep(Pacing)
			b, status, _, err = d.do(ctx, path, d.authHeader(path))
			if err != nil {
				return nil, err
			}
		}
	}
	switch {
	case status == http.StatusOK:
		return b, apiErrorInBody(b)
	case status == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, apiErrorFrom(status, b)
	}
}

// GetJSON is Get followed by json.Unmarshal into v.
func (d *Conn) GetJSON(ctx context.Context, path string, v any) error {
	b, err := d.Get(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (d *Conn) do(ctx context.Context, path, auth string) ([]byte, int, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+d.addr+path, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := d.client.HTTP.Do(req)
	if err != nil {
		return nil, 0, nil, &OfflineError{err}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 0, nil, &OfflineError{err}
	}
	return b, resp.StatusCode, resp.Header, nil
}

func (d *Conn) authHeader(path string) string {
	if d.cred == nil {
		return ""
	}
	if d.gen1 {
		return basicAuth(d.cred.User, d.cred.Password)
	}
	if d.digest != nil {
		return d.digest.header(http.MethodGet, path, d.cred.Password)
	}
	return ""
}

// apiErrorInBody catches Gen2 errors returned with HTTP 200 as
// {"code":-114,"message":"..."} (ShellyScanner: AbstractG2Device.getJSON).
func apiErrorInBody(b []byte) error {
	if len(b) == 0 || b[0] != '{' {
		return nil
	}
	var e struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &e) == nil && e.Code != nil && e.Message != "" {
		return &APIError{HTTPStatus: http.StatusOK, Code: *e.Code, Message: e.Message}
	}
	return nil
}

func apiErrorFrom(status int, b []byte) error {
	e := &APIError{HTTPStatus: status}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &body) == nil {
		e.Code, e.Message = body.Code, body.Message
	}
	return e
}
