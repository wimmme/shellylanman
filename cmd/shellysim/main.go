// Command shellysim runs simulated Shelly devices from fixture sets.
//
//	shellysim -port 8081 testdata/gen1/SHPLG-S testdata/gen2/Plus1
//
// Each fixture directory gets its own port, counting up from -port. Useful for
// trying ShellyLanMan without hardware (IP scan of 127.0.0.1) and for tests.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/wimmme/shellylanman/internal/sim"
)

func main() {
	port := flag.Int("port", 8081, "first port")
	addr := flag.String("addr", "127.0.0.1", "address to bind")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: shellysim [-addr 127.0.0.1] [-port 8081] fixture-dir...")
		os.Exit(2)
	}
	errc := make(chan error, flag.NArg())
	for i, dir := range flag.Args() {
		d, err := sim.New(dir)
		if err != nil {
			log.Fatalf("%s: %v", dir, err)
		}
		listen := fmt.Sprintf("%s:%d", *addr, *port+i)
		log.Printf("%s on http://%s", dir, listen)
		go func() { errc <- http.ListenAndServe(listen, d) }()
	}
	log.Fatal(<-errc)
}
