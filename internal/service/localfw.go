package service

// Local firmware download via QR code — the one new feature (DECISIONS §4.4):
// the server compares each device's firmware with Shelly's index and, for a
// newer stable version, hands out a link on this server from which a phone
// downloads the verified file (to update the device through its own access
// point). Devices are never told to update from it.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/model"
)

// LinkTTL is how long a download link stays valid.
var LinkTTL = 24 * time.Hour

var (
	// ErrUpToDate: the index has no newer stable firmware for the device.
	ErrUpToDate = errors.New("the device already runs the latest stable firmware")
	// ErrLinkExpired: the link is too old or the index moved on.
	ErrLinkExpired = errors.New("download link expired")
	// ErrNoLocalFW: the device type has no firmware in the indexes (BLU devices, unmanaged).
	ErrNoLocalFW = errors.New("no local firmware for this device")
)

// IndexRow is the server-side comparison of one device with the Shelly index.
type IndexRow struct {
	ID      string `json:"id"`
	Key     string `json:"key,omitempty"` // Gen1 type / Gen2+ app
	Current string `json:"current,omitempty"`
	Latest  string `json:"latest,omitempty"`
	Source  string `json:"source,omitempty"` // shelly | shelly-tools
	Newer   bool   `json:"newer"`
	Error   string `json:"error,omitempty"`
}

// LocalLink is what the QR modal shows.
type LocalLink struct {
	URL      string `json:"url"`
	QR       string `json:"qr"` // data:image/png;base64,…
	Name     string `json:"name"`
	Model    string `json:"model"`
	Current  string `json:"current"`
	Version  string `json:"version"`
	Source   string `json:"source"`
	FileName string `json:"fileName"`
	Expires  int64  `json:"expires"` // unix ms
	Warning  string `json:"warning,omitempty"`
}

// FirmwareSource returns the index (created on first use under /data/firmware).
func (m *Devices) FirmwareSource() *firmware.Index {
	m.fwIndexOnce.Do(func() {
		if m.fwIndex == nil {
			m.fwIndex = firmware.New(m.store.Dir() + "/firmware")
		}
	})
	return m.fwIndex
}

// SetFirmwareSource replaces the index (tests).
func (m *Devices) SetFirmwareSource(x *firmware.Index) { m.fwIndex = x }

// fwKey: the index key and the running version of a device, from /shelly.
func fwKey(d model.Device, info shellyInfo) (gen, key, current string, ok bool) {
	switch {
	case !d.Managed || d.Gen == model.GenBLU || d.Gen == model.GenBTHome:
		return "", "", "", false
	case d.Gen == "1":
		return "1", info.Type, ShortVersion(info.FW), info.Type != ""
	default:
		cur := info.Ver
		if cur == "" {
			cur = ShortVersion(info.FWID)
		}
		return "2", info.App, cur, info.App != ""
	}
}

type shellyInfo struct{ Type, App, FW, FWID, Ver string }

func (m *Devices) infoOf(e *entry) (model.Device, shellyInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return e.dev, shellyInfo{e.info.Type, e.info.App, e.info.FW, e.info.FWID, e.info.Ver}
}

// FirmwareIndex compares the devices with the index (all Wi-Fi devices when ids is empty).
func (m *Devices) FirmwareIndex(ctx context.Context, ids []string) ([]IndexRow, error) {
	var list []*entry
	m.mu.Lock()
	if len(ids) == 0 {
		for _, e := range m.devs {
			list = append(list, e)
		}
	} else {
		for _, id := range ids {
			e, ok := m.devs[id]
			if !ok {
				m.mu.Unlock()
				return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
			}
			list = append(list, e)
		}
	}
	m.mu.Unlock()
	x := m.FirmwareSource()
	out := []IndexRow{}
	for _, e := range list {
		d, info := m.infoOf(e)
		gen, key, cur, ok := fwKey(d, info)
		if !ok {
			continue
		}
		row := IndexRow{ID: d.ID, Key: key, Current: cur}
		l, err := x.Latest(ctx, gen, key)
		if err != nil {
			row.Error = err.Error()
		} else {
			row.Latest, row.Source = l.Version, l.Source
			row.Newer = cur != "" && firmware.Compare(l.Version, cur) > 0
		}
		out = append(out, row)
	}
	return out, nil
}

type linkClaims struct {
	Gen     string `json:"g"`
	Key     string `json:"k"`
	Version string `json:"v"`
	Exp     int64  `json:"e"`
}

const linkContext = "fw-link"

// LocalDownload makes the download link and QR code for a device. base is
// the address phones use to reach this server ("http://host:port").
func (m *Devices) LocalDownload(ctx context.Context, id, base string) (*LocalLink, error) {
	e, err := m.entryFor(id)
	if err != nil {
		return nil, err
	}
	d, info := m.infoOf(e)
	gen, key, cur, ok := fwKey(d, info)
	if !ok {
		return nil, ErrNoLocalFW
	}
	l, err := m.FirmwareSource().Latest(ctx, gen, key)
	if err != nil {
		return nil, err
	}
	if cur != "" && firmware.Compare(l.Version, cur) <= 0 {
		return nil, ErrUpToDate
	}
	exp := time.Now().Add(LinkTTL)
	claims, _ := json.Marshal(linkClaims{gen, key, l.Version, exp.Unix()})
	sealed, err := m.store.Seal(linkContext, string(claims))
	if err != nil {
		return nil, err
	}
	token := strings.TrimRight(strings.NewReplacer("+", "-", "/", "_").Replace(sealed), "=")
	u := strings.TrimRight(base, "/") + "/fw/" + token + "/" + url.PathEscape(l.FileName())
	png, err := qrcode.Encode(u, qrcode.Medium, 320)
	if err != nil {
		return nil, err
	}
	return &LocalLink{
		URL: u, QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		Name: descName(d), Model: d.TypeName, Current: cur, Version: l.Version, Source: l.Source,
		FileName: l.FileName(), Expires: exp.UnixMilli(), Warning: baseWarning(base),
	}, nil
}

// baseWarning: a link on localhost cannot be opened from a phone.
func baseWarning(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "badBase"
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "localhost"
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return "localhost"
	}
	return ""
}

// FirmwareFile resolves a download link to the verified file (downloaded
// and checked on first use) and the name it is served under.
func (m *Devices) FirmwareFile(ctx context.Context, token string) (path, name string, err error) {
	sealed := strings.NewReplacer("-", "+", "_", "/").Replace(token)
	if n := len(sealed) % 4; n != 0 {
		sealed += strings.Repeat("=", 4-n)
	}
	raw, err := m.store.Unseal(linkContext, sealed)
	if err != nil {
		return "", "", ErrLinkExpired
	}
	var c linkClaims
	if json.Unmarshal([]byte(raw), &c) != nil || time.Now().Unix() > c.Exp {
		return "", "", ErrLinkExpired
	}
	x := m.FirmwareSource()
	l, err := x.Latest(ctx, c.Gen, c.Key)
	if err != nil {
		return "", "", err
	}
	if l.Version != c.Version { // the index moved on: make a new link
		return "", "", ErrLinkExpired
	}
	p, err := x.File(ctx, l)
	return p, l.FileName(), err
}
