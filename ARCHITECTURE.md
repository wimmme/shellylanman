# ShellyLanMan — Architecture

> Status: **Phase 0 design, decisions of 2026-09-26 applied** (`DECISIONS.md` §8). Nothing here is built.
> Companion documents: [`FEATURE_PARITY.md`](FEATURE_PARITY.md) (every feature, where it lives in the
> Java code, how it maps) and [`DECISIONS.md`](DECISIONS.md) (technology choice, UI mapping, licensing,
> firmware/QR, tests, plan, risks, open questions).

Sources analysed (shallow clones, 2026-09-26):

| Repository | Commit | Notes |
|---|---|---|
| usnasoft/shellyscanner | `a8e9b93` (2026-09-17), version 1.3.4 | 319 Java files, ~52k lines, GPL-3.0 |
| usnasoft/usnalib2 | `3784d9d` (2026-09-17) | Swing MVC framework + utilities, GPL-3.0 |
| SecOps-7/MikroDash | `2269c08` (2026-09-25) | Go + TypeScript, MIT |

Java paths below are relative to `src/main/java/it/usna/shellyscan/`.

---

## Part 1 — How ShellyScanner is built

### 1.1 Overview

ShellyScanner is a single-process Java 17 Swing desktop application in classic MVC style
(on top of usnalib2's `MainWindow`, `UsnaObservable`, `UsnaTableModel`, `AppProperties`).

```
Main.java ─┬─ reads ~/.shellyScanner (AppProperties) and CLI arguments
           ├─ non-interactive CLI modes (-backup/-restore/-list) → NonInteractiveDevices, then exit
           └─ interactive: Devices (model) + MainView (Swing) + DeferrablesContainer
                 │
                 ├─ discovery: JmDNS (_http._tcp.local.) or IP-range scan  → DevicesFactory
                 ├─ per-device refresh loop (ScheduledExecutorService, 128 threads)
                 ├─ events ADD / UPDATE / SUBSTITUTE / DELETE / READY / CLEAR → MainView, DeferrablesContainer, charts
                 └─ ghosts (archive) ← DevicesStore (JSON file ShellyStore.arc)
```

External libraries (pom.xml): `usnalib 1.3.1-SNAPSHOT`, `jackson-databind 3.2.2` (JSON),
`jmdns 3.6.3` (mDNS), `jetty-client 12.1.12` + `jetty-websocket-jetty-client` (HTTP, digest auth,
WebSocket), `jfreechart 1.5.6` (charts), `slf4j-simple`.

### 1.2 Packages

| Package | Role |
|---|---|
| `Main` | Bootstrap: settings, CLI parsing, scan mode, credentials, look-and-feel, starts model + view, update check |
| `controller/` | Swing actions (`Usna*Action`), `BackupAction`, `RestoreAction`, `ExportCSVAction`, `CLIController`, `DeferrableTask` + `DeferrablesContainer` (actions queued for offline devices) |
| `model/` | `Devices` (the live device list, discovery, refresh scheduling), `DevicesFactory` (identify & instantiate), `DevicesStore` (archive), `IPCollection` (IP ranges), `NonInteractiveDevices` (CLI model), exceptions |
| `model/device/` | `ShellyAbstractDevice` (base: HTTP GET, status, common fields), `GhostDevice` (archived/not-yet-seen device), `InetAddressAndPort`, `RestoreMsg`, `RestoreUtil`, marker interfaces |
| `model/device/g1/` | `AbstractG1Device`, `AbstractBatteryG1Device`, 27 Gen1 models, `modules/` (Relay, Roller, lights, Actions, FW/WiFi/MQTT/Login/Time/InputReset managers), `meters/` |
| `model/device/g2/` | `AbstractG2Device` (RPC over HTTP POST, digest auth, WebSocket), `AbstractProDevice`, `AbstractBatteryG2Device`, 33 Gen2 models, `modules/` (Script, KVS, Schedule, Webhooks, DynamicComponents, SensorAddOn, Input, Relay, Roller, lights, EM/EM1, CBreaker, LoRa, managers), `meters/` |
| `model/device/g3/` | `AbstractG3Device` (= G2 with generation "3"), 28 Gen3 models incl. "Powered by Shelly" (LinkedGo XT1, Ogemray), `modules/` (Camera, RGBCCT, XT1Thermostat) |
| `model/device/g4/` | `AbstractG4Device`, 15 Gen4 models (identified by `model`, not `app`) |
| `model/device/blu/` | `AbstractBTHomeDevice`, `BTHomeDevice`, `BluTRV`, `BLEDevice`/`BLEGateway` (unused draft), `modules/` (sensors, DW, motion, TRV schedule & firmware) |
| `model/device/modules/` | Capability interfaces: `RelayInterface`, `RollerInterface`, `WhiteInterface`, `RGB(W)Interface`, `CCTInterface`, `RGBCCTInterface`, `ThermostatInterface`, `InputInterface`, `CBreakerInterface`, `CameraInterface`, sensor interfaces, and the manager interfaces (`FirmwareManager`, `WIFIManager`, `MQTTManager`, `LoginManager`, `TimeAndLocationManager`, `InputResetManager`) |
| `model/device/meters/` | `Meters` (typed measurement set: W, VA, VAR, PF, V, VL, VX, I, FREQ, T…T4, H, HD, L, LE, LIGHT, EX, PERC, NUM, DMM, RAIN, VIB, ANG…, CHANNEL, BAT, BATE) |
| `view/` | `MainView` (toolbar, table, status bar, filter), `DevicesTable` + cell renderers/editors (the "Command" column is an interactive control per device), dialogs: device info, logs, authentication, deferrables, device selection, about, notes |
| `view/appsettings/` | Application settings dialog (General, Network, Archive, IDE tabs) |
| `view/devsettings/` | "Devices conf." dialog: FW Update, Wi-Fi 1, Wi-Fi 2, Restricted login, MQTT, Others |
| `view/checklist/` | Configuration checklist (eco, LED, logs, BLE, AP, roaming, Wi-Fi, extender, scripts, auto-FW) |
| `view/chart/` | Live measurement charts, CSV export of series, `-graphs` stdout mode |
| `view/scheduler/` | Cron schedule editors: Gen2+, Wall Display thermostat profiles, BLU TRV |
| `view/scripts/` | Scripts & KVS dialog, script IDE (`ide/`) |
| `view/lightsEditor/` | RGB / RGBW / white / CCT editors |
| `view/util/` | `ScannerProperties` (settings), `ApplicationUpdateCHK`, `Msg`, helpers |

### 1.3 Device model

- **Identity is the MAC address.** `ShellyAbstractDevice.equals()/hashCode()` use `mac` only. A device
  found again at a new IP replaces the old entry (`Devices.newDevice` → `SUBSTITUTE`).
- **Status** (`ShellyAbstractDevice.Status`): `ON_LINE`, `OFF_LINE`, `NOT_LOOGGED` (401), `READING`,
  `ERROR`, `GHOST` (known from archive, not seen this session). Status is set as a side effect of every
  HTTP call (`getJSON`, `executeRPC`, `sendCommand`).
- **Common fields**: hostname, mac, name, cloud enabled/connected, MQTT enabled/connected, log
  ("debug") mode, RSSI, SSID, uptime, reboot-required, last connection time, address+port.
- **Per-model classes** add: type ID/name, `Meters[]` (for the Measurements column), `DeviceModule[]`
  (for the Command column: relays, rollers, lights, inputs, thermostats…), internal temperature, and
  model-specific backup/restore (`restore(...)`, `restoreCheck(...)`).
- **Battery devices** (`BatteryDeviceInterface`) keep the last JSON of `/shelly`, settings, status and
  others (`getStoredJSON`), so info/backup/checklist can show "stored data" while the device sleeps.
- **Unmanaged devices** (`Shelly{G1,G2,G3,G4}Unmanaged`, `ShellyGenericUnmanagedImpl`,
  `ShellyBTHomeUnmanaged`) represent Shellies of unknown type or that failed to initialise; they keep
  the exception and are retried (`Devices.errorsReconnect`, 30 s after start).
- **BLU devices** are not reached over Bluetooth by the PC. They are **dynamic components of Gen2+
  gateways** (Pro, Gen3, Gen4, BLU Gateway): `bthomedevice:<id>` and `blutrv:<id>` keys from
  `Shelly.GetComponents?dynamic_only=true`. Their address is the gateway's (`BluInetAddressAndPort`,
  with alternative parents when several gateways see the same device).

### 1.4 Supported generations and device types

Identification happens in `DevicesFactory` from the `/shelly` response:

| Generation | Detected by | Discriminator | Classes |
|---|---|---|---|
| Gen1 | `gen` absent | `type` (e.g. `SHSW-1`) | 27 models + `ShellyG1Unmanaged` |
| Gen2 | `gen == 2` | `app` (e.g. `Plus1PM`), sometimes `model` (Pro 4PM vs Dual Cover, Pro Dimmer 1 vs 2) | 33 models + `ShellyG2Unmanaged` |
| Gen3 | `gen == 3` | `app`; for `XT1` also `svc0.type` | 28 models + `ShellyG3Unmanaged` |
| Gen4 | `gen == 4` | **`model`** (e.g. `S4SW-001P16EU`) | 15 models + `ShellyG4Unmanaged` |
| other gen | — | — | `ShellyGenericUnmanagedImpl` |
| BLU | gateway component key | `bthomedevice:` → `BTHomeDevice` (model from `attrs.model_id`), `blutrv:` → `BluTRV` | + `ShellyBTHomeUnmanaged` |

The complete per-model list (IDs and type names) is in `FEATURE_PARITY.md` §2.

### 1.5 Discovery

Code: `model/Devices.java`, `model/DevicesFactory.java`, `model/IPCollection.java`, `Main.java`.

Scan modes (setting `SCAN_MODE`, default `FULL`; overridable by CLI):

| Mode | What happens |
|---|---|
| **Full mDNS** (`FULL`, `-fullscan`) | `JmmDNS` on every network interface; listener for service type `_http._tcp.local.` (the `_shelly._tcp` listener is commented out). Interfaces added/removed at runtime are followed. |
| **Local mDNS** (`LOCAL`, `-localscan`) | One `JmDNS` bound to `InetAddress.getLocalHost()`; re-bound on rescan if the local address changed. |
| **IP scan** (`IP`, `-ipscan a.b.c.x-y`, `-ipscan1`…) | Up to 10 ranges `base.first-last` (last octet only). For each address, staggered 4 ms apart: `InetAddress.isReachable(10 s)`, then `GET http://ip:80/shelly` (80 s timeout); a Shelly is recognised by `"mac"` in the answer. |
| **Offline** (`OFFLINE`, `-noscan`) | No network scan at all; only the archive (ghost devices) is shown. |

Identification: `GET /shelly` → `gen`, `type`/`app`/`model`, `mac`, `auth`/`auth_en`. A device answering
mDNS whose hostname starts with `shelly`/`Shelly` but whose `/shelly` fails is still added as
"unmanaged with error" so it is visible.

Follow-up discovery from each new device:

- **Range extender**: for a Gen2+ with range extender enabled (or not logged in), `WiFi.ListAPClients`
  gives the ports on which clients behind the extender are reachable; each `ip:port` is probed with
  `/shelly` and created with hostname `<extender>-EX:<port>` (`RangeExtenderManager`).
- **BLU**: for Pro/Gen3/Gen4 (non-battery) devices, `Shelly.GetComponents?dynamic_only=true` (paged)
  lists `blutrv:*` and `bthomedevice:*` components, each becoming a BLU device.

Re-detection and offline handling:

- A periodic refresh per device (see 1.6) turns it `OFF_LINE` on timeout and back `ON_LINE` when it
  answers. There is no removal of devices that disappear; they stay in the list as offline.
- **Rescan** clears the list and scans again (with archive ghosts re-added if the archive is on).
- **Reload / Login** (context menu) recreates one device from its address (this is also how a
  `NOT_LOOGGED` device gets credentials).
- **Ghost auto-reload**: with archive + auto-reload enabled, 45 s after start every non-battery,
  non-BLU ghost is probed at its last known `ip:port` (catches devices mDNS did not announce).
- **Unmanaged retry**: 30 s after start, devices created with an error are re-created.

### 1.6 Network communication

- One shared Jetty `HttpClient` (max 8 connections per destination, 5 min idle) and one
  `WebSocketClient`.
- **Gen1**: HTTP GET REST (`/shelly`, `/settings`, `/status`, `/settings/...?...`, `/relay/0?turn=`…);
  HTTP Basic auth through Jetty's authentication store (`LoginManagerG1`).
- **Gen2+**: `GET /rpc/<Method>?...` for reads, `POST /rpc` with `{"id":1,"method":...,"params":...}`
  for writes. Digest auth: user always `admin`; on 401 the challenge from the response body is answered
  with an `auth` object (SHA-256, `ha2 = sha256("dummy_method:dummy_uri")`) — `LoginManagerG2.getAuthNode`.
  GETs use Jetty's `DigestAuthentication`.
- **WebSocket** (`ws://host/rpc`): used for firmware-update progress (`NotifyEvent` `ota_progress`,
  `ota_success`, `scheduled_restart`) and for Gen2+ live logs (`ws://host/debug/log`, with auth passed
  as query parameters on protected devices).
- **Pacing**: `Devices.MULTI_QUERY_DELAY = 59 ms` is slept between consecutive calls to the same device
  ("too many calls disturb some devices, especially Gen1").
- **Refresh loop**: per device, `scheduleWithFixedDelay(interval + index ms, interval)`; every run calls
  `refreshStatus()`, every `refreshTics`-th run also `refreshSettings()`. Defaults: status every **2 s**,
  config every **5** status refreshes. Gen1: `/status` and `/settings`; Gen2+: `Shelly.GetStatus` and
  `Shelly.GetConfig`; BLU: via the gateway.
- **Paging** (Gen2+): `offset`/`total` handled by `JsonPageIterator` / `getPagedJson` for
  `Shelly.GetComponents`, `KVS.GetMany`, `BLE.CloudRelay.ListInfos`, etc.
- **Internet**: only `https://www.usna.it/shellyscanner/last_version.txt` (application update check,
  setting `UPDATE_CHK`) and links to the online manual. Firmware checks are done **by the devices**
  (`/ota/check`, `Shelly.CheckForUpdate`), not by the application.

The full list of Shelly API calls, by feature, is in `FEATURE_PARITY.md` §3.

### 1.7 Persistence

| What | Where | Format |
|---|---|---|
| Settings | `~/.shellyScanner` | Java properties (usnalib2 `AppProperties`): scan mode, IP ranges, refresh intervals, UI options, column layout, window geometry, IDE options, last paths, restricted-login credentials (**Base64, not encrypted** — the UI warns "credentials are saved in a not secured format") |
| Archive ("store") | `~/ShellyStore.arc` (configurable) | JSON `{ver:0, time, dev:[{tid,tn,host,mac,ip,port,name,ssid,last,bat,gen,note,keyword}]}` — written on exit; notes and keyword live only here |
| Device backups | user-chosen path, `<hostname>.sbk` | ZIP of JSON sections (+ `*.mjs` scripts) — see `FEATURE_PARITY.md` backup rows |
| Deferred actions | memory only | lost on exit |
| Charts | memory only (while the chart window is open) | exportable to CSV |

### 1.8 Threading

`ScheduledExecutorService` with 128 threads for the model, 35 for the FW panel, 64 in CLI mode;
Swing EDT for the view. Model events are fired from worker threads and marshalled with
`SwingUtilities.invokeLater`. Mutable device objects are read by the view without locking (acceptable
in Swing, not something to copy).

---

## Part 2 — Proposed architecture for ShellyLanMan

The technology recommendation (Go backend, TypeScript frontend) is argued in `DECISIONS.md` §1 and is
**not yet decided**. The structure below is written for Go; it would look the same in another
language, with different package names.

### 2.1 Principles that shape it

1. **One service layer, several clients.** Everything a user can do is a method on the service layer.
   The HTTP/WebSocket API is a thin adapter over it; the web UI is one client of that API; a future MCP
   server and Home Assistant integration are further thin adapters. No business logic in handlers or in
   the browser.
2. **Read before write.** Protocol clients and parsers first; every write path goes through a single
   place (`service`), so confirmation, deferral and audit hooks exist once.
3. **Data-driven device types where the protocol allows it.** Gen2+ status and config are component
   keyed (`switch:0`, `cover:0`, `light:0`, `em:0`, `temperature:100`…), so one generic parser covers
   most of the ~75 Gen2+ Java classes; a model registry holds the per-model facts (type name, expected
   components, quirks). Gen1 needs more per-model code because its REST payloads differ per type. Every
   Java class is still mapped explicitly (§2.9), so nothing gets lost in the generalisation.
4. **Faithful pacing.** One request in flight per device and a ~59 ms gap between requests to the same
   device, like the original. Many devices are polled concurrently.
5. **Two polling modes (Q8).** The configured refresh rate (status every 2 s, config every 5th refresh by
   default) applies only while at least one browser is connected over WebSocket. Without viewers a slow
   presence check (proposal: 60 s) keeps online/offline status, deferred tasks and the chart ring buffer
   going without loading the devices 24/7.
6. **Credentials (Q9).** A global default credential set plus optional per-device credentials, all
   encrypted in `/data/settings.json`; the per-device one wins. Never returned by the API.

### 2.2 Component diagram

```
                          ┌───────────────────────── browser (TypeScript SPA) ─────────────────────────┐
                          │  pages: Devices · Checklist · Charts · Firmware · Deferred · Settings · About │
                          └───────────────▲──────────────────────────────▲───────────────────────────────┘
                                          │ REST /api/v1/*              │ WebSocket /ws (events)
┌─────────────────────────────────────────┴──────────────────────────────┴─────────────────────────────┐
│ internal/httpapi   routes, JSON, auth (optional), origin check, /healthz, static files, WS hub       │
│ internal/mcp           MCP tools → same service calls         (later) HA: see §2.8                    │
├──────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ internal/service   Devices · Control · Config (Wi-Fi/MQTT/Login/Others) · Backup · Restore · Firmware │
│                    Checklist · Scripts · KVS · Schedules · Logs · Charts sampler · Deferred · Archive │
├───────────────────────┬──────────────────────┬─────────────────────┬─────────────────────────────────┤
│ internal/discovery    │ internal/poller      │ internal/firmware   │ internal/store                  │
│ mdns · ipscan ·       │ per-device refresh,  │ index (gen1/gen2+), │ /data: settings, key, secrets,  │
│ extender · blu        │ pacing, status       │ cache, QR           │ archive, deferred, backups      │
├───────────────────────┴──────────────────────┴─────────────────────┴─────────────────────────────────┤
│ internal/model     Device, Status, Meters, Modules, model registry (per-generation descriptors)       │
│ internal/shelly    gen1 client (REST, Basic) · rpc client (Gen2+, digest SHA-256, WS) · blu via gateway│
└──────────────────────────────────────────────────────────────────────────────────────────────────────┘
                                   │ HTTP / WebSocket / mDNS on the LAN
                               Shelly devices (Gen1, Gen2, Gen3, Gen4, BLU via gateways)
```

### 2.3 Service layer and API

The service layer is plain Go with no HTTP types in its signatures. Sketch of its surface (names
provisional):

| Service | Operations |
|---|---|
| `Devices` | `List(filter)`, `Get(id)`, `Rescan()`, `Refresh(id / all)`, `Reload(id)` (= Login), `Reboot(ids)`, `Remove(ghost id)`, `Info(id)` (raw JSON per info request), `Events()` |
| `Control` | `Relay(id, ch, on/toggle)`, `Roller(id, ch, open/close/stop/position)`, `Light(id, ch, on, brightness, rgb, white, temp, gain, mode)`, `Thermostat(id, target/enable)`, `Input(id, ch, event)`, `Breaker(id, …)`, `CameraPrivacy(…)` |
| `Config` | Wi-Fi 1/2/AP, MQTT, restricted login, NTP, cloud, input-reset, eco, LED, logs mode, BLE, AP, roaming, range extender, auto-FW — each for **a selection** of devices, returning per-device results (`ok / fail(reason) / queued / excluded`) |
| `Backup` / `Restore` | backup selection → `.sbk` files in `/data/backups` (and download); restore check → questions; restore with answers |
| `Firmware` | per-device current/stable/beta; update to stable/beta (with progress events); latest-stable download + QR (new feature) |
| `Scripts` / `KVS` / `Schedules` | list/get/put/create/delete/start/stop; KVS get/set/delete; schedule CRUD for Gen2+, Wall Display thermostat, BLU TRV |
| `Logs` | Gen1 `/debug/log(1)` snapshot; Gen2+ live log stream (proxied WebSocket) |
| `Charts` | sample selected metric for selected devices; CSV export |
| `Deferred` | list, cancel; tasks run automatically when a device comes back `ON_LINE` |
| `Archive` | notes + keyword per device; ghost list; import of a ShellyScanner `.arc` file |
| `Settings` | application settings (scan mode, ranges, intervals, credentials, CSV separator, uptime/temperature format…) |

HTTP API: REST under `/api/v1/` (JSON), one WebSocket `/ws` for server→client events
(`device.added|updated|substituted|removed`, `scan.ready`, `deferred.changed`, `firmware.progress`,
`log.line`). Browser→server actions go over REST, not over the socket, so the API is equally usable
by `curl`, an MCP adapter or Home Assistant. The API will be documented as an OpenAPI file once it
settles (Phase 2–3).

### 2.4 Repository layout (proposed)

```
cmd/shellylanman/        main: flags/env, wiring, graceful shutdown (no CLI subcommands, Q14)
cmd/shellysim/           the simulated Shelly device(s) for tests and demos
internal/shelly/         protocol clients: gen1/, rpc/ (incl. digest), ws/
internal/model/          device model, meters, modules, registry/ (gen1.go, gen2.go, gen3.go, gen4.go, blu.go)
internal/discovery/      mdns.go, ipscan.go, extender.go, blu.go
internal/poller/         refresh scheduling and pacing
internal/service/        one file (or small package) per service in §2.3
internal/firmware/       firmware.go (index, pinned TLS, verified file cache); QR in service/localfw.go
internal/store/          settings, secrets (AES-256-GCM), archive, deferred, paths
internal/httpapi/        router, handlers, ws hub, auth, origin, health
web/src/                 TypeScript: pages/, ui/ (components), api.ts, socket.ts, appearance.ts
web/public/              CSS (MikroDash-derived tokens), fonts, icons
testdata/                fixtures: gen1/<type>/, gen2/<app>/, gen3/…, gen4/…, blu/…, backups/, archive/
docs/                    brief.md, api/ (OpenAPI), analysis notes
Dockerfile, docker-compose.yml, Makefile or tools/verify.sh
README.md, CLAUDE.md, CHANGELOG.md, SECURITY.md, THIRD_PARTY_NOTICES.md, CONTRIBUTING.md, LICENSE (GPL-3.0)
ARCHITECTURE.md, FEATURE_PARITY.md, DECISIONS.md
```

### 2.5 Docker and networking model

**Deliberate choice (proposed): host networking is the documented default; bridge networking is a
supported alternative with IP scan.**

| Mode | mDNS discovery | IP scan | Device control, WS, FW | Notes |
|---|---|---|---|---|
| `network_mode: host` (**default**) | ✅ works, same as the desktop app ("full scan" = all host interfaces) | ✅ | ✅ | Linux only (Docker Desktop on macOS/Windows does not give real host networking). Port is the host's port; must not collide. |
| bridge (default Docker network) | ❌ multicast from the LAN does not reach the container; mDNS replies to the container's queries do not come back | ✅ (routed via NAT; devices see the Docker host's IP) | ✅ | Works everywhere. The UI must say clearly that mDNS is unavailable and propose IP scan. |
| macvlan / ipvlan | ✅ container has its own LAN IP, multicast works | ✅ | ✅ | Needs a free LAN IP and host-specific setup; host cannot talk to the container without extra config. Documented as an advanced option, not supported by default. |
| bridge + mDNS reflector (avahi `enable-reflector` on the host) | ⚠️ possible | ✅ | ✅ | Extra moving part on the host, fragile; **not recommended**, mentioned only for completeness. |

The application detects at start whether mDNS is usable (it can send a query and see any response,
including its own) and shows a banner in bridge mode instead of silently finding nothing.

`InetAddress.isReachable` in the original is only a pre-filter before `GET /shelly`; without root it is
a TCP connect to port 7 anyway. ShellyLanMan skips it and uses a short connect timeout on port 80, so
no `NET_RAW` capability is needed. (Recorded as a deliberate difference in `FEATURE_PARITY.md`.)

Image: multi-stage build; final stage `alpine` (or distroless static) + `ca-certificates` + `tzdata`,
one static binary with the web assets embedded. Target image size: ~20–30 MB. One volume: `/data`.
Platforms: `linux/amd64` and `linux/arm64`. Default port **3082** (`SHELLYLANMAN_LISTEN` to change).
Runs as a non-root user. No `.env` needed: everything is configured in the browser; a few environment
variables exist for things that must be known before the UI (listen address, allowed origins, trusted
proxies).

### 2.6 `/data` contents (proposed, exhaustive)

| Path | Content | Sensitive |
|---|---|---|
| `/data/secret.key` | 32 random bytes, generated on first start (mode 0600) | yes |
| `/data/settings.json` | application settings (the equivalent of `~/.shellyScanner`, minus window geometry); secret fields (device credentials, optional UI password hash) stored encrypted with AES-256-GCM using `secret.key` | encrypted fields |
| `/data/archive.json` | the device archive: last known identity, address, type, SSID, last-seen time, battery flag, **notes and keyword** (equivalent of `ShellyStore.arc`) | no (but contains your network layout) |
| `/data/deferred.json` | queued actions for offline devices, persisted across restarts (Q10) | may contain Wi-Fi/MQTT passwords → encrypted |
| `/data/backups/` | device backups `<hostname>-<timestamp>.sbk` (same ZIP layout as ShellyScanner where practical); the last N per device are kept, N configurable, default 10 (Q12) | **yes** — backups contain Wi-Fi SSIDs, MQTT settings, scripts, KVS (same as in the original) |
| `/data/firmware/` | cached firmware files + small index cache (for the QR download feature) | no |
| `/data/users.json` or a field in settings | only if authentication is enabled: password hash (scrypt/argon2id) | yes |

Nothing else is written. Logs go to stdout. Chart samples are held in an in-memory ring buffer (Q7), not on disk. Honest note for `SECURITY.md`:
the key lives in the same volume as the data it protects; encryption at rest protects copies of
`settings.json` (e.g. in a support request or a backup of just that file), not someone who has the
whole volume.

### 2.7 Security model

- **Authentication optional, off by default** (LAN tool, same as MikroDash); a warning is logged at
  start and shown in the UI while it is off. When on: one local admin password (scrypt/argon2id),
  session cookie (HttpOnly, SameSite=Strict).
- **WebSocket origin check**: `Origin` must match the `Host` or an allow-list (`SHELLYLANMAN_ORIGINS`).
- **CSRF**: state-changing requests require the JSON content type plus the same-origin check.
- **Reverse proxy guidance** in the README (TLS termination, WebSocket upgrade headers, trusted proxies).
- Device passwords: never sent back to the browser; write-only fields.
- **Destructive actions** (reboot, restore, firmware update, factory-reset-related settings, deletes)
  always need a confirmation dialog in the UI; the API requires an explicit `confirm: true` on these so
  a script cannot trigger them by accident either.

### 2.8 MCP (built, DECISIONS §18) and room for Home Assistant

**MCP server.** An `internal/mcp` package in the same binary exposes tools such as `list_devices`,
`get_device`, `set_relay`, `backup_device`, `check_firmware` over MCP's Streamable HTTP transport at
`/mcp`, sharing authentication with the web UI. Each tool is a few lines that call the service layer,
so it has exactly the behaviour, pacing and deferral logic of the UI. Destructive tools require the same
explicit confirmation flag.

**Home Assistant — recommended approach: a small custom integration ("discovery hand-off") against our
API, not MQTT discovery.**

| Option | How it works | Assessment |
|---|---|---|
| **A. Discovery hand-off** (recommended) | A HACS custom integration reads ShellyLanMan's device list (and optionally the stored per-device credentials) and starts config flows of HA's **official Shelly integration** for each device, pre-filled | Solves the actual pain (manual adding per device) while HA keeps talking to devices directly with its mature integration; ShellyLanMan is not in the data path, so it can be down without breaking automations. Needs HA's config-flow API; must be verified. |
| B. Proxy entities | Custom integration that creates entities from ShellyLanMan's API/WebSocket | ShellyLanMan becomes a single point of failure and must re-implement everything HA's Shelly integration already does well. |
| C. MQTT discovery | ShellyLanMan publishes HA discovery messages and bridges state to a broker | Requires a broker; ShellyLanMan becomes a bridge in the data path; duplicates the official integration; Shelly devices can already speak MQTT themselves. |

What the architecture needs for this now: stable device IDs (MAC), a documented read API, and events
over WebSocket. Nothing HA-specific is built.

### 2.9 Java → ShellyLanMan component mapping

| Java component | New counterpart | Category (`DECISIONS.md` §3.2: 2 ported · 3 clean · 4 new) |
|---|---|---|
| `Main` | `cmd/shellylanman/main.go` (flags/env, wiring) | new (4) |
| `CLIController`, `NonInteractiveDevices` | REST API only; no CLI (Q14) | ported (2) |
| `model/Devices` | `internal/service` Devices + `internal/poller` + `internal/discovery` | ported (2) + clean (3) |
| `model/DevicesFactory` | `internal/model/registry` + `discovery.identify()` | ported (2) (tables carry the same IDs) |
| `model/DevicesStore` | `internal/store/archive.go` (no `.arc` import, Q11) | ported (2) |
| `model/IPCollection` | `internal/discovery/ipscan.go` (ranges) | ported (2) |
| `model/device/ShellyAbstractDevice` | `internal/model.Device` + `internal/shelly` clients | ported (2) + clean (3) |
| `GhostDevice` | `model.Device` with `Status = Ghost` from the archive | ported (2) |
| `InetAddressAndPort`, `BluInetAddressAndPort` | `model.Address{IP, Port}` + `model.BLUParents` | ported (2) |
| `g1/AbstractG1Device`, `AbstractBatteryG1Device` | `internal/shelly/gen1` + `model/registry/gen1.go` | ported (2) |
| `g1/*` (27 models) | one registry entry + parser per Gen1 type | ported (2) |
| `g2/AbstractG2Device`, `AbstractProDevice`, `AbstractBatteryG2Device`, `g3/AbstractG3Device`, `g4/AbstractG4Device` | `internal/shelly/rpc` + generic component parser | ported (2) |
| `g2/*`, `g3/*`, `g4/*` (76 models) | registry entries (app/model → type name, components, quirks) | ported (2) |
| `blu/*` | `internal/discovery/blu.go` + `model/registry/blu.go` | ported (2) |
| `modules/*Interface` | `model.Module` kinds (Relay, Roller, Light{White,RGB,RGBW,CCT,RGBCCT}, Thermostat, Input, Breaker, Camera, sensors) | ported (2) |
| `meters/Meters` | `model.Meters` with the same type set | ported (2) |
| `g1/modules/FirmwareManagerG1`, `g2/modules/FirmwareManagerG2`, `blu/modules/FirmwareManagerTRV` | `internal/service/firmware.go` | ported (2) |
| `*/WIFIManager*`, `MQTTManager*`, `LoginManager*`, `TimeAndLocationManager*`, `InputResetManager*`, `RangeExtenderManager` | `internal/service/config_*.go` | ported (2) |
| `LoginManagerG2` digest | `internal/shelly/rpc/digest.go` | clean (3), from the Shelly auth spec |
| `g2/modules/Script`, `KVS`, `ScheduleManager*`, `Webhooks`, `DynamicComponents`, `SensorAddOn*`, `LoRaAddOn` | `internal/service/{scripts,kvs,schedules}.go`, restore helpers | ported (2) |
| `g1/modules/Actions` | Gen1 restore helper | ported (2) |
| `controller/BackupAction`, `RestoreAction`, `RestoreMsg`, `RestoreUtil` | `internal/service/{backup,restore}.go` + restore wizard modal | ported (2), same `.sbk` format |
| `controller/DeferrableTask`, `DeferrablesContainer` | `internal/service/deferred.go` + "Deferred" page/badge | ported (2) |
| `controller/ExportCSVAction` | CSV export (server or browser) | ported (2) |
| `view/MainView` (toolbar, status bar, filter, selection) | app shell (sidebar, topbar) + Devices page (toolbar actions, filter, selection menu, status line) | new (4) |
| `view/DevicesTable` + renderers | Devices table component (sortable, filterable, column chooser, default/detailed view) | new (4) |
| `view/DevicesCommandCellRenderer/Editor` | "Command" cell controls (switch, roller buttons/slider, light popover, thermostat…) | new (4) |
| `view/DialogDeviceInfo` | Device info modal/panel (tabs per info request, JSON viewer, auto refresh) | new (4) |
| `view/DialogDeviceLogsG1/G2` | Logs panel (snapshot / live stream) | new (4) |
| `view/DialogAuthentication` | Login modal | new (4) |
| `view/DialogDeferrables` | Deferred actions page | new (4) |
| `view/NotesEditor` | Notes modal (note + keyword) | new (4) |
| `view/DialogAbout` | About page (credits usnasoft, licences, trademark note) | new (4) |
| `view/appsettings/*` | Settings page (tabs: General, Network, Archive, Script editor, Appearance, Security) | new (4) |
| `view/devsettings/*` | "Devices configuration" modal (tabs: FW update, Wi-Fi 1, Wi-Fi 2, Restricted login, MQTT, Others) | new (4) |
| `view/checklist/*` | Checklist page | new (4) |
| `view/chart/*` | Charts page (Chart.js) + CSV export; `-graphs` → API stream | new (4) |
| `view/scheduler/*` | Scheduler modal (cron editor with hints; WD thermostat; BLU TRV) | new (4) |
| `view/scripts/*`, `scripts/ide/*` | Scripts & KVS modal with a code editor | new (4) |
| `view/lightsEditor/*` | Light editor popover/modal | new (4) |
| `view/util/ScannerProperties` | `internal/store/settings.go` + per-browser appearance (localStorage) | ported (2) |
| `view/util/ApplicationUpdateCHK` | opt-in GitHub release check for ShellyLanMan, off by default (Q18) | new (4) |
| usnalib2 (`UsnaObservable`, `UsnaTableModel`, `AppProperties`, Swing widgets) | not needed (Go channels / TS components) | — |
