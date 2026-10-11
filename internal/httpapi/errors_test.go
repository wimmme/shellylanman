package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Every code the API can answer with is in errorCodes (the published list), once.
func TestErrorCodesAreListed(t *testing.T) {
	listed := map[string]bool{}
	for _, c := range errorCodes {
		if listed[c.code] || c.desc == "" {
			t.Errorf("%q twice or without a description", c.code)
		}
		listed[c.code] = true
	}
	used := []string{CodeDeviceOffline, CodeDeviceError}
	for _, e := range errCodes {
		used = append(used, e.code)
	}
	for _, c := range statusCodes {
		used = append(used, c)
	}
	for _, c := range used {
		if !listed[c] {
			t.Errorf("code %q is not in errorCodes", c)
		}
	}
}

func TestErrorCode(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		want   string
	}{
		{fmt.Errorf("x: %w", service.ErrNotFound), 404, "device_not_found"},
		{fmt.Errorf("%w: name", service.ErrInvalid), 400, "invalid"},
		{service.ErrConfirm, 428, "confirm_required"},
		{shelly.ErrUnauthorized, 403, "device_unauthorized"},
		{&shelly.APIError{Code: -103, Message: "bad"}, 502, "device_error"},
		{fmt.Errorf("something else"), 500, "internal"},
		{fmt.Errorf("something else"), 418, "bad_request"},
		{fmt.Errorf("something else"), 504, "device_offline"},
	} {
		if got := errorCode(c.err, c.status); got != c.want {
			t.Errorf("%v (%d) = %q, want %q", c.err, c.status, got, c.want)
		}
	}
}

// The shape on the wire: {"error": {"code", "message"}}.
func TestErrorShape(t *testing.T) {
	srv, _ := newDeviceServer(t)
	for _, c := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"GET", "/api/v1/devices/nope", "", 404, "device_not_found"},
		{"GET", "/api/v1/nothing", "", 404, "not_found"},
		{"PUT", "/api/v1/settings", `{"bogus": 1}`, 400, "invalid_json"},
		{"PUT", "/api/v1/settings", `x`, 415, "unsupported_media_type"},
	} {
		hdr := jsonHdr
		if c.body == "x" {
			hdr = map[string]string{"Content-Type": "text/plain"}
		}
		r := do(t, c.method, srv.URL+c.path, c.body, hdr)
		var e errorBody
		decode(t, r, &e)
		if r.StatusCode != c.status || e.Error.Code != c.code || e.Error.Message == "" {
			t.Errorf("%s %s: %d %+v, want %d %s", c.method, c.path, r.StatusCode, e, c.status, c.code)
		}
		if r.StatusCode >= 400 && r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s: content type %q", c.path, r.Header.Get("Content-Type"))
		}
	}
}

// Codes are written as the constants of errors.go, never as strings elsewhere: a code that is not in
// errorCodes would not be in the published list.
func TestNoLiteralCodes(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	literal := regexp.MustCompile(`Code:\s*"|codeNote\("|"code: `)
	for _, f := range files {
		if f == "errors.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if literal.MatchString(line) {
				t.Errorf("%s:%d: a code as a string: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// A code outside the list never reaches a client.
func TestUnknownCodeNotSent(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCoded(rec, http.StatusConflict, apiError{Code: "made_up", Message: "x"})
	var e errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error.Code != CodeConflict {
		t.Fatalf("%s %v", rec.Body, err)
	}
}

// A value that does not pass names its field, so a client need not read the message.
func TestInvalidField(t *testing.T) {
	srv, _ := newDeviceServer(t)
	r := do(t, "POST", srv.URL+"/api/v1/profiles", `{"name": ""}`, jsonHdr)
	var e errorBody
	decode(t, r, &e)
	if r.StatusCode != 400 || e.Error.Code != CodeInvalid || e.Error.Field != "name" {
		t.Fatalf("%d %+v", r.StatusCode, e)
	}
	if !errors.Is(&service.FieldError{Field: "x"}, service.ErrInvalid) {
		t.Fatal("a FieldError is an ErrInvalid")
	}
}
