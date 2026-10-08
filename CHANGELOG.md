# Changelog

All notable changes. Format based on [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.9.6] - 2026-10-08

Create a profile from a Shelly you already set up, and a release check that fits the Home Assistant app.

### Added
- **Create profile…** (Devices page, one device selected): reads the device and lists the settings a profile can
  take over, with a tick; those that differ from a Shelly as it leaves the factory are ticked at first. Then the
  profile editor opens with them filled in; the profile's name is required and the passwords are typed (a
  device does not tell them). `GET /api/v1/profiles/from-device`.

### Changed
- *Check for new ShellyLanMan versions* is now the first setting under Settings → General, where
  the About page's "Release check off" link leads (it was at the bottom, between other settings).
- README links to the Home Assistant Community thread.
- The profile's name is marked as required in the editor.
- In the Home Assistant app the release check looks at the app's releases (`shellylanman-ha`), where Home Assistant gets
  its updates from, and links there; "Skip this version" is not offered in the app (it would only hide ShellyLanMan's
  own line).

## [0.9.5] - 2026-10-07

Two wizards for new Shellys (firmware through the device's own access point, and
"Set up a new Shelly" with profiles), an update arrow, an OpenAPI description of the
API — and two security fixes: the device log needs the login, and the raw RPC
endpoint no longer runs anything it is given.

### Added
- A device with a newer stable firmware shows **↑** after its status (like ↻ for "reboot
  needed"), with the version in the tooltip; a summary chip *Updates* and a selection
  *Firmware update available*. The device reports it itself, so it costs no extra request.
  The MCP device list says `update_available`.
- The REST API is described in **OpenAPI 3.1** at `/api/v1/openapi.json` (linked on the
  About page): every operation with its parameters, bodies, answers and errors, for
  Postman, Swagger UI or a code generator. A test keeps it equal to the routes.

- **Update firmware through a device's own access point**, a wizard with QR codes (Firmware
  page): give the access point's name (or pick the device); the file goes to the phone, a QR code
  joins the phone to the device's access point, another opens `192.168.33.1`, and ShellyLanMan waits
  until the device is back and shows its version. It also serves a Shelly ShellyLanMan does not know.
  Where the access point of a Gen2+ device is switched off, the wizard offers to switch it on; for Gen1 it says how to do it in the device's page.

- **Set up a new Shelly**: a wizard (button on the Devices page) with **profiles** (Settings →
  Profiles). A profile says what a new device gets: a name made from a pattern, login, MQTT, time
  server, cloud and the checklist's settings (eco, LED, access point, roaming, automatic firmware
  update), and a reminder of your Wi-Fi's name (its password is never kept). The phone joins the new
  device's own access point (QR code), opens its page (QR code) and you enter your Wi-Fi there; when the
  device is on your network ShellyLanMan shows what the profile would do and, after your go, does it.
  Passwords in a profile are stored encrypted and are never shown again. `GET/POST /api/v1/profiles` and more.

### Changed
- The raw RPC endpoint behind the scheduler's *test method* button (`POST /api/v1/devices/{id}/rpc`)
  no longer runs anything it is given: methods that restart, update, delete, or replace code
  or a settings block ask for confirmation (a dialog, or `confirm: true` in the API); a
  factory reset, a Wi-Fi reset and the delete-all methods are refused there.

### Fixed
- A device's live log (`/ws/log/…`) could be opened without the UI password; it is now
  behind the login like the rest.

## [0.9.4] - 2026-10-06

A Log page: see ShellyLanMan's own log in the browser.

### Added
- **Log** page, above Settings: ShellyLanMan's own log — the last 1000 lines of this run,
  kept in memory, live while the page is open. Filter by level (information and up,
  warnings and errors, errors only) and by text; pause, clear, copy. `GET /api/v1/log`.

## [0.9.3] - 2026-10-06

Ports: one setting, shown in ShellyLanMan — and the Home Assistant app no longer
clashes with other apps on the same host.

### Changed
- The port has one setting: `SHELLYLANMAN_PORT` (Docker: `docker-compose.yml` or
  `docker run -e`; the Home Assistant app: option `port`). *Settings → General → Ports*
  shows the ports in use and where to change them; the page no longer changes the port.
  `SHELLYLANMAN_LISTEN` is gone.
- `docker-compose.yml` lists every option, the optional ones in comments; the README's
  Quick start is that file.
- A port that is already taken stops ShellyLanMan with a message naming the port and
  where to set another one.
- Home Assistant app: the sidebar (ingress) uses a free port that Home Assistant
  chooses, instead of 8099 — it clashed with another app on the same host.

### Added
- README: *Shellys with a password* — how ShellyLanMan logs in to protected Shellys.
- `localUrl` in `GET /api/v1/status` and `/api/v1/about` (Home Assistant app): where the
  integration next to the app reaches it without token.

## [0.9.2] - 2026-10-05

More room on the screen: the sidebar can be full or minimal, as in Home Assistant.

### Added
- Full or minimal sidebar on wide screens, as in Home Assistant: the button at the top
  left switches between icons with labels and icons only (remembered per browser);
  the logo moved to the right of the name. On a phone the ☰ drawer stays.

### Fixed
- Settings → Security in Home Assistant's sidebar no longer says that switching the
  password off needs the current one.

### Changed
- README: screenshots with the new sidebar header.

## [0.9.1] - 2026-10-05

A forgotten UI password can be reset in the Home Assistant app too.

### Fixed
- Home Assistant app: in Home Assistant's sidebar the UI password can be changed or
  switched off without the current one (you are logged in to Home Assistant there,
  and the app cannot be started with `SHELLYLANMAN_RESET_PASSWORD`).

### Changed
- README and `docker-compose.yml`: how to reset a forgotten password.

## [0.9.0] - 2026-10-05

Security: an optional password for the UI, two fixes found by code scanning, and
the repository checked by Dependabot and CodeQL.

### Added
- Optional password for the UI (*Settings → Security*, off by default): one password,
  no user name; a login page with *Stay logged in* (30 days since the last use);
  slower after five wrong tries; not asked under Home Assistant's sidebar, where
  Home Assistant's login applies. Programs use the MCP token for the API; the
  Home Assistant integration next to the app keeps working without one (the app's
  loopback listener serves its calls). Forgotten: start once with
  `SHELLYLANMAN_RESET_PASSWORD=1`.

### Changed
- Repository security: Dependabot (alerts, security and weekly version updates),
  CodeQL code scanning and private vulnerability reporting are on.
- Updated: Go modules, Node 26 for building the web UI.
- README: tests, CodeQL and PayPal badges.

### Fixed
- Device login (digest): the realm, nonce and opaque a device sends are quoted
  properly in the answer, so a device cannot add header fields of its own (CodeQL).
- Backups: a device id `.` or `..` can no longer point outside the backups folder
  (CodeQL).

## [0.8.0] - 2026-10-05

BLU devices that a gateway only relays get a row of their own, and a wizard tells
you which model they are.

### Added
- Rows for BLU devices that a gateway only relays (BLE.CloudRelay): readings,
  buttons and sensors from their BTHome messages; read only — no backup, restore
  or logs. The model is estimated from what the device sends ("?") until it is
  identified; a name can be given under Notes.
- *Identify BLU devices*: a wizard in which a gateway listens while you put a BLU
  device in pairing mode, and shows the model of every device that answers. The
  model is stored in ShellyLanMan's archive; nothing changes on the device or the
  gateway. On the Devices page (Identify) and in the checklist's BLE dialog.
- API: `relay` in the device list, `GET /api/v1/blu/gateways`,
  `POST /api/v1/blu/identify` (events `blu.identify`, `blu.discovered`), and an
  optional `name` when saving the notes of a relayed BLU device.

### Changed
- README: screenshots with a relayed BLU device and the Identify wizard.

## [0.7.0] - 2026-10-04

Your Shellys into Home Assistant in one go: the Home Assistant integration adds the
Shellys ShellyLanMan knows to Home Assistant's Shelly integration.

### Added
- API for the Home Assistant integration (adding Shellys to Home Assistant's Shelly
  integration): `protected` in the device list, and
  `GET /api/v1/devices/{id}/credentials` with the credentials ShellyLanMan uses for a
  device — only with the MCP token at access level *configure*, or on the Home
  Assistant app's loopback listener; never on the open LAN port without that token.

## [0.6.4] - 2026-10-04

The script editor gives feedback at once and never opens blind.

### Changed
- Script editor: the window opens at once and says it is reading the script, instead
  of nothing happening for seconds on a device with weak Wi-Fi. A second click or
  double-click does not open a second editor.

### Fixed
- Script editor: when the device could not send the code (busy, HTTP error), an empty
  editor opened, and *Upload* would have wiped the script on the device. It now shows
  the error with *Retry*, and *Upload* / *Upload and run* stay off until the code was
  read. The API and MCP report the error too (`Script.GetCode`).

## [0.6.3] - 2026-10-04

The script editor follows the app's light or dark look.

### Changed
- Script editor colours follow the app (light or dark) by default. Settings → Script
  editor → *Editor colours* can fix them to dark or light; a dark editor chosen before
  stays dark.

## [0.6.2] - 2026-10-03

The script editor shows the code again.

### Fixed
- Script editor: the code was not shown (only line numbers, the text far below the
  editor) because the Content-Security-Policy blocked the editor's stylesheet. Inline
  styles are now allowed only with a nonce that is new for every page load, and the
  editor uses it.

### Added
- Script editor: a "Reading the script from the device…" dialog while a slow device
  sends its code.

## [0.6.1] - 2026-10-03

Charts follow the shared selection, like Checklist and Firmware.

### Fixed
- Charts follow the shared selection too: devices ticked on Devices, Checklist or
  Firmware are charted when you open Charts from the menu (it only worked with the
  *Charts* button).

## [0.6.0] - 2026-10-03

First use made easier: one selection across Devices, Checklist and Firmware, buttons
that explain themselves, a calmer rescan, the Home Assistant look everywhere, a menu
for phones, and installable as an app. ShellyLanMan started from ShellyScanner and now
also goes its own way where the web makes that clearer (`DECISIONS.md` §21).

### Fixed
- A device that did not answer when it was discovered under a name without its MAC
  address (for example a custom name) stayed in the list as an extra row in *error*
  next to the same device once it was identified, until a rescan. The extra row now
  goes as soon as the device is identified.

### Changed
- One selection for Devices, Checklist and Firmware: devices ticked on one page are
  ticked on the others. Checklist and Firmware show the selected devices, or all
  devices when none is selected, with a button to switch between the two.
- Checklist has checkboxes, like Devices. The *Checklist* button on Devices is always
  available.
- *Web UI* asks before opening more than one device, and says it opens one tab per
  device (was: more than eight).
- *Print* no longer clears the selection.
- Checklist: a grey button says why in its tooltip (nothing ticked, devices without the
  setting, different values, one device only); every button says what it does.
- Devices toolbar in groups: selection with *Checklist* and *Firmware* (new button);
  actions for several devices; actions for one device. Every button has a tooltip
  that says what it does and, when it is grey, why.
- *Refresh* reads only the selected devices when some are selected (*Refresh (3)*),
  else all. Clearer tooltips for *Refresh*, *Rescan* and *Reload*.
- Firmware shows its rows at once and fills them in as each device answers, like the
  Checklist (was: a spinner until every device had answered).
- The Home Assistant palette is now the start look everywhere, light or dark as your
  system prefers, until you choose another. A look you already chose stays. The
  former *Default* palette is called *Midnight*.
- On narrow screens (phones, either way round, and small tablets) the navigation is a
  ☰ menu in the top bar instead of a bottom bar or an icon rail, so pages get the full
  width.
- *Rescan* no longer shows almost every device as archived for a while: the devices
  that were listed show *searching*, are asked at their last address at once, and
  become archived (or leave the list without the archive) only when the search ends —
  the end of the IP scan, or 30 s in the mDNS modes. New status `searching` in the API
  and MCP.
- A device in error says in its status tooltip how to read it again (*Reload*).
- About page and README: ShellyLanMan started from ShellyScanner, is built on its basis
  and has been developed further as a web application of its own.
- English writes *online* / *offline* (was *on line* / *off line*), Italian too.

### Added
- Settings → Appearance: open device web pages *in a new tab* (default) or *in this
  tab*, per browser — for kiosk browsers such as Fully Kiosk that allow no new tabs.
  Links and buttons that open another page show ↗.
- As a Home Assistant app, the log says when ShellyLanMan announced itself to Home
  Assistant (it only said so when that failed).
- ShellyLanMan can be installed as an app (PWA) when it is served over HTTPS or on
  `localhost`: a web app manifest and icons, no offline cache.

## [0.5.0] - 2026-10-02

ShellyLanMan and Home Assistant work together: the app announces itself, a new
integration puts ShellyLanMan's checks on your Shelly devices and its tools in Assist.

### Fixed
- Checklist page: the row under the mouse flickered between light and dark: every
  device update (every few seconds) rebuilt the whole table and toolbar. It now redraws
  only when a status changes, and then updates the table in place.

### Added
- As a Home Assistant app, ShellyLanMan announces itself to Home Assistant (Supervisor
  discovery `shellylanman` and `mcp`), so the integration and Home Assistant's own MCP
  client are offered with one click; announced again after a port change.
- `SHELLYLANMAN_MCP_LOCAL`: an MCP listener without token on a loopback address, for
  Home Assistant on the same host (the app sets it; MCP must be enabled, the access
  level applies).
- Checklist page: the setting buttons are labelled "Change on the selected devices",
  a hint explains them while nothing is selected, and their tooltips also show when
  they are disabled.
- About page: icons on the cards, a paragraph on MCP and Home Assistant, and "Built
  with the help of Claude by Anthropic"; README: a new introduction (one go-to app,
  standalone or as a Home Assistant app), more reasons why, Contributing.
- `/api/v1/about` reports an `instanceId` (stable per data folder) for the integration.

## [0.4.0] - 2026-10-01

ShellyLanMan in Home Assistant, an MCP server that can do everything the Shelly-MCP
does, and protected devices with firmware 2.x working again.

### Added
- MCP: everything the Shelly-MCP offers, locally (DECISIONS §20). A third access level
  **configure** (Settings → MCP) for scripts (create, upload, start/stop, eval,
  delete), KVS, schedules, webhooks, virtual components, any Gen2+ RPC write and the
  device password; raw status and configuration for every generation with passwords
  masked; component list, script code, energy history of EM meters; scenes stored in
  ShellyLanMan; a flip-back timer for relays and a fade for lights; rescan. Deletes,
  code, RPC writes and the device password need `confirm: true`; factory reset also
  `allow_data_loss`.
- Palette **Home Assistant** (dark and light): the colours of Home Assistant's default
  themes, so ShellyLanMan blends in when it runs as a Home Assistant app. Opened in Home
  Assistant and with nothing chosen yet, ShellyLanMan uses it, light or dark as the
  browser prefers (and follows a change); a palette or theme you pick is kept.
- Home Assistant app support: with `SHELLYLANMAN_INGRESS` set, a second listener for
  Home Assistant's ingress that only the Supervisor may use; the web UI works under a
  path prefix (relative URLs), hides the no-auth banner inside Home Assistant and shows
  the LAN address of the MCP there. The app itself lives in `shellylanman-ha`.
- About page and README: Shelly-MCP by Buggy1111 under "Based on" and "Credits", as the
  model for the MCP server.
- `docs/phase-11-ha-mcp.md`: the Home Assistant plan (app, integration, Assist) and MCP
  parity with the Shelly-MCP.

### Fixed
- Password-protected Gen2+ devices with firmware 2.0 or newer: every command and
  setting sent with POST /rpc (thermostats, configuration, scripts, KVS, login,
  restore) failed with "unauthorized", and the retries set off the device's
  brute-force lock. The firmware now sends the login challenge only in a header;
  ShellyLanMan answers it.
- Releases: a version tag now also creates the GitHub release (notes from this
  changelog). The in-app release check reads GitHub releases, and there were none, so
  it never reported a new version.

## [0.3.0] - 2026-10-01

ShellyLanMan as a tool for AI assistants: an MCP server for the devices on the LAN.

### Added
- MCP server for AI assistants at `/mcp` (Model Context Protocol, Streamable HTTP):
  read tools (devices, readings, firmware, checklist, backups, read-only Gen2+ RPC),
  control tools (relays, lights, covers, thermostats, backup) and destructive tools
  (reboot, firmware update, with `confirm`). Local only, off by default, bearer token,
  read-only unless set to control, control actions logged. Settings → MCP shows the
  token once and the command for Claude Code. API: `GET/PUT /api/v1/mcp`,
  `POST /api/v1/mcp/token`.
- MCP `shelly_checklist`: the tool description explains every cell (`led` is
  "LED off", an empty `ble` list means Bluetooth on with nothing relayed, `-` / `null`
  mean not applicable / no such setting).

### Fixed
- Documentation: `DECISIONS.md` §1–15 and the Phase 2–9 hardware test results in
  `docs/hardware-tests.md` were lost when later phases overwrote the files instead of
  appending; restored from history. Status headers of `ARCHITECTURE.md`,
  `DECISIONS.md` and `FEATURE_PARITY.md` brought up to date.

## [0.2.0] - 2026-09-28

Feedback on the first release: a steady devices table, eight languages, the port as a
setting, a new About page and logo.

### Added
- The UI in German, French, Spanish, Italian, Bulgarian and Chinese (next to English
  and Dutch).
- Web server port in Settings → General: ShellyLanMan moves to the new port at once
  and the page follows. `SHELLYLANMAN_LISTEN`, when set, still wins and locks the
  setting. The Docker health check follows the saved port.
- New logo: in the sidebar, as favicon and on the About page and README.
- About page rebuilt: what ShellyLanMan is, version with up-to-date state, uptime,
  system information (runtime, platform, memory, data, languages, licence and
  third-party notices), support links, release notes and dependencies with their
  licences. API: `GET /api/v1/about/changelog`, `/license`, `/notices`,
  `GET/PUT /api/v1/server`.
- README: badges, why, features, pages, screenshots, security and support.

### Changed
- ShellyScanner and MikroDash are both credited under Based on and Credits; the
  notice now says ShellyLanMan is heavily based on ShellyScanner but not affiliated.
- The "UI authentication is off" banner is dismissed once per browser instead of once
  per tab.

### Fixed
- Devices table: a refresh no longer rebuilds the whole table. Rows and cells that did
  not change stay in place, so the row under the mouse no longer flickers and a click
  on a row checkbox or toolbar button is no longer lost when an update arrives.

## [0.1.0] - 2026-09-28

First release: the features of ShellyScanner as a web application, plus the local
firmware download via QR code.

### Added — Phase 10: release
- Opt-in check for new ShellyLanMan releases (GitHub; never / stable / all, off by
  default) with a banner and "skip this version".
- README: feature overview, what leaves the LAN, image platforms and tags, update and
  data-volume backup instructions.

### Changed — Phase 9: parity review
- Right-click menu on the devices table (device: info, web UI, settings, backup,
  restore, notes, reload; archived device: reload, notes, remove) — it was missing.
- API: `GET /api/v1/devices?gen=` (ShellyScanner's `-gen`).
- A device that could not be read when discovered is retried every 2 minutes
  (ShellyScanner retries once, 30 s after the scan starts — O33, agreed extension).

### Added — Phase 8: advanced functions
- Notes and keyword per device (Notes action, kept in the archive; keyword up to 32
  characters; only with the archive in use, as in ShellyScanner).
- Export the devices table as CSV (shown columns and rows, separator setting per
  browser) and print it (print stylesheet).
- Scripts and KVS of Gen2+ devices (API): list, create, rename, enable, start/stop,
  delete, read and write code (1024-character segments), scripts inside a `.sbk`;
  KVS list (paged), set, delete.
- Scripts dialog (Scripts and KVS tabs) and the script editor (CodeMirror 6, loaded on
  demand): open/save files, upload, run/stop, upload and run, script output, find,
  go to line, completion, comments, case change; editor settings per browser.
- Scheduler back end: RPC call to a Gen2+ device (BLU TRV through its gateway),
  method hints from the device's components, JSON entries of a `.sbk`.
- Scheduler dialogs: Gen2+ jobs (cron editor with sunrise/sunset, value pickers,
  weekdays and months, calls with method hints, parameter editor and test button,
  enable, add/duplicate/remove/copy/paste, load from a backup), Wall Display thermostat
  profiles and rules, BLU TRV rules (temperature or valve position).
- Chart samples kept on the server per device (24 h in memory), so charts open with
  history; EM energy records (EMData / EM1Data) for the EM chart. API: `GET/DELETE
  /api/v1/samples`, `GET /api/v1/devices/{id}/emdata`.
- Charts page (Chart.js, loaded on demand): the graph types of ShellyScanner for the
  selected devices, range, series, markers, pause, zoom and pan, CSV export
  (horizontal / vertical), image copy, clear; default graph and CSV layout settings.

### Added — Phase 7: firmware
- Firmware check per device like ShellyScanner (Gen1 `/ota/check` + `/ota`, Gen2+
  `Shelly.CheckForUpdate` + `Shelly.GetDeviceInfo`, BLU TRV through its gateway; stored
  data for sleeping battery devices), short version names, update to stable or beta,
  download progress and restart followed over the device's WebSocket, the row checked
  again when the device is back; archived and off-line devices get a deferred update.
- API: `GET /api/v1/firmware`, `POST /api/v1/firmware/update` (needs `confirm: true`).
- **New: local firmware download via QR code.** The server compares every Wi-Fi device
  with Shelly's firmware index (Gen1: official index, fallback shelly-tools archive;
  Gen2+: `updates.shelly.cloud`, stable only, pinned certificates) and, for a newer
  stable version, makes a 24-hour link on this server with a QR code; the file is
  downloaded once, verified (SHA-256 / manifest) and cached in `/data/firmware`.
  API: `GET /api/v1/firmware/index`, `POST /api/v1/firmware/{id}/local`, `GET /fw/{token}/{file}`;
  setting `phoneBaseURL`. Dependency: `skip2/go-qrcode` (MIT).
- Firmware page: "Shelly index" column with the ⚡ local download (QR code, link, device,
  model, version change, source, expiry); "Address of ShellyLanMan for phones" setting.
- FW Update panel as the first tab of Devices settings (as in ShellyScanner) and on the
  Firmware page (all devices or `#/firmware?ids=`): current / new stable / new beta,
  select buttons, counters, filter, Check, confirmation, live progress.

### Added — Phase 6: backup and restore
- Backup engine for every generation (`.sbk`, compatible with ShellyScanner): Gen1
  settings and actions; Gen2+ configuration, schedules, webhooks, KVS, scripts,
  dynamic components, add-on peripherals and the model-specific sections; battery
  devices from their stored data when they sleep; BTHome devices and the BLU TRV
  through their gateway.
- Restore engine with the checks of the original (other host, model, profile, mode,
  add-on, passwords to ask, script conflicts) and the restore steps per model.
- Backups are kept in `/data/backups/<device>/`, newest first, with a retention
  setting (default 10 per device); backup and restore of an off-line device are
  queued as deferred tasks; multi-device restore from the newest backups.
- REST API: `POST /api/v1/backup`, `GET /api/v1/backups`, backup download,
  restore check, restore and multi restore (both need `confirm: true`).
- Backup and Restore buttons on the devices table: backup results per device; restore
  wizard (backup of this device, of another device or an uploaded `.sbk`, download
  link), the questions of the original (other host, errors, warnings, passwords for
  login / Wi-Fi / AP / MQTT, script name conflicts and enabling), confirmation, and a
  reboot offer; multi-device restore from the newest backups. Retention setting on
  the network settings page.

### Added — Phase 5: configuration
- Devices settings for one or more devices: Wi-Fi 1 and Wi-Fi 2 (enable/disable,
  DHCP/static/keep, copy from another device, warning before applying), restricted
  login (new credentials are kept for the device), MQTT (Gen1 panel with reconnect,
  clean session, keep alive, QoS, retain, update period; Gen2+ panel with MQTT
  control, RPC over MQTT and notifications; mixed panel), NTP server, cloud and reset
  by input; one result line per device.
- Deferred tasks: login, MQTT, NTP, cloud and input-reset changes for off-line or
  archived devices are queued and run when the device is back; "Deferred" page with
  cancel and a waiting count on the sidebar; kept in `/data/deferred.json` with the
  passwords encrypted.
- Configuration checklist: eco mode, LED, logs, Bluetooth (relayed BLU devices and
  gateways), access point, roaming, Wi-Fi static/DHCP, range extender, scripts and
  automatic firmware update per device, with the toolbar and right-click actions of
  the original.
- MQTT delay between devices (ShellyScanner's `-slow`) as a setting.

### Fixed — found during Phase 5
- The browser kept an old `app.js` after an upgrade: every static file is now served
  with `Cache-Control: no-cache`.

### Added — Phase 4: device controls
- Command column with the controls of ShellyScanner, drawn per model like its cell
  editor: relays (ON/OFF, input indicator), covers (open/stop/close, position slider
  when calibrated), dimmers and white lights, RGB/RGBW/RGBCCT (gain, white),
  thermostats (Wall Display, LinkedGo XT1, BLU TRV: enable, target ▲▼ and slider;
  Gen1 TRV: profile and target), circuit breaker (with confirmation, locked state),
  camera privacy, input event buttons (the server calls the configured action URLs /
  webhooks, as the original does).
- Lights editor: switch, brightness/gain, red/green/blue/white, preset colours,
  colour temperature with 3000/4500/6000 K, white/colour mode, all channels on/off.
- Reboot of selected devices (confirmation; `confirm: true` in the API), refresh
  paused 3 s like the original.
- API: `POST /api/v1/devices/{id}/command`, `POST /api/v1/devices/reboot`; Gen2+
  commands with the same GET/POST-RPC calls as ShellyScanner, including the JSON-RPC
  `auth` object for protected devices.
- Simulator: logs every request and keeps relay state, so commands are tested end to end.

### Fixed — Phase 3 readings found while porting the controls
- Gen1 Bulb and DUO RGBW are RGBCCT lights (colour/white mode), not RGBW.
- Wall Display shows its thermostat or its relay, as the original.
- The Pro Sensor Add-on digital output appears as an extra relay (Pro 1/1PM/2/2PM,
  Pro Dimmer, Pro EM).
- LinkedGo XT1 (ST1820, ST802) temperature, humidity and thermostat are read.
- Gen1 roller with a position above 100 is shown as not calibrated.

### Added — Phase 3: read-only device information
- All 17 device-table columns of ShellyScanner: status, type, device, name, keyword,
  MAC, IP, SSID, RSSI, cloud and MQTT (enabled/connected), uptime, internal
  temperature, measurements, logs, source and command (read-only state).
- Per-model parsing ported from ShellyScanner for every Gen1, Gen2, Gen3, Gen4 and
  BLU model (meters, temperature source, modules), Sensor Add-on and BLU sensors;
  tested against recorded fixtures of 16 real device types.
- Column chooser per view, default and detailed view, sorting, filter by column,
  selection helpers, status line, tooltips, keyboard shortcuts, double-click action,
  Web UI links.
- Device info panel (one tab per info request, stored data for sleeping devices) and
  logs (Gen1 files, Gen2+ live WebSocket with level filter and pause refresh).
- Display preferences per browser: uptime format, temperature unit, double-click
  action, default filter column.

### Added — Phase 2: discovery
- Device discovery: full mDNS scan (all interfaces, container bridges skipped), local
  mDNS scan on a chosen interface, IP scan of up to 10 ranges, offline mode (archive
  only). mDNS is a small browse-only implementation on golang.org/x/net, verified on
  a real network (43 instances, 25 Shellies of all generations).
- Identification of every ShellyScanner 1.3.4 model (27 Gen1, 33 Gen2, 28 Gen3, 15
  Gen4, BLU TRV and BTHome devices); unknown models as "Generic Gn", unreachable
  shelly* hosts as "Generic" with the error.
- Follow-up discovery: devices behind a range extender (ip:port) and BLU devices
  through Pro, Gen3 and Gen4 gateways, with ShellyScanner's rules for devices seen by
  several gateways.
- Status per device (on line, off line, not logged, updating, error, archived,
  reboot required), refreshed every 2 s (configuration every 5th time) while a
  browser is open and once a minute otherwise.
- Protected devices: Gen1 Basic and Gen2+ SHA-256 Digest authentication; a Login
  dialog and default credentials, stored encrypted, per device or global.
- Device archive (`/data/archive.json`): archived devices shown when not found,
  auto reload after 45 s, notes and keywords kept, remove from archive, clear.
- Devices page with live table, summary cards, Refresh and Rescan; Settings →
  Network and Archive; banner when no device answers mDNS (bridge networking).
- API: `/api/v1/devices`, `/scan`, `/credentials`, `/network/interfaces`, `/archive`.

### Added — Phase 1: skeleton
- Go server (`cmd/shellylanman`) on port 3082: `/healthz`, REST API under
  `/api/v1` (`about`, `status`, `settings`), WebSocket `/ws` with origin check,
  embedded frontend, security headers (CSP), graceful shutdown, `-healthcheck`
  for Docker.
- `/data` store: `secret.key` generated on first start, `settings.json` with
  AES-256-GCM-encrypted secrets, atomic writes.
- Themed web shell in TypeScript: sidebar (Devices, Checklist, Charts, Firmware,
  Deferred, Settings, About), connection indicator, first-run dialog, "UI
  authentication is off" banner, English and Dutch, per-browser appearance
  (17 palettes, dark/light, contrast and brightness, 7 fonts, font size) adapted
  from MikroDash.
- Simulated Shelly device (`cmd/shellysim`, `internal/sim`) serving recorded
  fixtures; `cmd/record` to record and scrub fixtures from real devices; first
  fixtures: Plug S (Gen1) and Plus 1 (Gen2).
- Multi-stage Dockerfile (Alpine runtime, non-root, amd64 + arm64), minimal
  `docker-compose.yml` with host networking, `tools/verify.sh`, GitHub Actions
  (checks on every push/PR, image publishing on version tags).
- Phase 0 analysis: `ARCHITECTURE.md`, `FEATURE_PARITY.md`, `DECISIONS.md`.
