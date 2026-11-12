package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wimmme/shellylanman/internal/store"
)

func checkAddrs(t *testing.T, srv *httptest.Server) AddressChecks {
	t.Helper()
	var out AddressChecks
	decode(t, do(t, "GET", srv.URL+"/api/v1/settings/check-addresses", "", nil), &out)
	return out
}

// P32-1: the saved address for phones or devices is checked: is it this ShellyLanMan that
// answers there, something else (Home Assistant's port), or nothing?
func TestCheckAddresses(t *testing.T) {
	srv, st := newTestServer(t, nil)
	if got := checkAddrs(t, srv); got.Phone != nil || got.Device != nil {
		t.Fatalf("nothing set, nothing checked: %+v", got)
	}

	// Itself.
	st.Update(func(s *store.Settings) { s.PhoneBaseURL = srv.URL; s.Firmware.DeviceBaseURL = srv.URL })
	got := checkAddrs(t, srv)
	if got.Phone == nil || got.Phone.Result != "ok" || got.Device == nil || got.Device.Result != "ok" {
		t.Fatalf("own address not ok: %+v %+v", got.Phone, got.Device)
	}

	// Something else answers (Home Assistant on 8123), and another ShellyLanMan.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Redirect(w, r, "/", http.StatusFound) // a name sent on without the path
		}
	}))
	defer other.Close()
	twin, _ := newTestServer(t, nil)
	st.Update(func(s *store.Settings) { s.PhoneBaseURL = other.URL; s.Firmware.DeviceBaseURL = twin.URL })
	got = checkAddrs(t, srv)
	if got.Phone == nil || got.Phone.Result != "other" {
		t.Fatalf("another server is not 'other': %+v", got.Phone)
	}
	if got.Device == nil || got.Device.Result != "other" {
		t.Fatalf("another ShellyLanMan is not 'other': %+v", got.Device)
	}

	// Nothing listens.
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	st.Update(func(s *store.Settings) { s.PhoneBaseURL = gone.URL; s.Firmware.DeviceBaseURL = "" })
	got = checkAddrs(t, srv)
	if got.Phone == nil || got.Phone.Result != "unreachable" || got.Phone.Error == "" || got.Device != nil {
		t.Fatalf("closed port: %+v %+v", got.Phone, got.Device)
	}
}
