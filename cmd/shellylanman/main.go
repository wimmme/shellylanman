// Command shellylanman is the ShellyLanMan server.
//
// Configuration that must be known before the UI is up comes from flags or
// environment variables; everything else is set in the browser.
//
//	-port     SHELLYLANMAN_PORT     port of the web UI (default 3082); the Home Assistant
//	                                app sets it from its option "port" (DECISIONS P17-1)
//	-data     SHELLYLANMAN_DATA     data directory (default "/data")
//	-origins  SHELLYLANMAN_ORIGINS  extra allowed Origin hosts, comma separated
//	-ingress  SHELLYLANMAN_INGRESS  Home Assistant app: ingress listener address; port 0 =
//	                                the port the Supervisor chose (ingress_port: 0)
//	-ingress-from SHELLYLANMAN_INGRESS_FROM  the Supervisor's address (172.30.32.2)
//	-mcp-local SHELLYLANMAN_MCP_LOCAL  Home Assistant app: token-less MCP on a loopback address
//	SHELLYLANMAN_RESET_PASSWORD=1   remove the UI password at start (forgotten password)
//	-healthcheck                    probe /healthz of a running server and exit (used by Docker)
package main

import (
	"context"
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

	"github.com/wimmme/shellylanman/internal/auth"
	"github.com/wimmme/shellylanman/internal/httpapi"
	"github.com/wimmme/shellylanman/internal/hub"
	"github.com/wimmme/shellylanman/internal/listen"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/store"
	"github.com/wimmme/shellylanman/internal/supervisor"
	"github.com/wimmme/shellylanman/internal/update"
	"github.com/wimmme/shellylanman/internal/version"
	"github.com/wimmme/shellylanman/internal/web"
)

func main() {
	port := flag.String("port", env("SHELLYLANMAN_PORT", ""), "port of the web UI (default 3082)")
	dataDir := flag.String("data", env("SHELLYLANMAN_DATA", "/data"), "data directory")
	origins := flag.String("origins", env("SHELLYLANMAN_ORIGINS", ""), "extra allowed Origin hosts, comma separated")
	ingress := flag.String("ingress", env("SHELLYLANMAN_INGRESS", ""), "Home Assistant app: address of the ingress listener, e.g. 172.30.32.1:0 (0: the port the Supervisor chose)")
	ingressFrom := flag.String("ingress-from", env("SHELLYLANMAN_INGRESS_FROM", "172.30.32.2"), "the only client address the ingress listener accepts (the Supervisor)")
	mcpLocal := flag.String("mcp-local", env("SHELLYLANMAN_MCP_LOCAL", ""), "Home Assistant app: token-less MCP listener on a loopback address, e.g. 127.0.0.1:8097")
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz of a running server and exit")
	flag.Parse()

	if *healthcheck {
		os.Exit(probe(*dataDir))
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log)
	if err := run(log, *port, *dataDir, splitList(*origins), *ingress, *ingressFrom, *mcpLocal); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, port, dataDir string, origins []string, ingress, ingressFrom, mcpLocal string) error {
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

	if os.Getenv("SHELLYLANMAN_RESET_PASSWORD") == "1" { // forgotten password (DECISIONS P15-5)
		if err := httpapi.ResetPassword(st); err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
		log.Warn("UI password removed (SHELLYLANMAN_RESET_PASSWORD=1): set a new one in Settings → Security and remove the variable")
	}
	if pw, _, _ := st.Secret(auth.PasswordSecret); pw == "" {
		log.Warn("UI authentication is off: anyone who can reach this port can use ShellyLanMan. Set a password in Settings → Security, keep it on a trusted LAN, or put it behind a reverse proxy with authentication.")
	} else {
		log.Info("UI password is on")
	}

	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	sup := supervisor.FromEnv() // set only when running as a Home Assistant app
	announce := func(port int) {
		if sup == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Home Assistant Core runs on the host network: the loopback address reaches us.
		// Success is logged too, so the app's log shows that Home Assistant was told (P12-12).
		url := fmt.Sprintf("http://127.0.0.1:%d", port)
		if err := sup.Announce(ctx, supervisor.ServiceShellyLanMan, map[string]any{"url": url}); err != nil {
			log.Warn("Home Assistant discovery", "service", supervisor.ServiceShellyLanMan, "err", err)
		} else {
			log.Info("announced to Home Assistant", "service", supervisor.ServiceShellyLanMan, "url", url)
		}
		if mcpLocal != "" {
			url := "http://" + mcpLocal + "/mcp"
			if err := sup.Announce(ctx, supervisor.ServiceMCP, map[string]any{"url": url}); err != nil {
				log.Warn("Home Assistant discovery", "service", supervisor.ServiceMCP, "err", err)
			} else {
				log.Info("announced to Home Assistant", "service", supervisor.ServiceMCP, "url", url)
			}
		}
	}
	// Where the web UI listens: one setting (DECISIONS P17-1), shown on the settings page.
	addr, source, err := listen.Resolve(port, sup != nil)
	if err != nil {
		return err
	}
	where := "set another one with SHELLYLANMAN_PORT (docker-compose.yml or docker run -e)"
	if sup != nil {
		where = "set another one in Home Assistant: Settings → Apps → ShellyLanMan → Configuration → port"
	}
	ln, err := listen.New(srv, addr, log)
	if err != nil {
		return err
	}
	ports := httpapi.Ports{Source: source, App: sup != nil}
	if ingress != "" { // Home Assistant app: the port the Supervisor chose (ingress_port: 0)
		if host, p, err := net.SplitHostPort(ingress); err == nil && p == "0" && sup != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			ip, err := sup.IngressPort(ctx)
			cancel()
			if err != nil {
				return fmt.Errorf("ingress port from the Supervisor: %w", err)
			}
			ingress = net.JoinHostPort(host, fmt.Sprint(ip))
		}
		ports.Ingress = ingress
	}
	ports.MCPLocal = mcpLocal
	srv.Handler = httpapi.New(httpapi.Config{Store: st, Hub: h, Devices: devices, Updates: updates, Port: ln.Port, Ports: ports, Static: web.Files(), Origins: origins, Log: log})
	if _, err := ln.Start(); err != nil {
		return listen.InUse("web UI", addr, where, err)
	}
	writeAddr(dataDir, ln.Port(), log)
	var ingressSrv *http.Server
	if ingress != "" { // Home Assistant app: a listener only the Supervisor can use
		ingressLn, err := net.Listen("tcp", ingress)
		if err != nil {
			return listen.InUse("ingress (Home Assistant's sidebar)", ingress, "restart the app so the Supervisor chooses a free one", err)
		}
		ingressSrv = &http.Server{Handler: httpapi.Ingress(srv.Handler, ingressFrom), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
		go func() {
			if err := ingressSrv.Serve(ingressLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("ingress listener", "err", err)
			}
		}()
		log.Info("Home Assistant ingress", "listen", ingress, "from", ingressFrom)
	}
	var mcpLocalSrv *http.Server
	if mcpLocal != "" { // Home Assistant app: MCP without token, loopback only (DECISIONS Q5)
		host, _, err := net.SplitHostPort(mcpLocal)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("SHELLYLANMAN_MCP_LOCAL must be a loopback address, got %q", mcpLocal)
		}
		mcpLn, err := net.Listen("tcp", mcpLocal)
		if err != nil {
			return listen.InUse("local MCP listener", mcpLocal, "set another one in Home Assistant: Settings → Apps → ShellyLanMan → Configuration → mcp_local_port", err)
		}
		mcpLocalSrv = &http.Server{Handler: httpapi.MCPLocal(httpapi.Config{Store: st, Devices: devices, Ports: ports, Log: log}), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			if err := mcpLocalSrv.Serve(mcpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("local MCP listener", "err", err)
			}
		}()
		log.Info("local MCP listener (no token, loopback only)", "listen", mcpLocal)
	}
	go announce(ln.Port())

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
	if mcpLocalSrv != nil {
		_ = mcpLocalSrv.Shutdown(sctx)
	}
	if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// addrFile holds the port the running server listens on, for the health check:
// Docker runs it with the container's environment, which does not have what
// the Home Assistant app's start script exported.
const addrFile = "listen.port"

func writeAddr(dataDir string, port int, log *slog.Logger) {
	if err := os.WriteFile(filepath.Join(dataDir, addrFile), []byte(fmt.Sprint(port)), 0o600); err != nil {
		log.Warn("health check port file", "err", err)
	}
}

// probe returns 0 if the running server answers /healthz with 200.
func probe(dataDir string) int {
	port := strings.TrimSpace(readFile(filepath.Join(dataDir, addrFile)))
	if port == "" {
		port = fmt.Sprint(listen.DefaultPort)
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
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

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
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
