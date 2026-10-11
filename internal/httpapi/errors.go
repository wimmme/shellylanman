package httpapi

import (
	"errors"
	"net/http"

	"github.com/wimmme/shellylanman/internal/auth"
	"github.com/wimmme/shellylanman/internal/firmware"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
)

// Errors of the REST API (DECISIONS P22-2): {"error": {"code": "device_offline", "message": "…"}}.
// The code is for programs (the Home Assistant integration, other OpenAPI clients) and part of the
// API's promise; the message is for people and may change. errorCodes is the complete list; the
// OpenAPI description publishes it and a test keeps it complete.

type apiError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retryAfter,omitempty"` // seconds, with too_many_tries and wrong_password
}

type errorBody struct {
	Error apiError `json:"error"`
}

// errorCodes: every code the API answers with, and what it means.
var errorCodes = []struct{ code, desc string }{
	{"bad_request", "The request is wrong: an argument that is missing or does not fit."},
	{"invalid_json", "The body is not valid JSON or has an unknown field."},
	{"unsupported_media_type", "`Content-Type` must be `application/json`."},
	{"too_large", "The request body is too large."},
	{"invalid", "A value does not pass validation; the message says which (`invalid: …`)."},
	{"login_required", "A UI password is set and the request has neither a session nor a valid MCP token."},
	{"wrong_password", "The UI password (or the current one, when changing it) is wrong."},
	{"too_many_tries", "Too many wrong passwords from this address: wait `retryAfter` seconds."},
	{"password_too_short", "The new UI password is shorter than 8 characters."},
	{"password_no_capital", "The new UI password has no capital letter."},
	{"no_password", "No UI password is set."},
	{"forbidden", "Not allowed, e.g. a browser request from another origin or a token with too little access."},
	{"not_found", "No such endpoint or thing."},
	{"device_not_found", "No such device."},
	{"device_unauthorized", "The device asked for a password ShellyLanMan does not have, or has wrong."},
	{"device_offline", "The device could not be reached."},
	{"device_error", "The device answered with an error."},
	{"device_relayed", "A BLU device a gateway only relays: no settings, backup or control."},
	{"unsupported", "The device does not support this."},
	{"scripts_unsupported", "The device has no scripts or KVS."},
	{"confirm_required", "Repeat the request with `confirm: true`."},
	{"all_excluded", "Every selected device was left out (not supported or excluded)."},
	{"conflict", "The request is understood but cannot be done now."},
	{"busy", "The same job is already running."},
	{"archive_off", "Notes need the archive (Settings → Archive)."},
	{"no_credentials", "No credentials are stored for this device."},
	{"profile_not_found", "No such profile."},
	{"scene_not_found", "No such scene."},
	{"scene_exists", "A scene with this name exists; ask to overwrite it."},
	{"not_blu_gateway", "The device cannot look for BLU devices."},
	{"rpc_blocked", "This RPC method is not available through ShellyLanMan."},
	{"firmware_not_found", "No firmware for this device in the index or the cache."},
	{"up_to_date", "The device already runs the latest stable firmware."},
	{"link_expired", "The download link has expired."},
	{"unavailable", "The device service is not running."},
	{"internal", "Something failed inside ShellyLanMan."},
}

// errCodes: errors of the layers below with a code of their own; the first match wins.
var errCodes = []struct {
	err  error
	code string
}{
	{service.ErrNotFound, "device_not_found"},
	{service.ErrConfirm, "confirm_required"},
	{service.ErrInvalid, "invalid"},
	{service.ErrAllExcluded, "all_excluded"},
	{service.ErrRelayed, "device_relayed"},
	{service.ErrNoScripts, "scripts_unsupported"},
	{service.ErrBadCommand, "unsupported"},
	{service.ErrNoArchive, "archive_off"},
	{service.ErrNoCredentials, "no_credentials"},
	{service.ErrNoProfile, "profile_not_found"},
	{service.ErrNoScene, "scene_not_found"},
	{service.ErrSceneExists, "scene_exists"},
	{service.ErrIdentifyBusy, "busy"},
	{service.ErrNotBLUGateway, "not_blu_gateway"},
	{service.ErrRPCBlocked, "rpc_blocked"},
	{service.ErrNoLocalFW, "firmware_not_found"},
	{firmware.ErrNotFound, "firmware_not_found"},
	{service.ErrUpToDate, "up_to_date"},
	{service.ErrLinkExpired, "link_expired"},
	{service.ErrNoConnection, "device_offline"},
	{shelly.ErrUnauthorized, "device_unauthorized"},
	{auth.ErrWrong, "wrong_password"},
	{auth.ErrTooShort, "password_too_short"},
	{auth.ErrNoCapital, "password_no_capital"},
}

// statusCodes: the code of an error that has none of its own.
var statusCodes = map[int]string{
	http.StatusBadRequest:            "bad_request",
	http.StatusUnauthorized:          "login_required",
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not_found",
	http.StatusConflict:              "conflict",
	http.StatusRequestEntityTooLarge: "too_large",
	http.StatusUnsupportedMediaType:  "unsupported_media_type",
	http.StatusPreconditionRequired:  "confirm_required",
	http.StatusTooManyRequests:       "too_many_tries",
	http.StatusInternalServerError:   "internal",
	http.StatusBadGateway:            "device_error",
	http.StatusServiceUnavailable:    "unavailable",
	http.StatusGatewayTimeout:        "device_offline",
}

func statusCode(status int) string {
	if c, ok := statusCodes[status]; ok {
		return c
	}
	if status >= 500 {
		return "internal"
	}
	return "bad_request"
}

// errorCode: the code of err, else the one of the HTTP status it is answered with.
func errorCode(err error, status int) string {
	for _, e := range errCodes {
		if errors.Is(err, e.err) {
			return e.code
		}
	}
	var api *shelly.APIError
	switch {
	case shelly.IsOffline(err):
		return "device_offline"
	case errors.As(err, &api):
		return "device_error"
	}
	return statusCode(status)
}

// writeError answers with msg and the code of the status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeCoded(w, status, apiError{Code: statusCode(status), Message: msg})
}

// writeErr answers with err and its code.
func writeErr(w http.ResponseWriter, status int, err error) {
	writeCoded(w, status, apiError{Code: errorCode(err, status), Message: err.Error()})
}

func writeCoded(w http.ResponseWriter, status int, e apiError) {
	writeJSON(w, status, errorBody{Error: e})
}
