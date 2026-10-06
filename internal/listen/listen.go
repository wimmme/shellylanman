// Package listen works out where the web server listens and owns its socket.
//
// One setting decides the port (DECISIONS P17-1): SHELLYLANMAN_PORT — in a
// Docker container from docker-compose.yml or `docker run -e`, in the Home
// Assistant app from the app's option "port" (its start script passes it on).
// The settings page only shows it and where it is set.
package listen

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// DefaultPort is the port when nothing else is set.
const DefaultPort = 3082

// Where the port comes from.
const (
	FromDefault = "default" // nothing set
	FromEnv     = "env"     // SHELLYLANMAN_PORT
	FromApp     = "app"     // the Home Assistant app's option "port"
)

// Resolve works out the listen address from SHELLYLANMAN_PORT (portEnv), else
// DefaultPort. app tells that the Home Assistant app's start script set it
// from the app's option "port".
func Resolve(portEnv string, app bool) (addr, source string, err error) {
	if portEnv == "" {
		return ":" + strconv.Itoa(DefaultPort), FromDefault, nil
	}
	p, err := strconv.Atoi(portEnv)
	if err != nil || p < 1 || p > 65535 {
		return "", "", fmt.Errorf("SHELLYLANMAN_PORT must be a port number from 1 to 65535, got %q", portEnv)
	}
	if app {
		return ":" + strconv.Itoa(p), FromApp, nil
	}
	return ":" + strconv.Itoa(p), FromEnv, nil
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

// InUse wraps a failed listen with what to do about it: an "address already
// in use" names the port and where to set another one.
func InUse(what, addr, where string, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) || strings.Contains(strings.ToLower(fmt.Sprint(err)), "address already in use") {
		return fmt.Errorf("%s: port %d is already used by another program on this host; %s: %w", what, Port(addr), where, err)
	}
	return fmt.Errorf("%s %s: %w", what, addr, err)
}

// Manager serves srv on its listener.
type Manager struct {
	srv  *http.Server
	log  *slog.Logger
	addr string

	mu   sync.Mutex
	port int
	errc chan error
}

// New prepares a manager for addr.
func New(srv *http.Server, addr string, log *slog.Logger) (*Manager, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("listen address %q: %w", addr, err)
	}
	return &Manager{srv: srv, log: log, addr: addr, port: Port(addr), errc: make(chan error, 1)}, nil
}

// Start opens the listener and serves on it. Errors of the server end up on Err.
func (m *Manager) Start() (net.Listener, error) {
	ln, err := net.Listen("tcp", m.addr)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.port = ln.Addr().(*net.TCPAddr).Port
	m.mu.Unlock()
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
	return ln, nil
}

// Err delivers a fatal server error.
func (m *Manager) Err() <-chan error { return m.errc }

// Port is the port it listens on.
func (m *Manager) Port() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.port
}
