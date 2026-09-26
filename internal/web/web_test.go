package web

import (
	"io/fs"
	"regexp"
	"testing"
)

// TestEmbeddedFrontendIsComplete runs where the frontend is built in (the
// Docker test stage, CI): every asset index.html references must be embedded,
// and the font licence must ship with the fonts.
func TestEmbeddedFrontendIsComplete(t *testing.T) {
	files := Files()
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		t.Skip("frontend not built into this checkout (run via Docker)")
	}
	refs := regexp.MustCompile(`(?:src|href)="/([^"]+)"`).FindAllStringSubmatch(string(index), -1)
	if len(refs) < 3 {
		t.Fatalf("index.html references only %d assets", len(refs))
	}
	for _, r := range refs {
		st, err := fs.Stat(files, r[1])
		if err != nil {
			t.Errorf("index.html references %s, which is not embedded", r[1])
			continue
		}
		if st.Size() == 0 {
			t.Errorf("%s is empty", r[1])
		}
	}
	if _, err := fs.Stat(files, "fonts/OFL.txt"); err != nil {
		t.Error("fonts are embedded without fonts/OFL.txt")
	}
}
