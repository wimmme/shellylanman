// Package update is the opt-in check for new ShellyLanMan releases on GitHub
// (ShellyScanner: ApplicationUpdateCHK against usna.it; DECISIONS Q18). It is
// off by default; when switched on it asks the GitHub releases API once a day.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// Status is what the UI shows.
type Status struct {
	Mode    string `json:"mode"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	URL     string `json:"url,omitempty"` // release notes
	Newer   bool   `json:"newer"`         // newer than the running version and not skipped
	Skipped bool   `json:"skipped,omitempty"`
	Checked int64  `json:"checked,omitempty"` // unix ms
	Error   string `json:"error,omitempty"`
}

// Checker checks for releases.
type Checker struct {
	Store   *store.Store
	Current string
	URL     string
	HTTP    *http.Client
	Every   time.Duration
	OnChange func(Status)

	mu   sync.Mutex
	last Status
	wake chan struct{}
}

// New returns a checker with the defaults.
func New(st *store.Store, current string, onChange func(Status)) *Checker {
	return &Checker{Store: st, Current: current, URL: ReleasesURL, HTTP: &http.Client{Timeout: 20 * time.Second},
		Every: 24 * time.Hour, OnChange: onChange, wake: make(chan struct{}, 1)}
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

// Check asks GitHub now.
func (c *Checker) Check(ctx context.Context) Status {
	st := c.Store.Settings()
	s := Status{Mode: modeOf(st.UpdateCheck), Current: c.Current, Checked: time.Now().UnixMilli()}
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
		s.Skipped = newer && st.SkipVersion == rel.Tag
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
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
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
