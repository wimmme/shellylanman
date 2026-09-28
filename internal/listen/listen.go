// Package listen owns the web server's listening socket, so the port can be
// changed from the settings page without a restart.
//
// The port comes from, in order: SHELLYLANMAN_LISTEN / -listen (then it is
// fixed and the settings page only shows it), the port saved in settings.json,
// the default 3082.
package listen

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// DefaultPort is the port of a fresh installation.
const DefaultPort = 3082

// ErrFixed is returned by Move when the address comes from the environment.
var ErrFixed = errors.New("the listen address is set by SHELLYLANMAN_LISTEN or -listen")

// Addr works out the listen address: fixed (from the environment or a flag)
// wins, then the saved port, then DefaultPort.
func Addr(fixed string, saved int) string {
	if fixed != "" {
		return fixed
	}
	if saved < 1 || saved > 65535 {
		saved = DefaultPort
	}
	return ":" + strconv.Itoa(saved)
}

// Port returns the port of a listen address, 0 if it has none.
func Port(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(p)
	return n
}

// Info is what the settings page shows.
type Info struct {
	Port  int  `json:"port"`
	Fixed bool `json:"fixed"` // set by SHELLYLANMAN_LISTEN or -listen
}

// Manager serves srv on one listener at a time.
type Manager struct {
	srv   *http.Server
	fixed bool
	save  func(port int) error
	log   *slog.Logger
	// Grace is how long the old port keeps accepting after a move, so the
	// response to the move request (and a straggling request) still arrive.
	Grace time.Duration

	mu   sync.Mutex
	host string
	port int
	ln   net.Listener
	errc chan error
}

// New prepares a manager for addr. save stores a new port (settings.json).
func New(srv *http.Server, addr string, fixed bool, save func(int) error, log *slog.Logger) (*Manager, error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("listen address %q: %w", addr, err)
	}
	port, _ := strconv.Atoi(p)
	return &Manager{srv: srv, fixed: fixed, save: save, log: log, host: host, port: port, Grace: 3 * time.Second, errc: make(chan error, 1)}, nil
}

// Start opens the listener and serves on it. Errors of the server end up on Err.
func (m *Manager) Start() error {
	ln, err := net.Listen("tcp", net.JoinHostPort(m.host, strconv.Itoa(m.port)))
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.ln = ln
	m.port = ln.Addr().(*net.TCPAddr).Port
	m.mu.Unlock()
	m.serve(ln)
	return nil
}

// Err delivers a fatal server error (not the closing of a moved listener).
func (m *Manager) Err() <-chan error { return m.errc }

func (m *Manager) serve(ln net.Listener) {
	m.log.Info("listening", "addr", ln.Addr().String())
	go func() {
		err := m.srv.Serve(ln)
		if errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
			return
		}
		select {
		case m.errc <- err:
		default:
		}
	}()
}

// Info reports the current port.
func (m *Manager) Info() Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Info{Port: m.port, Fixed: m.fixed}
}

// Move starts listening on port, saves it and closes the old port after Grace.
// Nothing changes if the new port cannot be opened or saved.
func (m *Manager) Move(port int) error {
	if m.fixed {
		return ErrFixed
	}
	if port < 1 || port > 65535 {
		return errors.New("port must be 1–65535")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if port == m.port {
		return nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(m.host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("port %d cannot be used: %w", port, err)
	}
	if err := m.save(port); err != nil {
		ln.Close()
		return err
	}
	old := m.ln
	m.ln, m.port = ln, port
	m.serve(ln)
	m.log.Info("moving to a new port", "port", port, "grace", m.Grace)
	time.AfterFunc(m.Grace, func() { old.Close() })
	return nil
}
