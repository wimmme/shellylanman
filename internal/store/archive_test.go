package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/discovery"
)

func TestArchiveRoundTrip(t *testing.T) {
	s, _ := Open(t.TempDir())
	if devs, err := s.LoadArchive(); err != nil || len(devs) != 0 {
		t.Fatalf("empty archive = %v, %v", devs, err)
	}
	in := []ArchivedDevice{{TypeID: "SHPLG-S", TypeName: "PlugS", Host: "shellyplug-s-aabbcc000001", MAC: "AABBCC000001",
		IP: "192.0.2.1", Port: 80, Name: "Pump", Last: 1790000000000, Gen: "1", Note: "garden", Keyword: "outside"}}
	if err := s.SaveArchive(in); err != nil {
		t.Fatal(err)
	}
	out, err := s.LoadArchive()
	if err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip: %+v, %v", out, err)
	}
	if err := s.ClearArchive(); err != nil {
		t.Fatal(err)
	}
	if devs, _ := s.LoadArchive(); len(devs) != 0 {
		t.Fatal("archive not cleared")
	}
}

func TestArchiveFieldNamesMatchShellyScanner(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_ = s.SaveArchive([]ArchivedDevice{{MAC: "AABBCC000001"}})
	b, _ := os.ReadFile(filepath.Join(dir, archiveFile))
	for _, k := range []string{`"ver"`, `"dev"`, `"tid"`, `"tn"`, `"host"`, `"mac"`, `"ip"`, `"port"`, `"ssid"`, `"last"`, `"bat"`, `"gen"`, `"note"`, `"keyword"`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("archive lacks key %s", k)
		}
	}
}

func TestScanSettingsValidation(t *testing.T) {
	s, _ := Open(t.TempDir())
	bad := []func(*Settings){
		func(st *Settings) { st.Scan.Mode = "nope" },
		func(st *Settings) { st.Scan.Mode = ScanLocal },
		func(st *Settings) { st.Scan.Mode = ScanIP },
		func(st *Settings) {
			st.Scan.Mode = ScanIP
			st.Scan.Ranges = []discovery.Range{{Base: "192.168", First: 1, Last: 2}}
		},
		func(st *Settings) { st.Scan.RefreshSeconds = 0 },
	}
	for i, fn := range bad {
		if _, err := s.Update(fn); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	good := func(st *Settings) {
		st.Scan.Mode = ScanIP
		st.Scan.Ranges = []discovery.Range{{Base: "192.168.1", First: 1, Last: 254}}
	}
	if _, err := s.Update(good); err != nil {
		t.Fatal(err)
	}
}
