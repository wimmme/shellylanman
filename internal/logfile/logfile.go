// Package logfile writes ShellyLanMan's log to a file in the data directory as
// well, so what happened survives a restart (DECISIONS P26-3): when the file
// reaches its size limit it is renamed to name.1 (name.1 to name.2, …) and the
// oldest is dropped.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Defaults: five files of 1 MB.
const (
	DefaultMaxSize = 1 << 20
	DefaultKeep    = 5
)

// Writer is an io.Writer that rotates. Safe for concurrent use. Write never
// fails: a log that cannot be written must not stop the log on stdout (an
// io.MultiWriter stops at the first error); the first error is reported once
// on stderr.
type Writer struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	keep    int // files in all, the current one included
	f       *os.File
	size    int64
	failed  bool
}

// Open opens (appends to) dir/name, creating dir. maxSize <= 0 and keep < 1
// take the defaults.
func Open(dir, name string, maxSize int64, keep int) (*Writer, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if keep < 1 {
		keep = DefaultKeep
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	w := &Writer{path: filepath.Join(dir, name), maxSize: maxSize, keep: keep}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.f, w.size = f, fi.Size()
	return nil
}

// Write appends p, rotating first when p would not fit any more.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil && w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		w.fail(w.rotate())
	}
	if w.f == nil { // a rotation that failed: try to go on in a fresh file
		if err := w.open(); err != nil {
			w.fail(err)
			return len(p), nil
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	w.fail(err)
	return len(p), nil
}

func (w *Writer) rotate() error {
	err := w.f.Close()
	w.f = nil
	os.Remove(fmt.Sprintf("%s.%d", w.path, w.keep-1))
	for i := w.keep - 2; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1))
	}
	if w.keep > 1 {
		if rerr := os.Rename(w.path, w.path+".1"); rerr != nil && err == nil {
			err = rerr
		}
	} else {
		os.Remove(w.path)
	}
	if oerr := w.open(); oerr != nil && err == nil {
		err = oerr
	}
	return err
}

func (w *Writer) fail(err error) {
	if err != nil && !w.failed {
		w.failed = true
		fmt.Fprintf(os.Stderr, "log file %s: %v (the log goes on on stdout)\n", w.path, err)
	}
}

// Close closes the current file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
