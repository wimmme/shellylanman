package discovery

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Range is an IP-scan range on the last octet: Base "192.168.1", First 1, Last 254
// (ShellyScanner: IPCollection, settings BASE_SCAN/FIRST_SCAN/LAST_SCAN, up to 10).
type Range struct {
	Base  string `json:"base"`
	First int    `json:"first"`
	Last  int    `json:"last"`
}

// MaxRanges is how many ranges ShellyScanner accepts.
const MaxRanges = 10

// Validate checks the format ShellyScanner requires ("192.168.1", 0..255, first <= last).
func (r Range) Validate() error {
	parts := strings.Split(r.Base, ".")
	if len(parts) != 3 {
		return fmt.Errorf("base %q: expected three octets, e.g. 192.168.1", r.Base)
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return fmt.Errorf("base %q: invalid octet %q", r.Base, p)
		}
	}
	if r.First < 0 || r.Last > 255 || r.First > r.Last {
		return fmt.Errorf("range %s.%d-%d: need 0 <= lower <= higher <= 255", r.Base, r.First, r.Last)
	}
	return nil
}

// Addresses expands the ranges in order.
func Addresses(ranges []Range) []string {
	var out []string
	for _, r := range ranges {
		for i := r.First; i <= r.Last; i++ {
			out = append(out, r.Base+"."+strconv.Itoa(i))
		}
	}
	return out
}

// IPScan probes every address of the ranges on port with probe, staggered
// 4 ms apart like ShellyScanner (Devices.scanByIP) and at most `parallel` at a
// time. ShellyScanner first pings (InetAddress.isReachable); we connect
// directly with a short timeout instead (FEATURE_PARITY D3, agreed).
func IPScan(ctx context.Context, ranges []Range, port, parallel int, probe func(ctx context.Context, addr string)) {
	if parallel <= 0 {
		parallel = 32
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, ip := range Addresses(ranges) {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-time.After(4 * time.Millisecond):
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(addr string) {
			defer func() { <-sem; wg.Done() }()
			probe(ctx, addr)
		}(net.JoinHostPort(ip, strconv.Itoa(port)))
	}
	wg.Wait()
}
