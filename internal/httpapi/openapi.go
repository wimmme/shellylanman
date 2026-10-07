package httpapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wimmme/shellylanman/internal/discovery"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/service"
	"github.com/wimmme/shellylanman/internal/shelly"
	"github.com/wimmme/shellylanman/internal/store"
	"github.com/wimmme/shellylanman/internal/update"
	"github.com/wimmme/shellylanman/internal/version"
)

// The OpenAPI 3.1 description of the REST API (DECISIONS P19-2): the table of
// operations below (words) plus schemas made from the Go types the handlers
// use (openapi_schema.go). A test compares the table with the routes the
// server registers, in both directions: a route without an entry, or an entry
// without a route, fails the build.

type paramSpec struct {
	name     string
	typ      string // string, integer, boolean
	desc     string
	required bool
}

type respSpec struct {
	code int
	desc string
	body any    // value of the response's type; nil: no body
	text string // media type of a non-JSON body (text/plain, application/zip, …)
}

type errSpec struct {
	code int
	desc string // empty: the standard text of the code
}

const (
	authDefault     = iota // a session or the MCP token, when a UI password is set
	authNone               // open
	authCredentials        // the MCP token at access level "configure"
	authMCP                // the MCP token
)

type operation struct {
	method, path, id, tag, summary, desc string

	query    []paramSpec
	body     any // value of the request type
	bodyDesc string
	bodyOpt  bool // the body may be left out
	ok       respSpec
	errs     []errSpec
	auth     int
}

func e(codes ...int) []errSpec {
	var out []errSpec
	for _, c := range codes {
		out = append(out, errSpec{code: c})
	}
	return out
}

func more(a []errSpec, b ...errSpec) []errSpec { return append(append([]errSpec(nil), a...), b...) }

var (
	idsQ     = paramSpec{name: "ids", typ: "string", desc: "Device ids, comma separated. Absent or empty: all devices."}
	idsQReq  = paramSpec{name: "ids", typ: "string", desc: "Device ids, comma separated.", required: true}
	eDevice  = e(400, 403, 404, 502, 504) // an operation that talks to a device
	eConfirm = errSpec{code: 428}
)

func okJSON(code int, desc string, v any) respSpec { return respSpec{code: code, desc: desc, body: v} }
func okText(code int, desc, media string) respSpec {
	return respSpec{code: code, desc: desc, text: media}
}
func done(desc string) respSpec     { return respSpec{code: 204, desc: desc} }
func accepted(desc string) respSpec { return respSpec{code: 202, desc: desc} }

type results struct {
	Results []service.ResultLine `json:"results"`
}

type checklistResult struct {
	Errors []service.ResultLine   `json:"errors"`
	Rows   []service.ChecklistRow `json:"rows"`
}

type resultBody struct {
	Result any `json:"result"`
}

type codeBody struct {
	Code string `json:"code"`
}

type healthBody struct {
	Status string `json:"status"`
}

type authEnabledBody struct {
	AuthEnabled bool `json:"authEnabled"`
}

type errorBody struct {
	Error string `json:"error"`
}

// configApplyBody: `ids` plus the fields of the section's apply type.
type configApplyBody struct {
	IDs []string `json:"ids"`
}

var operations = []operation{
	// ---- Status and settings ----
	{method: "GET", path: "/healthz", id: "health", tag: "Status", summary: "Health check",
		desc: "Answers `{\"status\":\"ok\"}` while the server runs. Open, says nothing about the version; for container health checks.",
		ok:   okJSON(200, "The server runs.", healthBody{}), auth: authNone},
	{method: "GET", path: "/api/v1/status", id: "getStatus", tag: "Status", summary: "What the UI needs first",
		desc: "Whether a UI password is set, whether the caller is logged in, and (when it is) whether the first-run dialog is done and how many browsers are connected. Open: this is how a client finds out that it has to log in.",
		ok:   okJSON(200, "The state.", Status{}), auth: authNone},
	{method: "GET", path: "/api/v1/settings", id: "getSettings", tag: "Status", summary: "Read the settings",
		ok: okJSON(200, "ShellyLanMan's settings.", store.Settings{})},
	{method: "PUT", path: "/api/v1/settings", id: "putSettings", tag: "Status", summary: "Change settings",
		desc: "A partial update: only the fields present change. A changed scan mode or archive setting starts a rescan. Sends a `settings.changed` event.",
		body: settingsPatch{},
		ok:   okJSON(200, "The settings after the change.", store.Settings{}),
		errs: e(400)},
	{method: "GET", path: "/api/v1/update", id: "getUpdate", tag: "Status", summary: "Last release check",
		desc: "The result of the check for a newer ShellyLanMan release. `mode` is `never` while the check is off (the default).",
		ok:   okJSON(200, "The last check.", update.Status{})},
	{method: "GET", path: "/api/v1/server", id: "getServer", tag: "Status", summary: "Ports in use",
		desc: "Where ShellyLanMan listens and where that is set. The port itself is set outside ShellyLanMan (`SHELLYLANMAN_PORT`).",
		ok:   okJSON(200, "The ports.", ServerInfo{})},
	{method: "GET", path: "/api/v1/about", id: "getAbout", tag: "Status", summary: "About: version, runtime, credits",
		ok: okJSON(200, "Version, uptime, runtime, dependencies and credits.", About{})},
	{method: "GET", path: "/api/v1/about/changelog", id: "getChangelog", tag: "Status", summary: "Release notes",
		ok: okText(200, "The CHANGELOG, Markdown.", "text/plain")},
	{method: "GET", path: "/api/v1/about/license", id: "getLicense", tag: "Status", summary: "The licence",
		ok: okText(200, "The licence text.", "text/plain")},
	{method: "GET", path: "/api/v1/about/notices", id: "getNotices", tag: "Status", summary: "Third-party notices",
		ok: okText(200, "THIRD_PARTY_NOTICES.", "text/plain")},
	{method: "GET", path: "/api/v1/log", id: "getLog", tag: "Status", summary: "ShellyLanMan's own log",
		desc:  "The last 1000 log lines of this run (information, warnings, errors), oldest first. In memory only. New lines arrive as `log.entry` events on `/ws`.",
		query: []paramSpec{{name: "after", typ: "integer", desc: "Only lines with a sequence number above this."}},
		ok:    okJSON(200, "The lines.", LogResponse{}), errs: e(400)},
	{method: "GET", path: "/api/v1/openapi.json", id: "getOpenAPI", tag: "Status", summary: "This description",
		desc: "The OpenAPI 3.1 description of this API, made by this server. `servers` is the address the request came to.",
		ok:   okText(200, "The OpenAPI document.", "application/json")},

	// ---- Events ----
	{method: "GET", path: "/ws", id: "events", tag: "Events", summary: "Server events (WebSocket)",
		desc: "A WebSocket that only sends: server → client, JSON text messages `{\"type\":…,\"data\":…}`; whatever the client sends is ignored. Everything a client can do goes through the REST API. The first message is `hello`. " +
			"The number of connected clients sets how often the devices are polled: with none, ShellyLanMan polls slowly.\n\n" +
			"| type | data |\n|---|---|\n" +
			"| `hello` | `{version}` |\n| `settings.changed` | Settings |\n| `device.upsert` | Device |\n| `device.removed` | `{id}` |\n| `devices.reset` | none: reload the list |\n" +
			"| `scan.state` | ScanState |\n| `update.status` | UpdateStatus |\n| `firmware.row` | FirmwareRow (progress of an update) |\n| `deferred.changed` | the deferred tasks changed |\n" +
			"| `blu.identify` | `{state: started\\|done\\|error, gateway, duration, found, error}` |\n| `blu.discovered` | a BLU device that answered the active scan |\n| `log.entry` | LogEntry (one new log line) |",
		ok: respSpec{code: 101, desc: "Switching protocols: the WebSocket is open."}, errs: e(403)},
	{method: "GET", path: "/ws/log/{id}", id: "deviceLogStream", tag: "Log", summary: "A device's live log (WebSocket)",
		desc: "Relays the live debug log of a Gen2+ device (the device's own `/debug/log` WebSocket); for a BLU device, its gateway's. Each message is the device's JSON line (`ts`, `level`, `fd`, `data`); an `{\"error\":…}` message and a close follow when the device log is unavailable. The log must be switched on in the device (Checklist → Logs).",
		ok:   respSpec{code: 101, desc: "Switching protocols: the WebSocket is open."}, errs: e(403, 404)},

	// ---- Authentication ----
	{method: "POST", path: "/api/v1/auth/login", id: "login", tag: "Authentication", summary: "Log in",
		desc: "Only when a UI password is set. Sets the session cookie `slm_session`. After 5 wrong passwords from one address the answers wait 1 s, 2 s, 4 s … up to 60 s (`429`, `Retry-After`). Programs do not need this: they send the MCP token as a bearer token.",
		body: loginBody{}, ok: done("Logged in; the session cookie is set."), auth: authNone,
		errs: []errSpec{{401, "Wrong password (`code: wrong`)."}, {409, "No password is set."}, {429, "Too many wrong passwords: wait `retryAfter` seconds."}}},
	{method: "POST", path: "/api/v1/auth/logout", id: "logout", tag: "Authentication", summary: "Log out",
		ok: done("The session ended.")},
	{method: "PUT", path: "/api/v1/auth/password", id: "setPassword", tag: "Authentication", summary: "Set, change or remove the UI password",
		desc: "An empty `password` removes it. Needs `current` when a password is set (not under Home Assistant ingress). Every session ends; the caller gets a new one. A password has at least 8 characters and one capital letter.",
		body: passwordBody{}, ok: okJSON(200, "Whether a password is set now.", authEnabledBody{}),
		errs: []errSpec{{400, "The password is too short or has no capital letter (`code: tooShort`, `noCapital`)."}, {401, "`current` is wrong."}, {429, "Too many wrong passwords."}}},

	// ---- MCP ----
	{method: "GET", path: "/api/v1/mcp", id: "getMCP", tag: "MCP", summary: "MCP server settings",
		ok: okJSON(200, "Whether the MCP server is on, its access level and whether it has a token (the token itself only when it was just made).", MCPInfo{})},
	{method: "PUT", path: "/api/v1/mcp", id: "putMCP", tag: "MCP", summary: "Change the MCP server settings",
		desc: "`access` is `read`, `control` or `configure`. Switching on without a token makes one.",
		body: mcpPatchBody{}, ok: okJSON(200, "The settings after the change.", MCPInfo{}), errs: e(400)},
	{method: "POST", path: "/api/v1/mcp/token", id: "newMCPToken", tag: "MCP", summary: "Make a new MCP token",
		desc: "The old token stops working at once. The new one is shown once, in the answer. Send `{}`.",
		body: struct{}{}, ok: okJSON(200, "The settings with the new token.", MCPInfo{})},
	{method: "POST", path: "/mcp", id: "mcp", tag: "MCP", summary: "The MCP endpoint (Model Context Protocol)",
		desc: "A Model Context Protocol server (streamable HTTP, JSON-RPC) for AI assistants: list devices, read, and with the right access level control and configure them. Not part of the REST API: clients speak MCP. Needs `Authorization: Bearer <MCP token>`. `GET` and `DELETE` are part of the MCP transport.",
		ok:   respSpec{code: 200, desc: "A JSON-RPC answer."}, auth: authMCP, errs: e(401, 503)},

	{method: "GET", path: "/mcp", id: "mcpStream", tag: "MCP", summary: "The MCP endpoint: server-to-client stream",
		desc: "Part of the MCP transport (see `POST /mcp`). Needs the MCP token.",
		ok:   respSpec{code: 200, desc: "An event stream, or `405` when the server does not offer one."}, auth: authMCP, errs: e(401, 503)},
	{method: "DELETE", path: "/mcp", id: "mcpClose", tag: "MCP", summary: "The MCP endpoint: end a session",
		desc: "Part of the MCP transport (see `POST /mcp`). Needs the MCP token.",
		ok:   respSpec{code: 200, desc: "The session ended."}, auth: authMCP, errs: e(401, 503)},

	// ---- Devices ----
	{method: "GET", path: "/api/v1/devices", id: "listDevices", tag: "Devices", summary: "List the devices",
		desc:  "Every device ShellyLanMan knows: found on the network, archived, and BLU devices behind their gateways. Kept current by `device.upsert` events on `/ws`.",
		query: []paramSpec{{name: "gen", typ: "string", desc: "Only this generation: `1`, `2`, `3`, `4`, `blu` or `bth`."}},
		ok:    okJSON(200, "The devices.", []model.Device{}), errs: e(503)},
	{method: "GET", path: "/api/v1/devices/{id}", id: "getDevice", tag: "Devices", summary: "One device",
		ok: okJSON(200, "The device.", model.Device{}), errs: e(404, 503)},
	{method: "DELETE", path: "/api/v1/devices/{id}", id: "removeDevice", tag: "Devices", summary: "Remove an archived device",
		desc: "Removes a stored (`ghost`) device from the list and the archive. Other devices cannot be removed.",
		ok:   done("Removed."), errs: []errSpec{{404, ""}, {409, "The device is not a stored one."}, {503, ""}}},
	{method: "POST", path: "/api/v1/devices/refresh", id: "refreshDevices", tag: "Devices", summary: "Read the devices' status again",
		desc: "The body may be left out: then all devices.",
		body: idsBody{}, bodyOpt: true, ok: accepted("Started; the results arrive as events."), errs: e(503)},
	{method: "POST", path: "/api/v1/devices/{id}/reload", id: "reloadDevice", tag: "Devices", summary: "Reload one device",
		desc: "Reads the device's full configuration again (Reload).",
		ok:   accepted("Started."), errs: e(404, 503)},
	{method: "POST", path: "/api/v1/devices/reboot", id: "rebootDevices", tag: "Devices", summary: "Reboot devices",
		desc: "Destructive: needs `confirm: true`.",
		body: idsConfirmBody{}, ok: accepted("The devices are rebooting."), errs: more(e(400, 404, 503), eConfirm)},
	{method: "POST", path: "/api/v1/devices/{id}/command", id: "deviceCommand", tag: "Control", summary: "Switch, dim, move: one action of the Command column",
		desc: "Controls one part (`Module.key`) of a device: relay, light, cover, thermostat, camera, circuit breaker, input events. Which actions apply to which kind is in `Command.action`.",
		body: service.Command{}, ok: done("Done."),
		errs: more(eDevice, errSpec{428, "Toggling a circuit breaker needs `confirm: true`."}, errSpec{409, "The device does not support the command."})},
	{method: "PUT", path: "/api/v1/devices/{id}/note", id: "putNote", tag: "Devices", summary: "Note and keyword (archive)",
		desc: "Notes live in the archive: they need the archive setting. `name` renames a relayed BLU device (it has no name of its own).",
		body: noteBody{}, ok: done("Saved."), errs: []errSpec{{404, ""}, {409, "The archive is off."}, {503, ""}}},
	{method: "PUT", path: "/api/v1/devices/{id}/pause", id: "pauseDevice", tag: "Devices", summary: "Pause refreshing a device",
		desc: "Used while the live log of a device is open, so that polling does not disturb it.",
		body: pauseBody{}, ok: done("Done."), errs: e(404, 503)},
	{method: "GET", path: "/api/v1/devices/{id}/info", id: "deviceInfoList", tag: "Devices", summary: "What the device information dialog asks the device",
		ok: okJSON(200, "The requests (tabs) of the dialog.", []service.InfoRequest{}), errs: e(404, 503)},
	{method: "GET", path: "/api/v1/devices/{id}/info/{index}", id: "deviceInfo", tag: "Devices", summary: "One answer of the device information dialog",
		ok: okJSON(200, "What the device answered.", service.InfoResult{}), errs: more(eDevice, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/devices/{id}/log", id: "deviceLogSnapshot", tag: "Log", summary: "A Gen1 device's log file",
		desc:  "`/debug/log` or `/debug/log1` of a Gen1 device, as text.",
		query: []paramSpec{{name: "file", typ: "integer", desc: "0 for `debug/log`, 1 for `debug/log1`."}},
		ok:    okText(200, "The log.", "text/plain"), errs: more(eDevice, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/rpc", id: "deviceRPC", tag: "Control", summary: "Call any RPC method on a Gen2+ device",
		desc: "Sends one RPC call to a Gen2+ device (for a BLU TRV, through its gateway); the scheduler's calls and its *test method* button. Methods that read (`Get*`, `List*`, `Check*`) and ordinary changes pass. Methods that restart, update, delete, run or replace code, set credentials or replace a settings block (`Shelly.Reboot`, `Shelly.Update`, `Script.Eval`, `Sys.SetConfig`, any `*.Delete` except `Schedule.Delete`, …) need `confirm: true`. A factory reset, a Wi-Fi reset and the delete-all methods (`Shelly.FactoryReset`, `Shelly.ResetWiFiConfig`, `Schedule.DeleteAll`, …) are refused with `400`. Needs a session or an MCP token with access level `control` or higher.",
		body: rpcBody{}, ok: okJSON(200, "The device's answer.", resultBody{}), errs: more(e(403, 404, 502, 504), errSpec{409, "Not a Gen2+ device, or not connected."}, errSpec{400, "Invalid request, or a method that is refused here (a factory reset and the like)."}, eConfirm, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/devices/{id}/schedule/hints", id: "scheduleHints", tag: "Scheduler", summary: "Methods a schedule can call",
		desc: "The RPC methods a scheduler entry on this device can call, with example parameters (the scheduler dialog's suggestions).",
		ok:   okJSON(200, "The hints.", []service.MethodHint{}), errs: more(eDevice, errSpec{409, "Not a Gen2+ device."}, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/devices/{id}/emdata", id: "energyHistory", tag: "Charts", summary: "Energy history of an energy meter (Gen2+)",
		desc:  "Watt-hours per time step from `EMData` / `EM1Data`, for a Pro EM or Pro 3EM.",
		query: []paramSpec{{name: "start", typ: "integer", desc: "Start, Unix seconds.", required: true}, {name: "end", typ: "integer", desc: "End, Unix seconds, after `start`.", required: true}},
		ok:    okJSON(200, "One series per meter.", []service.EMSeries{}), errs: more(eDevice, errSpec{409, "The device has no energy data."}, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/samples", id: "getSamples", tag: "Charts", summary: "The chart samples",
		desc:  "The last 24 hours of readings kept in memory (one sample per device refresh), per device id.",
		query: []paramSpec{idsQ, {name: "since", typ: "integer", desc: "Only samples after this, Unix milliseconds."}},
		ok:    okJSON(200, "Samples per device id.", map[string][]service.Sample{}), errs: e(503)},
	{method: "DELETE", path: "/api/v1/samples", id: "clearSamples", tag: "Charts", summary: "Clear chart samples",
		query: []paramSpec{idsQ}, ok: done("Cleared."), errs: e(503)},

	// ---- Discovery ----
	{method: "GET", path: "/api/v1/scan", id: "getScan", tag: "Discovery", summary: "State of the network scan",
		ok: okJSON(200, "The scan state.", service.ScanState{}), errs: e(503)},
	{method: "POST", path: "/api/v1/scan", id: "rescan", tag: "Discovery", summary: "Search the network again",
		desc: "Known devices stay listed as `searching`, are probed at their last address, and become archived (or leave the list) when the search ends.",
		ok:   accepted("Started."), errs: e(503)},
	{method: "DELETE", path: "/api/v1/archive", id: "clearArchive", tag: "Discovery", summary: "Clear the archive",
		desc: "Forgets the archived devices, with their notes and keywords.",
		ok:   done("Cleared."), errs: []errSpec{{500, "The archive could not be written."}, {503, ""}}},
	{method: "GET", path: "/api/v1/network/interfaces", id: "networkInterfaces", tag: "Discovery", summary: "Network interfaces of the server",
		desc: "For the scan setting *Local mDNS scan*.",
		ok:   okJSON(200, "The interfaces.", []discovery.Interface{}), errs: []errSpec{{500, ""}}},
	{method: "GET", path: "/api/v1/blu/gateways", id: "bluGateways", tag: "BLU", summary: "Gateways that can identify BLU devices",
		ok: okJSON(200, "The gateways.", []model.Device{}), errs: e(503)},
	{method: "POST", path: "/api/v1/blu/identify", id: "bluIdentify", tag: "BLU", summary: "Identify BLU devices",
		desc: "A gateway listens while you put the BLU device in pairing mode (`BTHome.StartDeviceDiscovery`); what answers arrives as `blu.discovered` events and the run as `blu.identify` events. Nothing changes on the gateway or the device.",
		body: identifyBLUBody{}, ok: accepted("Started."),
		errs: []errSpec{{400, ""}, {404, "No such gateway."}, {409, "An identification is already running."}, {503, ""}}},

	// ---- Credentials ----
	{method: "GET", path: "/api/v1/credentials", id: "getCredentials", tag: "Credentials", summary: "The shared device login",
		desc: "Whether a login is set that is tried on every protected device. Passwords are write-only: never returned.",
		ok:   okJSON(200, "Whether and for which user.", service.CredentialsInfo{}), errs: e(503)},
	{method: "PUT", path: "/api/v1/credentials", id: "putCredentials", tag: "Credentials", summary: "Set the shared device login",
		desc: "Stored encrypted. An empty password removes it.",
		body: credentialsBody{}, ok: okJSON(200, "Whether and for which user.", service.CredentialsInfo{}), errs: []errSpec{{500, ""}, {503, ""}}},
	{method: "PUT", path: "/api/v1/devices/{id}/credentials", id: "putDeviceCredentials", tag: "Credentials", summary: "Set the login of one device",
		body: credentialsBody{}, ok: done("Stored."), errs: []errSpec{{404, ""}, {500, ""}, {503, ""}}},
	{method: "GET", path: "/api/v1/devices/{id}/credentials", id: "getDeviceCredentials", tag: "Credentials", summary: "The login ShellyLanMan uses for a device",
		desc: "The user and **password in clear**, for programs that must log in to the device themselves (the Home Assistant integration). Only with the MCP token at access level `configure`; never with a session.",
		ok:   okJSON(200, "The login.", shelly.Credentials{}), auth: authCredentials,
		errs: []errSpec{{403, "The MCP server is off, or the token is missing or not at level `configure`."}, {404, "No such device, or no login stored for it."}, {503, ""}}},

	// ---- Configuration ----
	{method: "GET", path: "/api/v1/config/{section}", id: "getConfig", tag: "Configuration", summary: "Read a settings section of one or more devices",
		desc:  "Sections: `wifi1`, `wifi2`, `login`, `mqtt`, `others`. Reads the current values from the devices as one form; devices that cannot have the section are listed in `excluded`.",
		query: []paramSpec{idsQReq}, ok: okJSON(200, "The form.", service.ConfigForm{}),
		errs: more(eDevice, errSpec{409, "All devices are excluded."}, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/config/{section}", id: "applyConfig", tag: "Configuration", summary: "Change a settings section of one or more devices",
		desc: "`ids` plus the fields of the section: `wifi1` and `wifi2` WiFiApply (needs `confirm: true`: wrong values can take devices off the network), `login` LoginApply, `mqtt` MQTTApply, `others` OthersApply (`part`: `ntp`, `cloud` or `reset`). An offline device gets the change as a deferred task (`queued`).",
		body: configApplyBody{}, bodyDesc: "`ids` and the fields of the section's apply type.", ok: okJSON(200, "One line per device.", results{}),
		errs: more(eDevice, errSpec{409, "All devices are excluded."}, errSpec{404, "Unknown section, or no such device."}, eConfirm, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/checklist", id: "getChecklist", tag: "Checklist", summary: "The configuration checklist",
		query: []paramSpec{idsQ}, ok: okJSON(200, "One row per device.", []service.ChecklistRow{}), errs: e(503)},
	{method: "POST", path: "/api/v1/checklist/action", id: "checklistAction", tag: "Checklist", summary: "Change a checklist setting",
		desc: "eco, led, logs, ap, roaming, extender or autofw, on one or more devices. `errors` lists the devices where it failed; `rows` are the checklist rows after the change.",
		body: service.ChecklistAction{}, ok: okJSON(200, "Failures and the new rows.", checklistResult{}),
		errs: more(eDevice, errSpec{409, "Not supported on this device."}, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/deferred", id: "listDeferred", tag: "Checklist", summary: "Deferred tasks",
		desc: "Actions waiting for an offline device; they run when it is back.",
		ok:   okJSON(200, "The tasks.", []service.DeferredTask{}), errs: e(503)},
	{method: "DELETE", path: "/api/v1/deferred/{id}", id: "cancelDeferred", tag: "Checklist", summary: "Cancel a deferred task",
		ok: done("Cancelled."), errs: []errSpec{{404, ""}, {409, "It cannot be cancelled."}, {503, ""}}},

	// ---- Firmware ----
	{method: "GET", path: "/api/v1/firmware", id: "getFirmware", tag: "Firmware", summary: "Check firmware",
		desc:  "Asks each device (it asks Shelly itself) for its current, stable and beta firmware.",
		query: []paramSpec{idsQ}, ok: okJSON(200, "One row per device.", []service.FirmwareRow{}), errs: more(eDevice, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/firmware/update", id: "updateFirmware", tag: "Firmware", summary: "Update firmware",
		desc: "Starts the update per device, stable or beta. Progress arrives as `firmware.row` events. Destructive: needs `confirm: true`. An offline device gets it as a deferred task.",
		body: firmwareUpdateBody{}, ok: okJSON(200, "One line per device.", results{}), errs: more(eDevice, eConfirm, errSpec{409, "All devices are excluded."}, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/firmware/index", id: "firmwareIndex", tag: "Firmware", summary: "Compare with Shelly's firmware index",
		desc:  "Compares each device with the newest stable firmware in Shelly's index (or the archive, as a fallback). Contacts the internet from the server.",
		query: []paramSpec{idsQ}, ok: okJSON(200, "One row per device.", []service.IndexRow{}), errs: more(eDevice, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/firmware/{id}/local", id: "localFirmware", tag: "Firmware", summary: "A link and QR code to download the firmware to a phone",
		desc: "For updating a device through its own access point: the phone downloads the verified file from ShellyLanMan. The link is valid for 24 hours. Devices are never told to update from it.",
		ok:   okJSON(200, "The link.", service.LocalLink{}),
		errs: []errSpec{{404, ""}, {409, "The device already runs the latest stable firmware, or there is no local firmware for it."}, {502, "Shelly's index could not be read."}, {503, ""}}},
	{method: "GET", path: "/fw/{token}/{name}", id: "firmwareFile", tag: "Firmware", summary: "Download a firmware file",
		desc: "Opened by a phone with the link from `POST /api/v1/firmware/{id}/local`: the token in the path is the authorisation, no session is needed.",
		ok:   okText(200, "The firmware, a ZIP.", "application/zip"), auth: authNone,
		errs: []errSpec{{410, "The link has expired."}, {502, "The file could not be fetched."}, {500, ""}}},

	{method: "POST", path: "/api/v1/firmware/local", id: "localFirmwareModel", tag: "Firmware", summary: "A link and QR code to download the firmware of a model",
		desc: "As `POST /api/v1/firmware/{id}/local`, but for a model: for a device that is not in the list (a new Shelly, reached through its own access point). `gen` is `1` or `2` (Gen2 and newer); `key` is the Gen1 type or Gen2+ app (`GET /api/v1/ap/models`, or read from the access point's name by `GET /api/v1/ap/guide`).",
		body: modelBody{}, ok: okJSON(200, "The link.", service.LocalLink{}),
		errs: []errSpec{{404, "The index has no firmware for that model."}, {502, "Shelly's index could not be read."}, {503, ""}}},
	{method: "GET", path: "/api/v1/ap/guide", id: "apGuide", tag: "Access point", summary: "A device's own access point, with the QR codes to join it and to open its page",
		desc:  "For the wizards that update firmware or set up a new Shelly through the device's own access point (the phone joins it and opens `http://192.168.33.1`). Give a device in the list (`id`: its access point is read from it when it can be reached, else its default name is assumed) or the name of an access point as a phone shows it (`name`: the model is read from it; a MAC of a device in the list also works).",
		query: []paramSpec{{name: "id", typ: "string", desc: "A device in the list."}, {name: "name", typ: "string", desc: "An access point name (`ShellyPlus2PM-A8032AB636EC`, `shellyplug-s-80646F838136`) or a MAC."}},
		ok:    okJSON(200, "The access point.", service.APGuide{}),
		errs:  []errSpec{{404, "No such device."}, {409, "This device has no access point of its own (BLU, unmanaged)."}, {503, ""}}},
	{method: "GET", path: "/api/v1/ap/models", id: "apModels", tag: "Access point", summary: "Models to pick from when an access point's name does not say",
		ok: okJSON(200, "The models, by name.", []model.ModelChoice{})},

	// ---- Backup and restore ----
	{method: "POST", path: "/api/v1/backup", id: "backup", tag: "Backup", summary: "Back up devices",
		desc: "Writes a `.sbk` file per device on the server (ShellyScanner's format). Older ones beyond the `backupKeep` setting are deleted.",
		body: idsBody{}, ok: okJSON(200, "One line per device.", results{}), errs: more(eDevice, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/backups", id: "listBackups", tag: "Backup", summary: "List the backups on the server",
		query: []paramSpec{{name: "id", typ: "string", desc: "Only this device."}},
		ok:    okJSON(200, "The files, newest first.", []service.BackupFile{}), errs: e(503)},
	{method: "GET", path: "/api/v1/devices/{id}/backups/{name}", id: "downloadBackup", tag: "Backup", summary: "Download a backup",
		ok: okText(200, "The `.sbk` file.", "application/octet-stream"), errs: more(eDevice, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/restore/check", id: "restoreCheck", tag: "Backup", summary: "What a restore would do",
		desc: "Checks a backup against the device and lists what would be restored and which questions the restore asks (the original's dialog). Changes nothing. The body may be up to 24 MiB (an uploaded file).",
		body: restoreCheckBody{}, ok: okJSON(200, "The plan.", service.RestorePlan{}), errs: more(eDevice, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/restore", id: "restore", tag: "Backup", summary: "Restore a backup to a device",
		desc: "Destructive: needs `confirm: true`. `answers` are the replies to the questions from the check (key → value). An offline device gets it as a deferred task (`queued`).",
		body: restoreBody{}, ok: okJSON(200, "The result.", service.RestoreResult{}), errs: more(eDevice, eConfirm, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/restore/multi", id: "restoreMulti", tag: "Backup", summary: "Restore the latest backup to several devices",
		desc: "Each device gets its own latest backup. Destructive: needs `confirm: true`.",
		body: idsConfirmBody{}, ok: okJSON(200, "One line per device.", results{}), errs: more(eDevice, eConfirm, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/sbk/scripts", id: "backupScripts", tag: "Backup", summary: "The scripts inside an uploaded backup",
		desc: "Reads an uploaded `.sbk` (base64); changes nothing.",
		body: uploadBody{}, ok: okJSON(200, "The scripts.", []service.BackupScript{}), errs: e(400, 503)},
	{method: "POST", path: "/api/v1/sbk/json", id: "backupJSON", tag: "Backup", summary: "The JSON files inside an uploaded backup",
		desc: "Reads an uploaded `.sbk` (base64); changes nothing. The answer maps file names to their JSON.",
		body: uploadBody{}, ok: respSpec{code: 200, desc: "File name → the JSON in it."}, errs: e(400, 503)},

	// ---- Scripts and KVS ----
	{method: "GET", path: "/api/v1/devices/{id}/scripts", id: "listScripts", tag: "Scripts", summary: "Scripts and key-value store of a device",
		ok: okJSON(200, "The scripts and the KVS items.", service.ScriptsView{}), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/scripts", id: "createScript", tag: "Scripts", summary: "Create a script",
		desc: "The body may be left out: the script gets a default name.",
		body: scriptCreateBody{}, bodyOpt: true, ok: okJSON(200, "The new script.", service.ScriptInfo{}), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "PATCH", path: "/api/v1/devices/{id}/scripts/{sid}", id: "updateScript", tag: "Scripts", summary: "Rename a script or switch it on or off at start-up",
		body: scriptUpdateBody{}, ok: done("Done."), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "DELETE", path: "/api/v1/devices/{id}/scripts/{sid}", id: "deleteScript", tag: "Scripts", summary: "Delete a script",
		ok: done("Deleted."), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/scripts/{sid}/start", id: "startScript", tag: "Scripts", summary: "Start a script",
		query: []paramSpec{{name: "log", typ: "boolean", desc: "`true`: also switch the device's socket log on, as the original's editor does (a configuration change that is not undone)."}},
		ok:    done("Started."), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/scripts/{sid}/stop", id: "stopScript", tag: "Scripts", summary: "Stop a script",
		ok: done("Stopped."), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "GET", path: "/api/v1/devices/{id}/scripts/{sid}/code", id: "getScriptCode", tag: "Scripts", summary: "A script's code",
		ok: okJSON(200, "The code.", codeBody{}), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "PUT", path: "/api/v1/devices/{id}/scripts/{sid}/code", id: "putScriptCode", tag: "Scripts", summary: "Replace a script's code",
		desc: "Up to 24 MiB. A running script keeps running its old code until restarted.",
		body: scriptCodeBody{}, ok: done("Saved."), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/scripts/log", id: "scriptLogOn", tag: "Scripts", summary: "Switch the device's socket log on",
		desc: "Sets the device's debug WebSocket on, which the script editor needs to show a script's output. A configuration write; it is not switched off again.",
		ok:   done("Done."), errs: more(eDevice, errSpec{409, "The device has no scripts."}, errSpec{503, ""})},
	{method: "POST", path: "/api/v1/devices/{id}/kvs", id: "setKVS", tag: "Scripts", summary: "Set a key-value store item",
		body: kvsBody{}, ok: okJSON(200, "The item.", service.KVItem{}), errs: more(eDevice, errSpec{409, "The device has no KVS."}, errSpec{503, ""})},
	{method: "DELETE", path: "/api/v1/devices/{id}/kvs", id: "deleteKVS", tag: "Scripts", summary: "Delete a key-value store item",
		query: []paramSpec{{name: "key", typ: "string", desc: "The key.", required: true}},
		ok:    done("Deleted."), errs: more(eDevice, errSpec{409, "The device has no KVS."}, errSpec{503, ""})},
}

var tagDocs = []struct{ name, desc string }{
	{"Status", "Settings, version, log and this description."},
	{"Events", "The WebSocket that pushes changes."},
	{"Devices", "The list of devices and what you can do with one."},
	{"Control", "Switching, dimming, moving and raw RPC calls."},
	{"Discovery", "Finding devices and the archive."},
	{"BLU", "BLU devices behind gateways."},
	{"Credentials", "Logins for protected devices."},
	{"Configuration", "Wi-Fi, login, MQTT, NTP, cloud — on one or many devices."},
	{"Checklist", "The configuration checklist and deferred tasks."},
	{"Firmware", "Checking and updating firmware."},
	{"Access point", "A device's own Wi-Fi access point, for the wizards that update firmware or set up a new Shelly through it."},
	{"Backup", "Backup and restore (`.sbk`, ShellyScanner's format)."},
	{"Scripts", "Scripts and the key-value store of Gen2+ devices."},
	{"Scheduler", "Schedules of Gen2+ devices."},
	{"Charts", "Chart samples and energy history."},
	{"Log", "Device logs."},
	{"Authentication", "The optional UI password."},
	{"MCP", "The MCP server for AI assistants."},
}

var standardErr = map[int]string{
	400: "The request is wrong: invalid JSON, an unknown field, or an argument that does not fit. `error` says what.",
	401: "Login required: a UI password is set and the request has neither a session cookie nor a valid MCP token.",
	403: "The device asked for a password ShellyLanMan does not have (or has wrong), or a browser request came from another origin.",
	404: "No such device (or thing).",
	409: "The request is understood but cannot be done now.",
	413: "The request body is too large (64 KiB; restore and script uploads 24 MiB).",
	415: "`Content-Type` must be `application/json`.",
	428: "Confirmation required: repeat the request with `confirm: true`.",
	500: "Something failed inside ShellyLanMan.",
	502: "The device answered with an error.",
	503: "The device service is not running.",
	504: "The device could not be reached (offline).",
}

var pathDocs = map[string]string{
	"id":      "Device id: the MAC address, upper case, no separators (`Device.id`).",
	"sid":     "Script id on the device (0, 1, 2 …).",
	"index":   "Number of the request in `GET /api/v1/devices/{id}/info`.",
	"name":    "File name of the backup, from `GET /api/v1/backups`.",
	"section": "`wifi1`, `wifi2`, `login`, `mqtt` or `others`.",
	"token":   "The token in the link from `POST /api/v1/firmware/{id}/local`.",
}

var pathParam = regexp.MustCompile(`\{(\w+)\}`)

const infoDescription = `The REST API of ShellyLanMan, the same API its web page uses. It is on the same address and port as the page.

**Authentication.** With no UI password set, no login is needed. With a password set, send either the session cookie ` + "`slm_session`" + ` (` + "`POST /api/v1/auth/login`" + `) or ` + "`Authorization: Bearer <MCP token>`" + ` (Settings → MCP; the MCP server must be on). A token at access level ` + "`read`" + ` may only use ` + "`GET`" + `; ` + "`control`" + ` and ` + "`configure`" + ` may use every method. Under Home Assistant ingress, Home Assistant's own login applies. Browsers: a state-changing request whose ` + "`Origin`" + ` is another site is refused (403); programs that send no ` + "`Origin`" + ` header are fine.

**Requests.** Bodies are JSON with ` + "`Content-Type: application/json`" + `; unknown fields are refused (400). Lists of devices are ` + "`ids`" + ` in the body, or comma separated in the query. A device is named by its id (the MAC, upper case, no separators) as it is in ` + "`GET /api/v1/devices`" + `.

**Confirmation.** Destructive actions (reboot, restore, firmware update, Wi-Fi settings, a circuit breaker) need ` + "`\"confirm\": true`" + ` in the body; without it the answer is ` + "`428`" + `. ` + "`POST /api/v1/devices/{id}/rpc`" + ` asks for it with risky RPC methods and refuses factory resets.

**Errors.** A failure is ` + "`{\"error\": \"…\"}`" + ` with a status code. For an operation on a device: ` + "`404`" + ` no such device, ` + "`403`" + ` the device wants a password ShellyLanMan does not have, ` + "`504`" + ` the device cannot be reached, ` + "`502`" + ` the device answered with an error.

**Several devices.** Actions on several devices answer ` + "`200`" + ` with one result line per device (` + "`ok`" + `, ` + "`fail`" + ` or ` + "`queued`" + `), also when some failed. A device that is offline gets the action as a *deferred task* where the original does the same.

**Events.** Changes are pushed over the WebSocket ` + "`/ws`" + `; the REST API is for asking and doing.

**Not in this description.** The web page and its files, and the Home Assistant app's local listener.

Written for the version of ShellyLanMan that serves it; it may gain fields and operations with a release. Fields you do not know: ignore them.`

var (
	specOnce  sync.Once
	specCache map[string]any
)

// buildSpec makes the OpenAPI document (without `servers`, which depends on the request).
func buildSpec() map[string]any {
	g := newSchemaGen()
	paths := map[string]map[string]any{}
	for _, op := range operations {
		item := paths[op.path]
		if item == nil {
			item = map[string]any{}
			paths[op.path] = item
		}
		item[strings.ToLower(op.method)] = g.operation(op)
	}
	var tags []any
	for _, t := range tagDocs {
		tags = append(tags, map[string]any{"name": t.name, "description": t.desc})
	}
	g.comps["Error"] = map[string]any{
		"type": "object", "required": []string{"error"},
		"properties": map[string]any{"error": map[string]any{"type": "string", "description": "What went wrong."}},
	}
	shared := map[string]any{}
	for code, d := range standardErr {
		shared["E"+strconv.Itoa(code)] = errorResponse(d)
	}
	comps := map[string]any{
		"schemas":   g.comps,
		"responses": shared,
		"securitySchemes": map[string]any{
			"sessionCookie": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName, "description": "The session from `POST /api/v1/auth/login`. Needed only when a UI password is set."},
			"mcpToken":      map[string]any{"type": "http", "scheme": "bearer", "description": "The MCP token (Settings → MCP). Needed only when a UI password is set; a read-only token may only use GET."},
		},
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title": "ShellyLanMan API", "version": version.Version, "description": infoDescription,
			"license": map[string]any{"name": "GPL-3.0-or-later", "identifier": "GPL-3.0-or-later"},
		},
		"tags":       tags,
		"paths":      paths,
		"components": comps,
		// Either a session or a token; neither when no UI password is set.
		"security": []any{map[string]any{"sessionCookie": []any{}}, map[string]any{"mcpToken": []any{}}},
	}
}

func (g *schemaGen) operation(op operation) map[string]any {
	o := map[string]any{"operationId": op.id, "summary": op.summary, "tags": []string{op.tag}}
	if op.desc != "" {
		o["description"] = op.desc
	}
	var params []any
	for _, m := range pathParam.FindAllStringSubmatch(op.path, -1) {
		typ := "string"
		if m[1] == "sid" || m[1] == "index" {
			typ = "integer"
		}
		params = append(params, map[string]any{"name": m[1], "in": "path", "required": true, "description": pathDocs[m[1]], "schema": map[string]any{"type": typ}})
	}
	for _, q := range op.query {
		p := map[string]any{"name": q.name, "in": "query", "description": q.desc, "schema": map[string]any{"type": q.typ}}
		if q.required {
			p["required"] = true
		}
		params = append(params, p)
	}
	if len(params) > 0 {
		o["parameters"] = params
	}
	if op.body != nil {
		var schema map[string]any
		if _, isCfg := op.body.(configApplyBody); isCfg {
			schema = g.configApply()
		} else {
			schema = g.of(op.body)
		}
		rb := map[string]any{"required": !op.bodyOpt, "content": map[string]any{"application/json": map[string]any{"schema": schema}}}
		if op.bodyDesc != "" {
			rb["description"] = op.bodyDesc
		}
		o["requestBody"] = rb
	}
	resp := map[string]any{}
	ok := map[string]any{"description": op.ok.desc}
	switch {
	case op.ok.body != nil:
		ok["content"] = map[string]any{"application/json": map[string]any{"schema": g.of(op.ok.body)}}
	case op.ok.text != "":
		ok["content"] = map[string]any{op.ok.text: map[string]any{"schema": map[string]any{"type": "string"}}}
	}
	resp[strconv.Itoa(op.ok.code)] = ok
	errs := append([]errSpec(nil), op.errs...)
	if op.body != nil {
		errs = append(errs, errSpec{code: 415}, errSpec{code: 413})
		if !hasCode(errs, 400) {
			errs = append(errs, errSpec{code: 400})
		}
	}
	if op.auth != authNone && op.auth != authMCP && op.auth != authCredentials {
		errs = append(errs, errSpec{code: 401})
	}
	for _, er := range errs {
		if _, dup := resp[strconv.Itoa(er.code)]; dup {
			continue
		}
		if er.desc == "" { // the standard text of the code: one shared response
			resp[strconv.Itoa(er.code)] = map[string]any{"$ref": "#/components/responses/E" + strconv.Itoa(er.code)}
			continue
		}
		resp[strconv.Itoa(er.code)] = errorResponse(er.desc)
	}
	o["responses"] = resp
	switch op.auth {
	case authNone:
		o["security"] = []any{}
	case authCredentials, authMCP:
		o["security"] = []any{map[string]any{"mcpToken": []any{}}}
	}
	return o
}

// configApply: `ids` plus one of the sections' apply types.
func (g *schemaGen) configApply() map[string]any {
	ids := g.of(configApplyBody{})
	var one []any
	for _, v := range []any{service.WiFiApply{}, service.LoginApply{}, service.MQTTApply{}, service.OthersApply{}} {
		one = append(one, g.of(v))
	}
	return map[string]any{"allOf": []any{ids, map[string]any{"oneOf": one}}}
}

func errorResponse(desc string) map[string]any {
	return map[string]any{"description": desc, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
}

func hasCode(errs []errSpec, code int) bool {
	for _, e := range errs {
		if e.code == code {
			return true
		}
	}
	return false
}

func (s *server) openapiRoutes(mux router) {
	mux.HandleFunc("GET /api/v1/openapi.json", s.getOpenAPI)
}

// getOpenAPI serves the description; `servers` is the address the request came to.
func (s *server) getOpenAPI(w http.ResponseWriter, r *http.Request) {
	specOnce.Do(func() { specCache = buildSpec() })
	doc := make(map[string]any, len(specCache)+1)
	for k, v := range specCache {
		doc[k] = v
	}
	doc["servers"] = []any{map[string]any{"url": baseURL(r), "description": "This ShellyLanMan"}}
	writeJSON(w, http.StatusOK, doc)
}

// baseURL is the address the client used, also behind a reverse proxy.
func baseURL(r *http.Request) string {
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// operationKeys lists "METHOD path" of the description, sorted (tests).
func operationKeys() []string {
	var keys []string
	for _, op := range operations {
		keys = append(keys, op.method+" "+op.path)
	}
	sort.Strings(keys)
	return keys
}
