package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRotates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	w, err := Open(dir, "x.log", 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, s := range []string{"aaaa\n", "bbbb\n", "cccc\n", "dddd\n", "eeee\n", "ffff\n", "gggg\n"} {
		if n, err := w.Write([]byte(s)); n != len(s) || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
	}
	p := filepath.Join(dir, "x.log")
	if got := read(t, p); got != "gggg\n" {
		t.Errorf("current = %q", got)
	}
	if got := read(t, p+".1"); got != "eeee\nffff\n" {
		t.Errorf(".1 = %q", got)
	}
	if got := read(t, p+".2"); got != "cccc\ndddd\n" {
		t.Errorf(".2 = %q", got)
	}
	if _, err := os.Stat(p + ".3"); !os.IsNotExist(err) {
		t.Errorf(".3 exists: only 3 files are kept")
	}
}

func TestAppendsAfterRestart(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir, "x.log", 0, 0)
	w.Write([]byte("one\n"))
	w.Close()
	w, _ = Open(dir, "x.log", 0, 0)
	w.Write([]byte("two\n"))
	w.Close()
	if got := read(t, filepath.Join(dir, "x.log")); got != "one\ntwo\n" {
		t.Errorf("file = %q", got)
	}
}

func TestLongLineStillWritten(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir, "x.log", 4, 2)
	defer w.Close()
	long := strings.Repeat("z", 20) + "\n"
	w.Write([]byte(long))
	if got := read(t, filepath.Join(dir, "x.log")); got != long {
		t.Errorf("file = %q", got)
	}
}
