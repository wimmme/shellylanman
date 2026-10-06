// Package logbuf keeps ShellyLanMan's own log in memory, for the Log page
// (DECISIONS P18-1): the last lines, in a ring buffer, nothing on disk.
//
// Handler wraps the real slog handler: every record the real handler accepts
// goes to it unchanged and into the buffer.
package logbuf

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultSize is how many entries the Log page can go back.
const DefaultSize = 1000

// Entry is one log line.
type Entry struct {
	Seq   uint64 `json:"seq"`   // 1, 2, 3 … since the start; the page asks for what comes after the last one it has
	Time  int64  `json:"time"`  // Unix ms
	Level string `json:"level"` // DEBUG, INFO, WARN, ERROR
	Msg   string `json:"msg"`
	Attrs string `json:"attrs,omitempty"` // key=value pairs as the text log writes them
}

// Buffer is a ring buffer of entries. Safe for concurrent use.
type Buffer struct {
	mu      sync.Mutex
	entries []Entry // oldest first, at most size
	size    int
	seq     uint64
	notify  chan Entry
}

// New makes a buffer for size entries (DefaultSize when size <= 0).
func New(size int) *Buffer {
	if size <= 0 {
		size = DefaultSize
	}
	return &Buffer{size: size}
}

// Entries returns the entries with a sequence number above after (0: all), oldest first.
func (b *Buffer) Entries(after uint64) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	i := 0
	for i < len(b.entries) && b.entries[i].Seq <= after {
		i++
	}
	return append([]Entry(nil), b.entries[i:]...)
}

func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	b.seq++
	e.Seq = b.seq
	if len(b.entries) == b.size {
		copy(b.entries, b.entries[1:])
		b.entries = b.entries[:b.size-1]
	}
	b.entries = append(b.entries, e)
	ch := b.notify
	b.mu.Unlock()
	if ch != nil {
		select { // never block the code that logs
		case ch <- e:
		default:
		}
	}
}

// OnEntry calls fn for every new entry, from its own goroutine, so fn may log
// or broadcast without a lock held by the logger. Entries are dropped if fn
// cannot keep up. Call it once, before logging starts.
func (b *Buffer) OnEntry(fn func(Entry)) {
	ch := make(chan Entry, 256)
	b.mu.Lock()
	b.notify = ch
	b.mu.Unlock()
	go func() {
		for e := range ch {
			fn(e)
		}
	}()
}

// Handler returns a slog handler that passes everything to inner and keeps
// what inner accepts in the buffer.
func (b *Buffer) Handler(inner slog.Handler) slog.Handler {
	return &handler{buf: b, inner: inner}
}

type handler struct {
	buf   *Buffer
	inner slog.Handler
	attrs string // from WithAttrs, already formatted
	group string // from WithGroup: "a.b."
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return h.inner.Enabled(ctx, l) }

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&sb, h.group, a)
		return true
	})
	h.buf.add(Entry{Time: r.Time.UnixMilli(), Level: r.Level.String(), Msg: r.Message, Attrs: strings.TrimSpace(sb.String())})
	return h.inner.Handle(ctx, r)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var sb strings.Builder
	sb.WriteString(h.attrs)
	for _, a := range attrs {
		writeAttr(&sb, h.group, a)
	}
	return &handler{buf: h.buf, inner: h.inner.WithAttrs(attrs), attrs: sb.String(), group: h.group}
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{buf: h.buf, inner: h.inner.WithGroup(name), attrs: h.attrs, group: h.group + name + "."}
}

func writeAttr(sb *strings.Builder, group string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, g := range a.Value.Group() {
			writeAttr(sb, group+a.Key+".", g)
		}
		return
	}
	var v string
	switch a.Value.Kind() {
	case slog.KindTime:
		v = a.Value.Time().Format(time.RFC3339)
	default:
		v = a.Value.String()
	}
	if v == "" || strings.ContainsAny(v, " \t\n\"=") {
		v = strconv.Quote(v)
	}
	sb.WriteString(group + a.Key + "=" + v + " ")
}
