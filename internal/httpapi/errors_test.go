package httpapi

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	used := []string{"invalid_json", "no_password", "too_many_tries", "wrong_password", "device_offline", "device_error"}
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

// The texts of the OpenAPI description name only codes that exist ("`code: wrong_password`", "`password_no_capital`").
func TestOpenAPINamesKnownCodes(t *testing.T) {
	b, err := json.Marshal(buildSpec())
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, c := range errorCodes {
		listed[c.code] = true
	}
	bq := "`" // the codes are quoted with backticks: (`code: a`, `b`)
	codeRef := regexp.MustCompile(`\(` + bq + `code: ([A-Za-z_]+)` + bq + `(?:, ` + bq + `([A-Za-z_]+)` + bq + `)*\)`)
	for _, m := range codeRef.FindAllStringSubmatch(string(b), -1) {
		for _, c := range m[1:] {
			if c != "" && !listed[c] {
				t.Errorf("the OpenAPI description names an unknown code %q", c)
			}
		}
	}
}
