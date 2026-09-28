package httpapi

import (
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wimmme/shellylanman"
	"github.com/wimmme/shellylanman/internal/store"
	"github.com/wimmme/shellylanman/internal/version"
)

// started is when the server started, for the uptime on the About page.
var started = time.Now()

// About is what the About page shows.
type About struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Commit   string   `json:"commit,omitempty"`
	License  string   `json:"license"`
	Source   string   `json:"source"`
	Issues   string   `json:"issues"`
	Started  int64    `json:"started"` // Unix ms
	Runtime  Runtime  `json:"runtime"`
	BasedOn  []Credit `json:"basedOn"`
	Credits  []Credit `json:"credits"`
	Deps     []Dep    `json:"deps"` // Go modules in the binary; the frontend adds its own (deps.json)
	Notice   string   `json:"notice"`
	Language []string `json:"languages"`
	Donate   []Link   `json:"donate"`
}

// Credit names a project ShellyLanMan builds on.
type Credit struct {
	Name    string `json:"name"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	License string `json:"license"`
	What    string `json:"what"`
}

// Runtime describes what the server runs on.
type Runtime struct {
	Go     string `json:"go"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Kernel string `json:"kernel,omitempty"`
	MemMB  uint64 `json:"memMB"` // memory obtained from the OS
	Data   string `json:"data"`  // what stores the data: JSON files in /data
}

// Dep is one third-party module.
type Dep struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
}

// Link is a named URL.
type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// goLicences are the licences of the Go modules (THIRD_PARTY_NOTICES.md).
var goLicences = map[string]string{
	"github.com/coder/websocket": "ISC",
	"golang.org/x/net":           "BSD-3-Clause",
	"golang.org/x/sys":           "BSD-3-Clause",
	"github.com/skip2/go-qrcode": "MIT",
}

func (s *server) aboutRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/about", s.about)
	mux.HandleFunc("GET /api/v1/about/changelog", text(shellylanman.Changelog))
	mux.HandleFunc("GET /api/v1/about/license", text(shellylanman.License))
	mux.HandleFunc("GET /api/v1/about/notices", text(shellylanman.Notices))
}

func text(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(body))
	}
}

func goDeps() []Dep {
	deps := []Dep{{Name: "Go standard library", Version: runtime.Version(), License: "BSD-3-Clause"}}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return deps
	}
	for _, m := range info.Deps {
		lic := goLicences[m.Path]
		if lic == "" {
			lic = "see THIRD_PARTY_NOTICES.md"
		}
		deps = append(deps, Dep{Name: m.Path, Version: m.Version, License: lic})
	}
	return deps
}

func kernel() string {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *server) about(w http.ResponseWriter, r *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	writeJSON(w, http.StatusOK, About{
		Name:    "ShellyLanMan",
		Version: version.Version,
		Commit:  version.Commit,
		License: "GPL-3.0-or-later",
		Source:  "https://github.com/wimmme/shellylanman",
		Issues:  "https://github.com/wimmme/shellylanman/issues/new",
		Started: started.UnixMilli(),
		Runtime: Runtime{Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, Kernel: kernel(), MemMB: ms.Sys >> 20, Data: "JSON files"},
		BasedOn: []Credit{
			{
				Name:    "ShellyScanner",
				Author:  "Antonio Flaccomio (usnasoft)",
				URL:     "https://github.com/usnasoft/shellyscanner",
				License: "GPL-3.0",
				What:    "functional and code reference",
			},
			{
				Name:    "MikroDash",
				Author:  "SecOps-7",
				URL:     "https://github.com/SecOps-7/MikroDash",
				License: "MIT",
				What:    "idea, inspiration and look and feel",
			},
		},
		Credits: []Credit{
			{Name: "ShellyScanner", Author: "Antonio Flaccomio (usnasoft)", URL: "https://www.usna.it/shellyscanner/", License: "GPL-3.0", What: "every feature, the terminology and the Shelly know-how"},
			{Name: "MikroDash", Author: "SecOps-7", URL: "https://github.com/SecOps-7/MikroDash", License: "MIT", What: "the idea, the inspiration, design tokens, palettes and appearance settings"},
			{Name: "Bundled fonts", Author: "see OFL.txt", URL: "/fonts/OFL.txt", License: "OFL-1.1", What: "Inter, Oxanium, IBM Plex Sans, Nunito, Roboto, JetBrains Mono"},
			{Name: "CodeMirror 6", Author: "Marijn Haverbeke and others", URL: "https://codemirror.net", License: "MIT", What: "script editor"},
			{Name: "Chart.js", Author: "Chart.js contributors", URL: "https://www.chartjs.org", License: "MIT", What: "charts (with chartjs-plugin-zoom and Hammer.js, MIT)"},
			{Name: "go-qrcode", Author: "Tom Harwood", URL: "https://github.com/skip2/go-qrcode", License: "MIT", What: "QR codes of the local firmware download"},
		},
		Deps:     goDeps(),
		Notice:   "ShellyLanMan is an independent project. It is heavily based on ShellyScanner but not affiliated with it, nor with or endorsed by usnasoft or Shelly Group. Shelly is a trademark of its owner.",
		Language: store.Languages,
		Donate: []Link{
			{Name: "Buy me a coffee", URL: "https://buymeacoffee.com/wimmme"},
			{Name: "PayPal", URL: "https://www.paypal.com/donate/?business=LPS62D2BRTD2Y&no_recurring=0&item_name=You+help+me+buying+coffee+and+tokens+for+coding+%3A-%29&currency_code=EUR"},
		},
	})
}
