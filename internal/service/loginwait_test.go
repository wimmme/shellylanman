package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/sim"
	"github.com/wimmme/shellylanman/internal/store"
)

// P33-1: a device that refuses the login is not asked every refresh (firmware 2.x locks
// itself longer after each failed login), but Refresh and new credentials ask at once.
func TestLoginRefusedIsAskedLessOften(t *testing.T) {
	m, _, ctx := newService(t, func(s *store.Settings) { s.Scan.RefreshSeconds = 1 })
	m.SetViewers(1) // the 1 s refresh of an open page
	dir := fixtureDir(t, "gen2/Plus1", map[string]string{
		"shelly.json": `{"id":"shellyplus1-aabbcc000012","mac":"AABBCC000012","model":"SNSW-001X16EU","gen":2,"app":"Plus1","auth_en":true}`,
	})
	d, err := sim.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Password = "s3cret"
	var asked atomic.Int32
	srv := httptest.NewServer(countRequests(&asked, d))
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")

	// No password set: login, "none set", a next try in about 30 s.
	m.handle(ctx, addr, "shellyplus1-aabbcc000012", true)
	dev := waitDevice(t, m, "AABBCC000012", func(d model.Device) bool { return d.Status == model.StatusLogin })
	if dev.LoginRefused || dev.LoginRetry < time.Now().Add(20*time.Second).UnixMilli() {
		t.Fatalf("no password: refused %v, retry in %v", dev.LoginRefused, time.Until(time.UnixMilli(dev.LoginRetry)))
	}

	// A wrong password: refused, and not asked at every 1 s refresh.
	if err := m.SetGlobalCredentials(context.Background(), shelly.Credentials{User: "admin", Password: "wrong"}); err != nil {
		t.Fatal(err)
	}
	waitDevice(t, m, "AABBCC000012", func(d model.Device) bool { return d.Status == model.StatusLogin && d.LoginRefused })
	time.Sleep(500 * time.Millisecond)
	before := asked.Load()
	time.Sleep(3500 * time.Millisecond)
	if n := asked.Load() - before; n != 0 {
		t.Fatalf("a device that refused the login was asked %d times in 3.5 s", n)
	}

	// Refresh asks at once (and keeps the wait).
	m.Refresh([]string{"AABBCC000012"})
	deadline := time.Now().Add(5 * time.Second)
	for asked.Load() == before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if asked.Load() == before {
		t.Fatal("Refresh did not ask the device")
	}

	// The right password: on line, the wait gone.
	if err := m.SetGlobalCredentials(context.Background(), shelly.Credentials{User: "admin", Password: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	dev = waitDevice(t, m, "AABBCC000012", online)
	if dev.LoginRefused || dev.LoginRetry != 0 {
		t.Fatalf("on line but still marked: %+v", dev)
	}
}

func TestLoginWaitsGrow(t *testing.T) {
	m, _, _ := newService(t, nil)
	e := &entry{}
	var got []time.Duration
	for i := 0; i < 6; i++ {
		d := model.Device{Status: model.StatusLogin}
		e.loginNext = time.Time{}.Add(time.Nanosecond) // the last wait has passed
		m.loginBackoff(e, &d)
		got = append(got, time.Until(e.loginNext).Round(time.Second))
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("waits %v, want %v", got, want)
		}
	}
	d := model.Device{Status: model.StatusOnline}
	m.loginBackoff(e, &d)
	if e.loginFails != 0 || !e.loginNext.IsZero() {
		t.Fatalf("an answer does not reset the waits: %d %v", e.loginFails, e.loginNext)
	}
}

func countRequests(n *atomic.Int32, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		next.ServeHTTP(w, r)
	})
}
