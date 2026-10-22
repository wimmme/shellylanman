// Package update is the opt-in check for new ShellyLanMan releases on GitHub
// (ShellyScanner: ApplicationUpdateCHK against usna.it; DECISIONS Q18). It is
// off by default; when switched on it asks the GitHub releases API once a day.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/store"
)

// Modes of the setting (ShellyScanner: never / stable / all).
const (
	Never  = "never"
	Stable = "stable"
	All    = "all" // pre-releases included
)

// ReleasesURL lists the project's releases.
const ReleasesURL = "https://api.github.com/repos/wimmme/shellylanman/releases?per_page=30"

// AppTagsURL lists the app's tags in shellylanman-ha (app-vX.Y.Z): an app is updated through Home
// Assistant, which sees a new version when that repository has one. The app has tags, no GitHub
// releases: the releases there are the integration's, with its own versions (DECISIONS P24-1).
const AppTagsURL = "https://api.github.com/repos/wimmme/shellylanman-ha/git/matching-refs/tags/app-v"

// AppNotesURL is the app's changelog; a version's section is its anchor (0.9.13 → #0913).
const AppNotesURL = "https://github.com/wimmme/shellylanman-ha/blob/main/shellylanman/CHANGELOG.md"

// Status is what the UI shows.
type Status struct {
	Mode    string `json:"mode"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	URL     string `json:"url,omitempty"` // release notes
	Newer   bool   `json:"newer"`         // newer than the running version and not skipped
	Skipped bool   `json:"skipped,omitempty"`
	App     bool   `json:"app,omitempty"`     // a Home Assistant app: the releases are the app's, "skip" does not apply
	Checked int64  `json:"checked,omitempty"` // unix ms
	Error   string `json:"error,omitempty"`
}

// Checker checks for releases.
type Checker struct {
	Store    *store.Store
	Current  string
	URL      string
	HTTP     *http.Client
	Every    time.Duration
	OnChange func(Status)

	app  bool // Home Assistant app
	mu   sync.Mutex
	last Status
	wake chan struct{}
}

// New returns a checker with the defaults.
func New(st *store.Store, current string, onChange func(Status)) *Checker {
	return &Checker{Store: st, Current: current, URL: ReleasesURL, HTTP: &http.Client{Timeout: 20 * time.Second},
		Every: 24 * time.Hour, OnChange: onChange, wake: make(chan struct{}, 1)}
}

// HomeAssistantApp makes the checker look at the app's tags (shellylanman-ha); skipping a version
// is not offered there, as Home Assistant keeps showing the update.
func (c *Checker) HomeAssistantApp() {
	c.URL, c.app = AppTagsURL, true
}

// Run checks at start and then every c.Every while the setting is on; Wake
// re-evaluates at once (after a settings change).
func (c *Checker) Run(ctx context.Context) {
	for {
		if c.Store.Settings().UpdateCheck != Never && c.Store.Settings().UpdateCheck != "" {
			c.Check(ctx)
		} else {
			c.set(Status{Mode: Never, Current: c.Current})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.Every):
		case <-c.wake:
		}
	}
}

// Wake asks Run to check again now.
func (c *Checker) Wake() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Status returns the last result.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.last
	if s.Mode == "" {
		s.Mode, s.Current = modeOf(c.Store.Settings().UpdateCheck), c.Current
	}
	return s
}

func modeOf(m string) string {
	if m == Stable || m == All {
		return m
	}
	return Never
}

func (c *Checker) set(s Status) {
	c.mu.Lock()
	changed := s != c.last
	c.last = s
	c.mu.Unlock()
	if changed && c.OnChange != nil {
		c.OnChange(s)
	}
}

type release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	URL        string `json:"html_url"`
}

// MinRecheck is how soon a check on request (the About page) may ask GitHub again: without a
// token GitHub allows 60 requests an hour per address.
const MinRecheck = time.Minute

// CheckNow checks on request (the About page is opened, or its ↻ pressed), unless the last
// check was less than MinRecheck ago: then it returns that one.
func (c *Checker) CheckNow(ctx context.Context) Status {
	c.mu.Lock()
	last := c.last
	c.mu.Unlock()
	if last.Checked > 0 && last.Mode == modeOf(c.Store.Settings().UpdateCheck) &&
		time.Since(time.UnixMilli(last.Checked)) < MinRecheck {
		return c.Status()
	}
	return c.Check(ctx)
}

// Check asks GitHub now.
func (c *Checker) Check(ctx context.Context) Status {
	st := c.Store.Settings()
	s := Status{Mode: modeOf(st.UpdateCheck), Current: c.Current, Checked: time.Now().UnixMilli(), App: c.app}
	if s.Mode == Never {
		c.set(s)
		return s
	}
	rel, err := c.latest(ctx, s.Mode == All)
	if err != nil {
		s.Error = err.Error()
		c.set(s)
		return s
	}
	if rel != nil {
		s.Latest, s.URL = rel.Tag, rel.URL
		newer := c.Current != "dev" && firmware.Compare(rel.Tag, c.Current) > 0
		s.Skipped = newer && !c.app && st.SkipVersion == rel.Tag
		s.Newer = newer && !s.Skipped
	}
	c.set(s)
	return s
}

func (c *Checker) latest(ctx context.Context, pre bool) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases: HTTP %d", resp.StatusCode)
	}
	var list []release
	if c.app {
		if list, err = appTags(resp.Body); err != nil {
			return nil, err
		}
	} else if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	var best *release
	for i := range list {
		r := &list[i]
		if r.Draft || (r.Prerelease && !pre) {
			continue
		}
		if best == nil || firmware.Compare(r.Tag, best.Tag) > 0 {
			best = r
		}
	}
	return best, nil
}

// appTags turns the app's tags (refs/tags/app-v0.9.13) into releases: tag v0.9.13, the notes the
// version's section of the app's changelog. App versions have no pre-releases.
func appTags(body io.Reader) ([]release, error) {
	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(body).Decode(&refs); err != nil {
		return nil, err
	}
	var list []release
	for _, r := range refs {
		v, ok := strings.CutPrefix(r.Ref, "refs/tags/app-v")
		if !ok || v == "" {
			continue
		}
		list = append(list, release{Tag: "v" + v, URL: AppNotesURL + "#" + strings.ReplaceAll(v, ".", "")})
	}
	return list, nil
}
