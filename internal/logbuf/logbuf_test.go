package logbuf

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func newLog(size int, level slog.Level) (*slog.Logger, *Buffer, *bytes.Buffer) {
	b := New(size)
	var out bytes.Buffer
	inner := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: level})
	return slog.New(b.Handler(inner)), b, &out
}

func TestKeepsTheLastEntriesOldestFirst(t *testing.T) {
	log, b, _ := newLog(3, slog.LevelInfo)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		log.Info(m)
	}
	got := b.Entries(0)
	if len(got) != 3 || got[0].Msg != "c" || got[2].Msg != "e" {
		t.Fatalf("entries: %+v", got)
	}
	if got[0].Seq != 3 || got[2].Seq != 5 {
		t.Fatalf("sequence numbers: %+v", got)
	}
}

func TestEntriesAfter(t *testing.T) {
	log, b, _ := newLog(10, slog.LevelInfo)
	log.Info("one")
	log.Info("two")
	log.Info("three")
	got := b.Entries(2)
	if len(got) != 1 || got[0].Msg != "three" {
		t.Fatalf("after 2: %+v", got)
	}
	if len(b.Entries(3)) != 0 || len(b.Entries(99)) != 0 {
		t.Fatal("nothing is newer than the last entry")
	}
}

func TestOnlyWhatTheRealHandlerAccepts(t *testing.T) { // DECISIONS P18-3: info and up
	log, b, out := newLog(10, slog.LevelInfo)
	log.Debug("hidden")
	log.Info("shown")
	log.Warn("careful")
	log.Error("broken")
	got := b.Entries(0)
	if len(got) != 3 || got[0].Level != "INFO" || got[1].Level != "WARN" || got[2].Level != "ERROR" {
		t.Fatalf("entries: %+v", got)
	}
	if strings.Contains(out.String(), "hidden") || !strings.Contains(out.String(), "shown") {
		t.Fatalf("the real handler still gets its own lines: %q", out.String())
	}
}

func TestAttributes(t *testing.T) {
	log, b, _ := newLog(10, slog.LevelInfo)
	log.With("tool", "shelly_switch").WithGroup("g").Info("call", "device", "Grondwaterpomp", "note", "two words", "n", 3)
	e := b.Entries(0)[0]
	want := `tool=shelly_switch g.device=Grondwaterpomp g.note="two words" g.n=3`
	if e.Msg != "call" || e.Attrs != want {
		t.Fatalf("attrs %q, want %q", e.Attrs, want)
	}
}

func TestOnEntryIsAsynchronousAndNeverBlocks(t *testing.T) {
	log, b, _ := newLog(10, slog.LevelInfo)
	got := make(chan Entry, 1)
	b.OnEntry(func(e Entry) { got <- e })
	log.Info("hello")
	select {
	case e := <-got:
		if e.Msg != "hello" || e.Seq != 1 {
			t.Fatalf("entry: %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
	}
	// A consumer that is stuck must not stop the logger.
	stuck := New(10)
	block := make(chan struct{})
	stuck.OnEntry(func(Entry) { <-block })
	l := slog.New(stuck.Handler(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			l.Info("x")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("logging blocked")
	}
	close(block)
}

func TestConcurrent(t *testing.T) {
	log, b, _ := newLog(50, slog.LevelInfo)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				log.Info("m", "j", j)
				_ = b.Entries(0)
			}
		}()
	}
	wg.Wait()
	got := b.Entries(0)
	if len(got) != 50 || got[49].Seq != 800 {
		t.Fatalf("%d entries, last %d", len(got), got[len(got)-1].Seq)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("sequence broken at %d: %d after %d", i, got[i].Seq, got[i-1].Seq)
		}
	}
}
