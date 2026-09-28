package listen

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestAddr(t *testing.T) {
	for _, c := range []struct {
		fixed string
		saved int
		want  string
	}{
		{"", 0, ":3082"},
		{"", 8095, ":8095"},
		{"", 70000, ":3082"},
		{"127.0.0.1:9000", 8095, "127.0.0.1:9000"},
	} {
		if got := Addr(c.fixed, c.saved); got != c.want {
			t.Errorf("Addr(%q, %d) = %q, want %q", c.fixed, c.saved, got, c.want)
		}
	}
	if Port(":8095") != 8095 || Port("bad") != 0 {
		t.Error("Port")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func get(port int) error {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func TestMove(t *testing.T) {
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), IdleTimeout: time.Millisecond}
	defer srv.Close()
	var saved []int
	m, err := New(srv, "127.0.0.1:0", false, func(p int) error { saved = append(saved, p); return nil }, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	m.Grace = 50 * time.Millisecond
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	first := m.Info().Port
	if err := get(first); err != nil {
		t.Fatal(err)
	}
	next := freePort(t)
	if err := m.Move(next); err != nil {
		t.Fatal(err)
	}
	if m.Info().Port != next || len(saved) != 1 || saved[0] != next {
		t.Fatalf("info %+v saved %v", m.Info(), saved)
	}
	if err := get(next); err != nil {
		t.Fatalf("new port: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if get(first) == nil {
		t.Fatal("old port still answers after the grace period")
	}
	select {
	case err := <-m.Err():
		t.Fatalf("closing the old port is not a server error: %v", err)
	default:
	}
}

func TestMoveRefused(t *testing.T) {
	srv := &http.Server{Handler: http.NotFoundHandler()}
	defer srv.Close()
	saveErr := errors.New("disk full")
	m, _ := New(srv, "127.0.0.1:0", false, func(int) error { return saveErr }, slog.New(slog.DiscardHandler))
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	port := m.Info().Port
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if m.Move(busy.Addr().(*net.TCPAddr).Port) == nil {
		t.Error("a port in use was accepted")
	}
	if !errors.Is(m.Move(freePort(t)), saveErr) {
		t.Error("a failed save must refuse the move")
	}
	if m.Move(0) == nil {
		t.Error("port 0 accepted")
	}
	if m.Info().Port != port {
		t.Error("port changed after refused moves")
	}
	if get(port) != nil {
		t.Error("server stopped answering")
	}

	fixed, _ := New(srv, "127.0.0.1:1", true, nil, slog.New(slog.DiscardHandler))
	if !errors.Is(fixed.Move(9000), ErrFixed) {
		t.Error("a fixed address moved")
	}
}
