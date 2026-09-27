// Package firmware finds the latest stable firmware of a device in Shelly's
// public (undocumented) indexes and keeps verified copies for local download
// (the QR feature, DECISIONS §4.4). Nothing here tells a device to update.
//
//   - Gen1: https://api.shelly.cloud/files/firmware → data.<TYPE>.{url,version};
//     fallback: the shelly-tools.de community archive (highest stable version).
//   - Gen2+: https://updates.shelly.cloud/update/<app> → stable.{version,build_id,url};
//     alt.* and beta are never used.
//
// The Gen2+ hosts use certificates of Shelly's private CA without a subject
// alternative name, which no standard client accepts; their public keys are
// pinned instead. Files are verified before they are served: Gen2+ by the
// SHA-256 in the URL and the manifest's name and version, Gen1 by the
// manifest's build id and the SHA-256 of every part.
package firmware

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sources, shown in the UI.
const (
	SourceShelly  = "shelly"       // the official index
	SourceArchive = "shelly-tools" // community archive (Gen1 fallback)
)

// Default index locations.
const (
	Gen1Index    = "https://api.shelly.cloud/files/firmware"
	Gen2Index    = "https://updates.shelly.cloud/update/"
	ArchiveIndex = "http://archive.shelly-tools.de/archive.php?type="
	ArchiveFile  = "http://archive.shelly-tools.de/version/"
	Gen2Files    = "https://fwcdn.shelly.cloud/"
)

// Pins: SHA-256 of the SubjectPublicKeyInfo of the Shelly update hosts
// (checked 2026-09-27; certificates valid until 2031 and 2033).
var Pins = map[string]string{
	"updates.shelly.cloud": "RGJxcV3nz1CQ56GBEXTeg1eT1ZnvxKqlSlfKfzmePZE=",
	"fwcdn.shelly.cloud":   "pbt8Rw0sM4/of7mADS3mPARWgbeD97GijnascA0gd8c=",
}

// maxFile bounds a firmware download.
const maxFile = 32 << 20

// ErrNotFound: the index has no firmware for this device.
var ErrNotFound = errors.New("no firmware in the index for this device")

// Latest is the newest stable firmware of one device type.
type Latest struct {
	Gen     string `json:"gen"` // "1" or "2" (Gen2+)
	Key     string `json:"key"` // Gen1 type or Gen2+ app
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
	URL     string `json:"-"`
	Source  string `json:"source"`
	sha256  string // Gen2+: from the URL
}

// FileName is the name the file is served under.
func (l *Latest) FileName() string {
	if l.Gen == "1" {
		return l.Key + ".zip"
	}
	return l.Key + "-" + l.Version + ".zip"
}

// Index resolves and caches firmware.
type Index struct {
	Dir  string        // cache directory (/data/firmware)
	TTL  time.Duration // index cache time
	Keep int           // cached files kept (oldest evicted)

	Gen1URL, Gen2URL, ArchiveURL, ArchiveFileURL string
	Gen2Files                                    string       // accepted prefix of Gen2+ file URLs
	HTTP                                         *http.Client // Gen1 / archive
	Pinned                                       *http.Client // Gen2+ hosts

	mu       sync.Mutex
	gen1     map[string]gen1Entry
	gen1At   time.Time
	gen2     map[string]cached
	archive  map[string]cached
	fileLock sync.Mutex
}

type gen1Entry struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

type cached struct {
	l   *Latest
	err error
	at  time.Time
}

// New returns an index with the default sources.
func New(dir string) *Index {
	return &Index{
		Dir: dir, TTL: 6 * time.Hour, Keep: 20,
		Gen1URL: Gen1Index, Gen2URL: Gen2Index, ArchiveURL: ArchiveIndex, ArchiveFileURL: ArchiveFile, Gen2Files: Gen2Files,
		HTTP:   &http.Client{Timeout: 30 * time.Second},
		Pinned: PinnedClient(Pins),
	}
}

// PinnedClient accepts a server only when its leaf public key is pinned for its host.
func PinnedClient(pins map[string]string) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true, // replaced by the pin check below
		VerifyConnection: func(cs tls.ConnectionState) error {
			want, ok := pins[cs.ServerName]
			if !ok || len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("firmware: %s is not a pinned host", cs.ServerName)
			}
			if spki(cs.PeerCertificates[0]) != want {
				return fmt.Errorf("firmware: certificate of %s changed (pin mismatch)", cs.ServerName)
			}
			return nil
		},
	}
	return &http.Client{Timeout: 60 * time.Second, Transport: tr}
}

func spki(c *x509.Certificate) string {
	sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (x *Index) get(ctx context.Context, c *http.Client, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: too large", u)
	}
	return b, nil
}

// Latest returns the newest stable firmware for a device: gen "1" with its
// type, or a Gen2+ generation with its app.
func (x *Index) Latest(ctx context.Context, gen, key string) (*Latest, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	if gen == "1" {
		return x.latestG1(ctx, key)
	}
	return x.latestG2(ctx, key)
}

func (x *Index) latestG1(ctx context.Context, typ string) (*Latest, error) {
	x.mu.Lock()
	fresh := x.gen1 != nil && time.Since(x.gen1At) < x.TTL
	x.mu.Unlock()
	if !fresh {
		if b, err := x.get(ctx, x.HTTP, x.Gen1URL, 4<<20); err == nil {
			var idx struct {
				IsOK bool                 `json:"isok"`
				Data map[string]gen1Entry `json:"data"`
			}
			if json.Unmarshal(b, &idx) == nil && idx.IsOK {
				x.mu.Lock()
				x.gen1, x.gen1At = idx.Data, time.Now()
				x.mu.Unlock()
			}
		}
	}
	x.mu.Lock()
	e, ok := x.gen1[typ]
	x.mu.Unlock()
	if ok && e.URL != "" && e.Version != "" {
		return &Latest{Gen: "1", Key: typ, Version: shortG1(e.Version), Build: e.Version, URL: e.URL, Source: SourceShelly}, nil
	}
	return x.latestArchive(ctx, typ)
}

var versionRe = regexp.MustCompile(`/v?(\d+(?:\.\d+)*)`)

// shortG1: "20230913-113421/v1.14.0-gcb84623" → "1.14.0".
func shortG1(build string) string {
	if m := versionRe.FindStringSubmatch(build); m != nil {
		return m[1]
	}
	return build
}

func (x *Index) latestArchive(ctx context.Context, typ string) (*Latest, error) {
	x.mu.Lock()
	if c, ok := x.archive[typ]; ok && time.Since(c.at) < x.TTL {
		x.mu.Unlock()
		return c.l, c.err
	}
	x.mu.Unlock()
	l, err := func() (*Latest, error) {
		b, err := x.get(ctx, x.HTTP, x.ArchiveURL+url.QueryEscape(typ), 1<<20)
		if err != nil {
			return nil, err
		}
		var list []struct {
			Version string `json:"version"`
			File    string `json:"file"`
		}
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, err
		}
		best := ""
		for _, v := range list {
			s := strings.TrimPrefix(v.Version, "v")
			if !stableRe.MatchString(s) { // rc, beta and custom builds are skipped
				continue
			}
			if best == "" || Compare(s, best) > 0 {
				best = s
			}
		}
		if best == "" {
			return nil, ErrNotFound
		}
		return &Latest{Gen: "1", Key: typ, Version: best, URL: x.ArchiveFileURL + "v" + best + "/" + url.PathEscape(typ) + ".zip", Source: SourceArchive}, nil
	}()
	x.mu.Lock()
	if x.archive == nil {
		x.archive = map[string]cached{}
	}
	x.archive[typ] = cached{l, err, time.Now()}
	x.mu.Unlock()
	return l, err
}

var stableRe = regexp.MustCompile(`^\d+(\.\d+)*$`)

func (x *Index) latestG2(ctx context.Context, app string) (*Latest, error) {
	x.mu.Lock()
	if c, ok := x.gen2[app]; ok && time.Since(c.at) < x.TTL {
		x.mu.Unlock()
		return c.l, c.err
	}
	x.mu.Unlock()
	l, err := func() (*Latest, error) {
		b, err := x.get(ctx, x.Pinned, x.Gen2URL+url.PathEscape(app), 1<<20)
		if err != nil {
			return nil, err
		}
		var idx struct {
			Stable struct {
				Version string `json:"version"`
				BuildID string `json:"build_id"`
				URL     string `json:"url"`
			} `json:"stable"`
		}
		if err := json.Unmarshal(b, &idx); err != nil {
			return nil, err
		}
		if idx.Stable.URL == "" || idx.Stable.Version == "" {
			return nil, ErrNotFound
		}
		m := regexp.MustCompile(`^` + regexp.QuoteMeta(x.Gen2Files) + `[\w\-]+/([\w\-]+)/([0-9a-f]{64})$`).FindStringSubmatch(idx.Stable.URL)
		if m == nil || m[1] != app {
			return nil, fmt.Errorf("unexpected firmware URL for %s", app)
		}
		return &Latest{Gen: "2", Key: app, Version: idx.Stable.Version, Build: idx.Stable.BuildID, URL: idx.Stable.URL, Source: SourceShelly, sha256: m[2]}, nil
	}()
	x.mu.Lock()
	if x.gen2 == nil {
		x.gen2 = map[string]cached{}
	}
	x.gen2[app] = cached{l, err, time.Now()}
	x.mu.Unlock()
	return l, err
}

// ---- files -----------------------------------------------------------------------

// File returns the path of the verified firmware file, downloading it first
// when it is not cached.
func (x *Index) File(ctx context.Context, l *Latest) (string, error) {
	x.fileLock.Lock()
	defer x.fileLock.Unlock()
	dir := filepath.Join(x.Dir, "gen"+l.Gen, safe(l.Key), safe(l.Version))
	path := filepath.Join(dir, l.FileName())
	if _, err := os.Stat(path); err == nil {
		now := time.Now()
		_ = os.Chtimes(path, now, now) // recently used
		return path, nil
	}
	c := x.HTTP
	if l.Gen != "1" {
		c = x.Pinned
	}
	b, err := x.get(ctx, c, l.URL, maxFile)
	if err != nil {
		return "", err
	}
	if err := Verify(l, b); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	x.evict()
	return path, nil
}

var unsafeChars = regexp.MustCompile(`[^\w.\-]`)

func safe(s string) string { return unsafeChars.ReplaceAllString(s, "_") }

// evict keeps the Keep most recently used files.
func (x *Index) evict() {
	type f struct {
		path string
		t    time.Time
	}
	var files []f
	_ = filepath.WalkDir(x.Dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".zip") {
			if info, err := d.Info(); err == nil {
				files = append(files, f{p, info.ModTime()})
			}
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].t.After(files[j].t) })
	for i := x.Keep; i < len(files); i++ {
		_ = os.Remove(files[i].path)
		_ = os.Remove(filepath.Dir(files[i].path)) // empty version folder
	}
}

// Verify checks a downloaded file against what the index said.
func Verify(l *Latest, b []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return errors.New("firmware: not a zip file")
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		BuildID string `json:"build_id"`
		Parts   map[string]struct {
			Src    string `json:"src"`
			SHA256 string `json:"cs_sha256"`
		} `json:"parts"`
	}
	var mdir string
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
		if filepath.Base(f.Name) == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&manifest)
			rc.Close()
			if err != nil {
				return fmt.Errorf("firmware: manifest: %w", err)
			}
			mdir = strings.TrimSuffix(f.Name, "manifest.json")
		}
	}
	if manifest.BuildID == "" {
		return errors.New("firmware: no manifest")
	}
	if l.Gen != "1" {
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != l.sha256 {
			return errors.New("firmware: SHA-256 does not match the index")
		}
		if manifest.Name != l.Key || manifest.Version != l.Version {
			return fmt.Errorf("firmware: file is %s %s, expected %s %s", manifest.Name, manifest.Version, l.Key, l.Version)
		}
		return nil
	}
	// Gen1: the build (official index) or the version (archive), and every part.
	if l.Build != "" && manifest.BuildID != l.Build {
		return fmt.Errorf("firmware: file is build %s, expected %s", manifest.BuildID, l.Build)
	}
	if l.Build == "" && shortG1(manifest.BuildID) != l.Version {
		return fmt.Errorf("firmware: file is version %s, expected %s", shortG1(manifest.BuildID), l.Version)
	}
	for name, p := range manifest.Parts {
		if p.Src == "" || p.SHA256 == "" {
			continue
		}
		f := files[mdir+p.Src]
		if f == nil {
			return fmt.Errorf("firmware: part %s missing", name)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, io.LimitReader(rc, maxFile))
		rc.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != p.SHA256 {
			return fmt.Errorf("firmware: part %s is corrupt", name)
		}
	}
	return nil
}

// ---- versions --------------------------------------------------------------------

// Compare orders versions like "1.14.0", "2.0.1", "1.14.1-rc1", "v1.10.4":
// numeric parts first; a version with a suffix sorts before the same version without.
func Compare(a, b string) int {
	na, sa := splitVersion(a)
	nb, sb := splitVersion(b)
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case sa == sb:
		return 0
	case sa == "":
		return 1
	case sb == "":
		return -1
	case sa < sb:
		return -1
	}
	return 1
}

func splitVersion(v string) ([]int, string) {
	v = strings.TrimPrefix(v, "v")
	num, suffix, _ := strings.Cut(v, "-")
	var out []int
	for _, p := range strings.Split(num, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out, suffix
}
