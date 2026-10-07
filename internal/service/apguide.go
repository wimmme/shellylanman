package service

// What the access-point wizards need to know (DECISIONS §29, docs/phase-20-ap-wizards.md):
// the device's own access point (its name, whether it is on and open), the model, and the
// two QR codes — to join the access point and to open the device's page — made on the
// server like the firmware link's.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/wimmme/shellylanman/internal/model"
)

// APPageURL is where a device's page is while the phone is on its access point.
const APPageURL = "http://192.168.33.1"

// APGuide is what the wizards show for one device or one access point name.
type APGuide struct {
	SSID       string `json:"ssid"`
	Assumed    bool   `json:"assumed,omitempty"` // the default name, not read from the device
	Gen        string `json:"gen,omitempty"`     // "1", or "2" for Gen2 and newer
	Key        string `json:"key,omitempty"`     // the firmware index's key: Gen1 type, Gen2+ app
	Model      string `json:"model,omitempty"`
	MAC        string `json:"mac,omitempty"`
	ID         string `json:"id,omitempty"`        // the device in the list, when it is there
	APEnabled  *bool  `json:"apEnabled,omitempty"` // read from the device; absent when it could not be read
	Open       *bool  `json:"open,omitempty"`      // Gen2+: the access point has no password
	Recognised bool   `json:"recognised"`          // the model is known
	JoinQR     string `json:"joinQR,omitempty"`    // data:image/png;base64,… — only for an open access point
	PageURL    string `json:"pageURL"`
	PageQR     string `json:"pageQR"`
}

// APGuide describes the access point of the device id, or of the access point
// called name (as a phone shows it; a bare MAC of a device in the list also works).
func (m *Devices) APGuide(ctx context.Context, id, name string) (*APGuide, error) {
	name = strings.TrimSpace(name)
	if id == "" && name != "" {
		if mac := model.NormalizeMAC(name); len(mac) == 12 {
			if _, err := m.entryFor(mac); err == nil {
				id = mac
			}
		}
	}
	g := &APGuide{PageURL: APPageURL}
	switch {
	case id != "":
		if err := m.apGuideDevice(ctx, g, id); err != nil {
			return nil, err
		}
	case name != "":
		g.SSID = name
		if p, ok := model.ParseAPName(name); ok {
			g.Gen, g.Key, g.Model, g.MAC, g.Recognised = p.Gen, p.Key, p.Name, p.MAC, true
			if g.Model == "" {
				g.Model = p.Key
			}
		} else {
			g.MAC = model.MACFromHostname(name) // the name still ends in a MAC, though the model is unknown
		}
		if g.MAC != "" {
			if _, err := m.entryFor(g.MAC); err == nil {
				g.ID = g.MAC
			}
		}
	default:
		return nil, invalid("the name of the access point, or a device")
	}
	if err := g.codes(); err != nil {
		return nil, err
	}
	return g, nil
}

// apGuideDevice fills the guide from a device in the list: the access point is read from
// the device when it can be reached, else its default name is assumed.
func (m *Devices) apGuideDevice(ctx context.Context, g *APGuide, id string) error {
	e, err := m.entryFor(id)
	if err != nil {
		return err
	}
	d, info := m.infoOf(e)
	if !d.Managed || d.Gen == model.GenBLU || d.Gen == model.GenBTHome {
		return fmt.Errorf("%w: this device has no access point of its own", ErrBadCommand)
	}
	g.ID, g.MAC, g.Model = d.ID, d.ID, d.TypeName
	if gen, key, _, ok := fwKey(d, info); ok {
		g.Gen, g.Key, g.Recognised = gen, key, true
	}
	app := info.App
	if app == "" && (d.Gen == "2" || d.Gen == "3") {
		app = d.TypeID
	}
	g.SSID = model.DefaultAPName(d.Gen, d.Hostname, app, d.ID)
	g.Assumed = true
	if d.Status != model.StatusOnline {
		return nil
	}
	if d.Gen == "1" {
		raw, err := m.DeviceGet(ctx, id, "/settings")
		if err != nil {
			return nil
		}
		var s struct {
			AP struct {
				SSID    string `json:"ssid"`
				Enabled bool   `json:"enabled"`
			} `json:"wifi_ap"`
		}
		if json.Unmarshal(raw, &s) == nil && s.AP.SSID != "" {
			g.SSID, g.Assumed, g.APEnabled = s.AP.SSID, false, &s.AP.Enabled
		}
		return nil
	}
	raw, err := m.DeviceRPC(ctx, id, "WiFi.GetConfig", nil)
	if err != nil {
		return nil
	}
	var c struct {
		AP struct {
			SSID   string `json:"ssid"`
			Enable bool   `json:"enable"`
			Open   bool   `json:"is_open"`
		} `json:"ap"`
	}
	if json.Unmarshal(raw, &c) == nil && c.AP.SSID != "" {
		g.SSID, g.Assumed, g.APEnabled, g.Open = c.AP.SSID, false, &c.AP.Enable, &c.AP.Open
	}
	return nil
}

// codes makes the two QR codes.
func (g *APGuide) codes() error {
	png, err := qrcode.Encode(g.PageURL, qrcode.Medium, 320)
	if err != nil {
		return err
	}
	g.PageQR = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	if g.SSID != "" && (g.Open == nil || *g.Open) {
		png, err := qrcode.Encode(WiFiQR(g.SSID), qrcode.Medium, 320)
		if err != nil {
			return err
		}
		g.JoinQR = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	return nil
}

// WiFiQR is the text of the QR code that makes a phone join an open network.
func WiFiQR(ssid string) string {
	return "WIFI:T:nopass;S:" + strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, `:`, `\:`, `"`, `\"`).Replace(ssid) + ";;"
}
