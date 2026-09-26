// Command discover is a development tool: it browses mDNS for a while and
// prints every resolved _http._tcp instance and whether it answers GET /shelly.
//
//	go run ./cmd/discover [-t 15s] [-i eth0]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wimmme/shellylanman/internal/discovery"
)

func main() {
	d := flag.Duration("t", 15*time.Second, "how long to browse")
	iface := flag.String("i", "", "interface (default: all)")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), *d)
	defer cancel()
	b := &discovery.Browser{}
	if *iface != "" {
		b.Interfaces = []string{*iface}
	}
	c := &http.Client{Timeout: 5 * time.Second}
	n := 0
	err := b.Run(ctx, func(in discovery.Instance) {
		n++
		shelly := "-"
		if resp, err := c.Get(fmt.Sprintf("http://%s:%d/shelly", in.IP, in.Port)); err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			if resp.StatusCode == 200 && strings.Contains(string(body), `"mac"`) {
				shelly = strings.Join(strings.Fields(string(body)), "")
				if len(shelly) > 110 {
					shelly = shelly[:110] + "…"
				}
			}
		}
		fmt.Printf("%-40s %15s:%-5d %s\n", in.Name, in.IP, in.Port, shelly)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d instances\n", n)
}
