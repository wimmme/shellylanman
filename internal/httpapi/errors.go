package httpapi

import (
	"errors"
	"log/slog"
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
	Field      string `json:"field,omitempty"`      // with invalid: the value that does not pass
	RetryAfter int    `json:"retryAfter,omitempty"` // seconds, with too_many_tries and wrong_password
}

type errorBody struct {
	Error apiError `json:"error"`
}

// The codes (see errorCodes for their meaning).
const (
	CodeBadRequest           = "bad_request"
	CodeInvalidJSON          = "invalid_json"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeTooLarge             = "too_large"
	CodeInvalid              = "invalid"
	CodeLoginRequired        = "login_required"
	CodeWrongPassword        = "wrong_password"
	CodeTooManyTries         = "too_many_tries"
	CodePasswordTooShort     = "password_too_short"
	CodePasswordNoCapital    = "password_no_capital"
	CodeNoPassword           = "no_password"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeDeviceNotFound       = "device_not_found"
	CodeDeviceUnauthorized   = "device_unauthorized"
	CodeDeviceOffline        = "device_offline"
	CodeDeviceError          = "device_error"
	CodeDeviceRelayed        = "device_relayed"
	CodeUnsupported          = "unsupported"
	CodeScriptsUnsupported   = "scripts_unsupported"
	CodeConfirmRequired      = "confirm_required"
	CodeAllExcluded          = "all_excluded"
	CodeConflict             = "conflict"
	CodeBusy                 = "busy"
	CodeArchiveOff           = "archive_off"
	CodeNoCredentials        = "no_credentials"
	CodeProfileNotFound      = "profile_not_found"
	CodeSceneNotFound        = "scene_not_found"
	CodeSceneExists          = "scene_exists"
	CodeNotBLUGateway        = "not_blu_gateway"
	CodeRPCBlocked           = "rpc_blocked"
	CodeFirmwareNotFound     = "firmware_not_found"
	CodeUpToDate             = "up_to_date"
	CodeLinkExpired          = "link_expired"
	CodeUnavailable          = "unavailable"
	CodeInternal             = "internal"
)

// errorCodes: every code the API answers with, and what it means.
var errorCodes = []struct{ code, desc string }{
	{CodeBadRequest, "The request is wrong: an argument that is missing or does not fit."},
	{CodeInvalidJSON, "The body is not valid JSON or has an unknown field."},
	{CodeUnsupportedMediaType, "`Content-Type` must be `application/json`."},
	{CodeTooLarge, "The request body is too large."},
	{CodeInvalid, "A value does not pass validation; the message says which (`invalid: …`)."},
	{CodeLoginRequired, "A UI password is set and the request has neither a session nor a valid MCP token."},
	{CodeWrongPassword, "The UI password (or the current one, when changing it) is wrong."},
	{CodeTooManyTries, "Too many wrong passwords from this address: wait `retryAfter` seconds."},
	{CodePasswordTooShort, "The new UI password is shorter than 8 characters."},
	{CodePasswordNoCapital, "The new UI password has no capital letter."},
	{CodeNoPassword, "No UI password is set."},
	{CodeForbidden, "Not allowed, e.g. a browser request from another origin or a token with too little access."},
	{CodeNotFound, "No such endpoint or thing."},
	{CodeDeviceNotFound, "No such device."},
	{CodeDeviceUnauthorized, "The device asked for a password ShellyLanMan does not have, or has wrong."},
	{CodeDeviceOffline, "The device could not be reached."},
	{CodeDeviceError, "The device answered with an error."},
	{CodeDeviceRelayed, "A BLU device a gateway only relays: no settings, backup or control."},
	{CodeUnsupported, "The device does not support this."},
	{CodeScriptsUnsupported, "The device has no scripts or KVS."},
	{CodeConfirmRequired, "Repeat the request with `confirm: true`."},
	{CodeAllExcluded, "Every selected device was left out (not supported or excluded)."},
	{CodeConflict, "The request is understood but cannot be done now."},
	{CodeBusy, "The same job is already running."},
	{CodeArchiveOff, "Notes need the archive (Settings → Archive)."},
	{CodeNoCredentials, "No credentials are stored for this device."},
	{CodeProfileNotFound, "No such profile."},
	{CodeSceneNotFound, "No such scene."},
	{CodeSceneExists, "A scene with this name exists; ask to overwrite it."},
	{CodeNotBLUGateway, "The device cannot look for BLU devices."},
	{CodeRPCBlocked, "This RPC method is not available through ShellyLanMan."},
	{CodeFirmwareNotFound, "No firmware for this device in the index or the cache."},
	{CodeUpToDate, "The device already runs the latest stable firmware."},
	{CodeLinkExpired, "The download link has expired."},
	{CodeUnavailable, "The device service is not running."},
	{CodeInternal, "Something failed inside ShellyLanMan."},
}

// errCodes: errors of the layers below with a code of their own; the first match wins.
var errCodes = []struct {
	err  error
	code string
}{
	{service.ErrNotFound, CodeDeviceNotFound},
	{service.ErrConfirm, CodeConfirmRequired},
	{service.ErrInvalid, CodeInvalid},
	{service.ErrAllExcluded, CodeAllExcluded},
	{service.ErrRelayed, CodeDeviceRelayed},
	{service.ErrNoScripts, CodeScriptsUnsupported},
	{service.ErrBadCommand, CodeUnsupported},
	{service.ErrNoArchive, CodeArchiveOff},
	{service.ErrNoCredentials, CodeNoCredentials},
	{service.ErrNoProfile, CodeProfileNotFound},
	{service.ErrNoScene, CodeSceneNotFound},
	{service.ErrSceneExists, CodeSceneExists},
	{service.ErrIdentifyBusy, CodeBusy},
	{service.ErrNotBLUGateway, CodeNotBLUGateway},
	{service.ErrRPCBlocked, CodeRPCBlocked},
	{service.ErrNoLocalFW, CodeFirmwareNotFound},
	{firmware.ErrNotFound, CodeFirmwareNotFound},
	{service.ErrUpToDate, CodeUpToDate},
	{service.ErrLinkExpired, CodeLinkExpired},
	{service.ErrNoConnection, CodeDeviceOffline},
	{shelly.ErrUnauthorized, CodeDeviceUnauthorized},
	{auth.ErrWrong, CodeWrongPassword},
	{auth.ErrTooShort, CodePasswordTooShort},
	{auth.ErrNoCapital, CodePasswordNoCapital},
}

// statusCodes: the code of an error that has none of its own.
var statusCodes = map[int]string{
	http.StatusBadRequest:            CodeBadRequest,
	http.StatusUnauthorized:          CodeLoginRequired,
	http.StatusForbidden:             CodeForbidden,
	http.StatusNotFound:              CodeNotFound,
	http.StatusConflict:              CodeConflict,
	http.StatusRequestEntityTooLarge: CodeTooLarge,
	http.StatusUnsupportedMediaType:  CodeUnsupportedMediaType,
	http.StatusPreconditionRequired:  CodeConfirmRequired,
	http.StatusTooManyRequests:       CodeTooManyTries,
	http.StatusInternalServerError:   CodeInternal,
	http.StatusBadGateway:            CodeDeviceError,
	http.StatusServiceUnavailable:    CodeUnavailable,
	http.StatusGatewayTimeout:        CodeDeviceOffline,
}

func statusCode(status int) string {
	if c, ok := statusCodes[status]; ok {
		return c
	}
	if status >= 500 {
		return CodeInternal
	}
	return CodeBadRequest
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
		return CodeDeviceOffline
	case errors.As(err, &api):
		return CodeDeviceError
	}
	if status == http.StatusBadRequest { // a guess that 1.0 would freeze: find these before then
		slog.Debug("API error without a code of its own", "err", err)
	}
	return statusCode(status)
}

// writeError answers with msg and the code of the status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeCoded(w, status, apiError{Code: statusCode(status), Message: msg})
}

// writeErr answers with err and its code.
func writeErr(w http.ResponseWriter, status int, err error) {
	e := apiError{Code: errorCode(err, status), Message: err.Error()}
	var fe *service.FieldError
	if errors.As(err, &fe) {
		e.Field = fe.Field
	}
	writeCoded(w, status, e)
}

var knownCodes = func() map[string]bool {
	m := make(map[string]bool, len(errorCodes))
	for _, c := range errorCodes {
		m[c.code] = true
	}
	return m
}()

// writeCoded answers with e. A code outside errorCodes never reaches a client: it becomes the
// code of the status (TestNoLiteralCodes keeps such codes out of the source).
func writeCoded(w http.ResponseWriter, status int, e apiError) {
	if !knownCodes[e.Code] {
		slog.Warn("API error code not in the published list", "code", e.Code)
		e.Code = statusCode(status)
	}
	writeJSON(w, status, errorBody{Error: e})
}
