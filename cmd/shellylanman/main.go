// Command shellylanman is the ShellyLanMan server.
//
// Configuration that must be known before the UI is up comes from flags or
// environment variables; everything else is set in the browser.
//
//	-listen   SHELLYLANMAN_LISTEN   address to listen on; when set, the port can no
//	                                longer be changed in the settings (default ":3082")
//	-data     SHELLYLANMAN_DATA     data directory (default "/data")
//	-origins  SHELLYLANMAN_ORIGINS  extra allowed Origin hosts, comma separated
//	-ingress  SHELLYLANMAN_INGRESS  Home Assistant app: ingress listener address
//	-ingress-from SHELLYLANMAN_INGRESS_FROM  the Supervisor's address (172.30.32.2)
//	-healthcheck                    probe /healthz of a running server and exit (used by Docker)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wimmme/shellylanman/internal/httpapi"
	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/listen"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/store"
	"github.com/wimmme/shellylanman/internal/update"
	"github.com/wimmme/shellylanman/internal/version"
	"github.com/wimmme/shellylanman/internal/web"
)

func main() {
	fixed := flag.String("listen", env("SHELLYLANMAN_LISTEN", ""), "address to listen on (default: the port in the settings, else :3082)")
	dataDir := flag.String("data", env("SHELLYLANMAN_DATA", "/data"), "data directory")
	origins := flag.String("origins", env("SHELLYLANMAN_ORIGINS", ""), "extra allowed Origin hosts, comma separated")
	ingress := flag.String("ingress", env("SHELLYLANMAN_INGRESS", ""), "Home Assistant app: address of the ingress listener, e.g. 172.30.32.1:8099")
	ingressFrom := flag.String("ingress-from", env("SHELLYLANMAN_INGRESS_FROM", "172.30.32.2"), "the only client address the ingress listener accepts (the Supervisor)")
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz of a running server and exit")
	flag.Parse()

	if *healthcheck {
		os.Exit(probe(listen.Addr(*fixed, savedPort(*dataDir))))
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log)
	if err := run(log, *fixed, *dataDir, splitList(*origins), *ingress, *ingressFrom); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, fixed, dataDir string, origins []string, ingress, ingressFrom string) error {
	log.Info("starting ShellyLanMan", "version", version.Version, "commit", version.Commit, "data", dataDir)

	st, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	h := hub.New(origins, func() hub.Event {
		return hub.Event{Type: "hello", Data: map[string]string{"version": version.Version}}
	}, log)
	devices := service.NewDevices(st, shelly.NewClient(), func(typ string, data any) {
		h.Broadcast(hub.Event{Type: typ, Data: data})
	}, log)
	updates := update.New(st, version.Version, func(s update.Status) { h.Broadcast(hub.Event{Type: "update.status", Data: s}) })
	h.OnClientsChanged(func(n int) {
		log.Debug("browsers connected", "n", n)
		devices.SetViewers(n)
	})

	log.Warn("UI authentication is off: anyone who can reach this port can use ShellyLanMan. Keep it on a trusted LAN or behind a reverse proxy with authentication.")

	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	savePort := func(port int) error {
		_, err := st.Update(func(s *store.Settings) { s.Port = port })
		return err
	}
	ln, err := listen.New(srv, listen.Addr(fixed, st.Settings().Port), fixed != "", savePort, log)
	if err != nil {
		return err
	}
	srv.Handler = httpapi.New(httpapi.Config{Store: st, Hub: h, Devices: devices, Updates: updates, Listener: ln, Static: web.Files(), Origins: origins, Log: log})
	if err := ln.Start(); err != nil {
		return err
	}
	var ingressSrv *http.Server
	if ingress != "" { // Home Assistant app: a listener only the Supervisor can use
		ingressLn, err := net.Listen("tcp", ingress)
		if err != nil {
			return fmt.Errorf("ingress listener: %w", err)
		}
		ingressSrv = &http.Server{Handler: httpapi.Ingress(srv.Handler, ingressFrom), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
		go func() {
			if err := ingressSrv.Serve(ingressLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("ingress listener", "err", err)
			}
		}()
		log.Info("Home Assistant ingress", "listen", ingress, "from", ingressFrom)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	devices.Start(ctx)
	go updates.Run(ctx) // does nothing while the setting is "never"
	defer func() {
		stop()         // ends discovery and polling
		devices.Wait() // last archive save
	}()

	select {
	case err := <-ln.Err():
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if ingressSrv != nil {
		_ = ingressSrv.Shutdown(sctx)
	}
	if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// probe returns 0 if the server on listen answers /healthz with 200.
func probe(listen string) int {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

// savedPort reads the port from settings.json without opening the store
// (the health check has no business with the secret key). 0 if unknown.
func savedPort(dataDir string) int {
	b, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if err != nil {
		return 0
	}
	var s struct {
		Port int `json:"port"`
	}
	_ = json.Unmarshal(b, &s)
	return s.Port
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
