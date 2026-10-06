package listen

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	for _, c := range []struct {
		port      string
		app       bool
		addr, src string
	}{
		{"", false, ":3082", FromDefault},
		{"8096", false, ":8096", FromEnv},
		{"8096", true, ":8096", FromApp},
	} {
		addr, src, err := Resolve(c.port, c.app)
		if err != nil || addr != c.addr || src != c.src {
			t.Errorf("Resolve(%q, %v) = %q %q %v", c.port, c.app, addr, src, err)
		}
	}
	for _, bad := range []string{"0", "65536", "abc", ":3082"} {
		if _, _, err := Resolve(bad, false); err == nil {
			t.Errorf("SHELLYLANMAN_PORT=%q accepted", bad)
		}
	}
}

func TestStartAndInUse(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := New(&http.Server{Handler: http.NotFoundHandler()}, "127.0.0.1:0", log)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := m.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if m.Port() == 0 {
		t.Fatal("no port")
	}
	busy := ln.Addr().String()
	_, err = net.Listen("tcp", busy)
	if err == nil {
		t.Fatal("second listen on the same port worked")
	}
	msg := InUse("web UI", busy, "set another one with SHELLYLANMAN_PORT", err).Error()
	if !strings.Contains(msg, "is already used by another program") || !strings.Contains(msg, "SHELLYLANMAN_PORT") {
		t.Fatalf("message %q", msg)
	}
}
