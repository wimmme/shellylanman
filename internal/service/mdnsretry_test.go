package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// No network address yet at start (an LXC whose DHCP is late): the browser is
// tried again until it runs, and each run's context ends with the run.
func TestKeepBrowsingRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	noIf := errors.New("mdns: no usable network interface")
	tries, stops := 0, []error{}
	running := make(chan context.Context, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		keepBrowsing(ctx, func(run context.Context) error {
			tries++
			if tries < 3 {
				return noIf
			}
			running <- run
			<-run.Done()
			return nil
		}, time.Millisecond, func(err error) { stops = append(stops, err) })
	}()
	var run context.Context
	select {
	case run = <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("the browser was not tried again")
	}
	cancel()
	<-done
	if tries != 3 || len(stops) != 2 || !errors.Is(stops[0], noIf) || !errors.Is(stops[1], noIf) {
		t.Fatalf("tries %d, stops %v", tries, stops)
	}
	if run.Err() == nil {
		t.Fatal("the run's context is not done")
	}
}

// A browser that stops without ctx being done is started again too.
func TestKeepBrowsingRestartsAfterStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tries := 0
	keepBrowsing(ctx, func(context.Context) error {
		tries++
		if tries == 2 {
			cancel()
		}
		return nil
	}, time.Millisecond, func(error) {})
	if tries != 2 {
		t.Fatalf("tries %d", tries)
	}
}
