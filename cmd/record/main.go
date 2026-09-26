// Command record reads a real Shelly device and writes a scrubbed fixture set.
//
//	record -host 192.168.1.50 -out testdata/gen1/SHPLG-S
//
// Password-protected devices are not supported yet (added with authentication
// in Phase 2).
//
// It only issues read requests (GET). Output files are scrubbed by package
// fixture (MACs, private IPs, SSIDs, names, credentials); review them before
// committing anyway.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wimmme/shellylanman/internal/fixture"
)

// Read-only requests recorded per generation. Mirrors the "info requests" of
// ShellyScanner (AbstractG1Device / AbstractG2Device.getInfoRequests).
var (
	gen1Paths = []string{"/shelly", "/settings", "/settings/actions", "/status", "/ota"}
	gen2RPC   = []string{
		"Shelly.GetDeviceInfo", "Shelly.GetConfig", "Shelly.GetStatus", "Shelly.CheckForUpdate",
		"Shelly.GetComponents", "Schedule.List", "Webhook.List", "Script.List", "KVS.GetMany",
		"WiFi.ListAPClients", "BLE.CloudRelay.ListInfos", "Sys.GetStatus",
	}
)

func main() {
	host := flag.String("host", "", "device address, e.g. 192.168.1.50 or 192.168.1.50:8080")
	out := flag.String("out", "", "output directory, e.g. testdata/gen1/SHPLG-S")
	flag.Parse()
	if *host == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := record(*host, *out); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		os.Exit(1)
	}
}

func record(host, out string) error {
	c := &http.Client{Timeout: 15 * time.Second}
	base := "http://" + host
	shelly, err := get(c, base+"/shelly")
	if err != nil {
		return fmt.Errorf("/shelly: %w", err)
	}
	var info struct {
		Gen int `json:"gen"`
	}
	if err := json.Unmarshal(shelly, &info); err != nil {
		return fmt.Errorf("/shelly is not JSON: %w", err)
	}
	paths := gen1Paths
	if info.Gen >= 2 {
		paths = []string{"/shelly"}
		for _, m := range gen2RPC {
			paths = append(paths, "/rpc/"+m)
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	s := fixture.NewScrubber()
	for _, p := range paths {
		b, err := get(c, base+p)
		if err != nil {
			fmt.Printf("skip %-32s %v\n", p, err)
			continue
		}
		clean, err := s.JSON(b)
		if err != nil {
			fmt.Printf("skip %-32s %v\n", p, err)
			continue
		}
		name := fixture.FileName(p)
		if err := os.WriteFile(filepath.Join(out, name), clean, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", filepath.Join(out, name))
		time.Sleep(59 * time.Millisecond) // same pacing as ShellyScanner (Devices.MULTI_QUERY_DELAY)
	}
	return nil
}

func get(c *http.Client, url string) ([]byte, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return b, nil
}
