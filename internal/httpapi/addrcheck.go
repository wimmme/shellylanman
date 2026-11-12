package httpapi

// Settings → Firmware: is it really ShellyLanMan that answers on the addresses for phones
// and devices (DECISIONS P32-1)? A wrong port is easy to miss (Home Assistant's 8123
// instead of the app's own): the wizard's QR code then opens Home Assistant's start page
// and the downloads fail. The check reads the saved settings, so it only ever asks the
// addresses the user set, and only for /healthz.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// instanceHeader carries a random id of this process on /healthz: the check knows it is
// itself that answered, not another ShellyLanMan or something else on that port.
const instanceHeader = "X-ShellyLanMan-Instance"

// addrTimeout: how long the check waits for an address.
var addrTimeout = 4 * time.Second

func newInstanceID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AddressCheck is what ShellyLanMan found on one address.
type AddressCheck struct {
	// Result: ok (this ShellyLanMan answers), other (something else answers, e.g. Home
	// Assistant), unreachable (no answer: phones or devices may still reach it).
	Result string `json:"result"`
	Error  string `json:"error,omitempty"` // with unreachable
	Port   int    `json:"port,omitempty"`  // ShellyLanMan's own port, with other and unreachable
}

// AddressChecks: the saved addresses, each only when it is set.
type AddressChecks struct {
	Phone  *AddressCheck `json:"phone,omitempty"`
	Device *AddressCheck `json:"device,omitempty"`
}

func (s *server) checkAddresses(w http.ResponseWriter, r *http.Request) {
	st := s.Store.Settings()
	var out AddressChecks
	if st.PhoneBaseURL != "" {
		out.Phone = s.checkAddress(r.Context(), st.PhoneBaseURL)
	}
	if st.Firmware.DeviceBaseURL != "" {
		out.Device = s.checkAddress(r.Context(), st.Firmware.DeviceBaseURL)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) checkAddress(ctx context.Context, base string) *AddressCheck {
	c := &AddressCheck{}
	if s.Port != nil {
		c.Port = s.Port()
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		c.Result, c.Error = "unreachable", "not an http address"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, addrTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/healthz", nil)
	if err != nil {
		c.Result, c.Error = "unreachable", err.Error()
		return c
	}
	// Redirects are followed, as a phone does: one that drops the path (a name sent on to
	// Home Assistant) ends at something else.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Result, c.Error = "unreachable", shortErr(err)
		return c
	}
	_ = resp.Body.Close()
	if resp.Header.Get(instanceHeader) == s.instance {
		return &AddressCheck{Result: "ok"}
	}
	c.Result = "other"
	return c
}

// shortErr: the cause without the URL Go puts before it (the page shows the address).
func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}
