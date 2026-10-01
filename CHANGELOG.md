# Changelog

All notable changes. Format based on [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
