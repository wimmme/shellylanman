package fixture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileName(t *testing.T) {
	cases := map[string]string{
		"/shelly":                              "shelly.json",
		"/settings/actions":                    "settings_actions.json",
		"/rpc/Shelly.GetStatus":                "rpc_Shelly.GetStatus.json",
		"/rpc/Shelly.GetDeviceInfo?ident=true": "rpc_Shelly.GetDeviceInfo.json",
	}
	for in, want := range cases {
		if got := FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrubReplacesConsistently(t *testing.T) {
	s := NewScrubber()
	in := `{"mac":"80646F838136","device":{"hostname":"shellyplug-s-80646f838136"},
		"wifi_sta":{"ssid":"MyHome","ip":"192.168.0.86","gw":"192.168.0.1","mask":"255.255.255.0"},
		"bssid":"de:ad:be:ef:00:01","name":"Grondwaterpomp","lat":51.05,"fw":"20230913-113421/v1.14.0-gcb84623"}`
	out, err := s.JSON([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, leak := range []string{"80646F838136", "80646f838136", "MyHome", "192.168.0.86", "de:ad:be:ef", "Grondwaterpomp", "51.05"} {
		if strings.Contains(got, leak) {
			t.Errorf("scrubbed output still contains %q:\n%s", leak, got)
		}
	}
	// Colon-form MACs are replaced first (bssid → …01), then plain ones (…02);
	// the device MAC and the MAC inside its hostname must get the same fake.
	for _, keep := range []string{`"bssid": "aa:bb:cc:00:00:01"`, `"mac": "AABBCC000002"`, "shellyplug-s-aabbcc000002", "255.255.255.0", "v1.14.0-gcb84623", "192.0.2.1"} {
		if !strings.Contains(got, keep) {
			t.Errorf("scrubbed output lacks %q:\n%s", keep, got)
		}
	}
	if found := FindUnscrubbed(got); len(found) != 0 {
		t.Errorf("FindUnscrubbed = %v", found)
	}
}

func TestScrubIsIdempotent(t *testing.T) {
	s := NewScrubber()
	once, _ := s.JSON([]byte(`{"mac":"80646F838136","ip":"10.1.2.3"}`))
	twice, _ := NewScrubber().JSON(once)
	if string(once) != string(twice) {
		t.Fatalf("second scrub changed output:\n%s\n%s", once, twice)
	}
}

// TestRepositoryFixturesAreScrubbed guards the public repository: no real MAC
// or private IP may appear anywhere under testdata/.
func TestRepositoryFixturesAreScrubbed(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if found := FindUnscrubbed(string(b)); len(found) > 0 {
			t.Errorf("%s contains unscrubbed data: %v", path, found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
