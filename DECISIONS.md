# ShellyLanMan — Decisions

> Status: **Phases 0–10 done, v0.2.0 released (2026-09-28); MCP server added after it (§18).** Sections 1–7 hold the
> Phase 0 analysis and proposals; where §8 or a later section differs, the later decision wins.
> Related: [`ARCHITECTURE.md`](ARCHITECTURE.md), [`FEATURE_PARITY.md`](FEATURE_PARITY.md).

---

## 1. Technology

### 1.1 What the backend has to do

Poll 10–200 devices concurrently with per-device pacing; HTTP client with Basic and SHA-256 digest
auth; WebSocket client (to devices) and server (to browsers); mDNS browsing on the host's interfaces;
ZIP read/write (`.sbk` backups); JSON everywhere; AES-GCM for secrets; serve a static SPA; run 24/7 in a
small container on amd64, arm64 and ideally armv7 (Raspberry Pi class).

### 1.2 Comparison

Figures are **estimates** from typical deployments of each stack, not measurements of this app; they
are there to show the order of magnitude.

| Criterion | **Go** (stdlib `net/http`) | **Java** Javalin (+ jlink JRE) | Java Spring Boot | **Node.js/TS** (Fastify) | **Python** (FastAPI/uvicorn) |
|---|---|---|---|---|---|
| Simplicity | High: one binary, stdlib covers HTTP/JSON/ZIP/crypto | Medium: small framework, but JVM tuning and packaging | Low: large framework, lots of convention | Medium: simple, but tooling layers (tsc, bundler, runtime) | High for code; packaging/async model add some friction |
| Community size | Large | Very large | Very large | Very large | Very large |
| Maintainability (long run) | High: strong compatibility promise, few deps, `gofmt` | High | Medium: frequent major upgrades | Medium: npm dependency churn | Medium: dependency churn, typing optional |
| Image size | ~25–30 MB | ~90–130 MB | ~180–280 MB | ~60–90 MB | ~70–120 MB |
| Idle memory | ~15–40 MB | ~80–200 MB | ~200–400 MB | ~50–100 MB | ~50–100 MB |
| Startup | < 1 s | 1–3 s | 3–10 s | < 1 s | 1–2 s |
| Multi-arch incl. armv7 | Excellent (cross-compile from one builder, static binary) | Good (arm32 JREs exist, fewer vendors) | Same as Java | Good (official armv7 images) | Good |
| HTTP client/server | stdlib, excellent | Jetty/Javalin, excellent | excellent | excellent | good (httpx/aiohttp) |
| WebSocket | `coder/websocket` (ISC, small) | Jetty (as original) | built in | `ws` | built in (Starlette) |
| mDNS/multicast | ⚠️ libraries exist (hashicorp/mdns, brutella/dnssd, grandcat/zeroconf); none as proven as JmDNS/python-zeroconf — **the weak spot** | JmDNS (what the original uses) | JmDNS | `multicast-dns` / `bonjour-service`, adequate | **python-zeroconf** (used by Home Assistant), excellent |
| Concurrency for polling | Excellent (goroutines, contexts, timeouts) | Excellent (virtual threads on 21+) | Excellent | Good (async I/O, single thread) | Good (asyncio), CPU-bound parts weaker |
| Testability | Excellent (`httptest`, table tests, race detector, one command) | Excellent (JUnit) | Excellent | Good | Excellent (pytest) |
| Readability for newcomers | High, deliberately plain language | High | Medium (annotations/magic) | High | High |
| Fit for small self-hosted app | Excellent | Good | Poor (heavy) | Good | Good |
| Ease of community contribution | Good; Go common in self-hosting/infra | Good; **original author's language** | Good | Very good | Very good |
| Reuse of original's logic | Translation (still derivative) | **Highest**: model and restore code could be ported almost 1:1 after removing Swing couplings (e.g. `DevicesFactory` opens a Swing dialog) | High | Translation | Translation; **aioshelly** (Apache-2.0, HA's Shelly library) could replace much protocol code — but then behaviour follows HA, not ShellyScanner |
| Consistency with MikroDash (the quality reference) | Same stack (Go + TS), same patterns and tooling | Different | Different | TS shared, backend different | Different |

### 1.3 Recommendation: Go backend + TypeScript frontend

Weighting per the brief: maintainability and footprint first, community second, closeness to the
original's language third.

- **Footprint**: Go wins clearly (≈4× smaller image and ≈3–5× less memory than a lean JVM), which
  matters for a 24/7 container on a Pi next to other services.
- **Maintainability**: one static binary, stdlib for almost everything, ~4–6 direct dependencies, and a
  language whose code from ten years ago still compiles. Easy for "me, Claude and a community".
- **Community**: Go is large and is the lingua franca of self-hosted infrastructure; not the largest
  of the five, but no weakness.
- **Closeness to the original**: Java would let the original author read and contribute more easily, and
  would allow the closest port. The cost — heavier image and runtime, JVM packaging — is real but not
  prohibitive; **if the author's direct participation is a priority for you, Javalin + jlink is the
  credible alternative.** Otherwise the original gets full credit as the functional and code reference
  (see §3).
- **The one weak spot, mDNS**, is contained: it is one package behind an interface. Phase 2 starts with a
  spike comparing `hashicorp/mdns` and `brutella/dnssd` against real devices; if neither is solid, a
  small browse-only implementation on `golang.org/x/net/dns/dnsmessage` (~300 lines, tested with
  recorded packets) is the fallback. Only browsing/resolving `_http._tcp` is needed, not advertising.

### 1.4 Frontend

- **TypeScript, no framework**, the way MikroDash does it: a small set of own DOM helpers and components
  (table, modal, tabs, toast, confirm), one module per page. Bundled with esbuild. Runtime deps only
  where they clearly pay for themselves:
  - **Chart.js** (MIT) for charts — also what MikroDash uses.
  - **CodeMirror 6** (MIT) for the script editor, loaded only on the Scripts panel; it gives the
    original IDE's features (auto-indent, bracket matching/closing, block comments, find/replace, dark
    mode, completion) without writing an editor.
  - QR codes are rendered **server-side** (Go, `skip2/go-qrcode`, MIT) so the API/MCP can use them too.
- Alternative worth naming: **Preact + htm** (MIT, ~5 KB) would reduce boilerplate in the ~20 dialogs
  and forms. I lean towards plain TS for consistency with MikroDash and zero framework churn, and would
  revisit only if the dialog code gets repetitive (question Q2).
- Build: in the Dockerfile only (Node stage for `tsc --noEmit` + esbuild, or esbuild's Go API as
  MikroDash does). The host needs only Docker.

### 1.5 Proposed dependencies

| Dependency | Why | Licence |
|---|---|---|
| Go stdlib | HTTP, JSON, ZIP, AES-GCM, templates, embed | BSD-3-Clause |
| `github.com/coder/websocket` | WebSocket server (browsers) and client (device logs, FW progress) | ISC |
| mDNS library (to be chosen in Phase 2) | discovery | MIT (both candidates) |
| `golang.org/x/crypto` | argon2id/scrypt for the optional UI password | BSD-3-Clause |
| `github.com/skip2/go-qrcode` | QR code for the firmware feature | MIT |
| TypeScript, esbuild (build only) | frontend build | Apache-2.0, MIT |
| Chart.js | charts | MIT |
| CodeMirror 6 | script editor | MIT |
| Fonts (vendored subset, no CDN) | UI font(s), mono font | SIL OFL 1.1 |
| Playwright (CI only, optional) | browser smoke tests | Apache-2.0 |

No database: the state is small and file-shaped (settings, archive, backups). SQLite only if a later
feature needs structured history (see Q7).

---

## 2. UI: ShellyScanner functions in MikroDash's visual language

### 2.1 What I plan to reuse from MikroDash (MIT, attributed in `THIRD_PARTY_NOTICES.md`)

- **Design tokens and palettes** from `web/public/app.css`: the CSS custom properties
  (`--bg-deep`, `--bg-card`, `--border`, `--accent-*`, `--text-main/muted`, `--nav-*`) and the named
  palettes with dark/light variants (default, nord, catppuccin, dracula, tokyo, gruvbox, rosé pine,
  one dark, solarized, everforest, kanagawa, monokai, material, palenight, github).
- **Appearance layer** (`web/src/appearance.ts` + its tables): palette, theme, contrast, text and
  background brightness, font and font size, per browser in `localStorage`, applied before first paint.
- **Component styles/patterns**: collapsible sidebar (icon rail ↔ labelled), top bar, card with title +
  count badge (`.card-badge`), summary cards, data tables with sort indicators, row hover, focus ring
  (`:focus-visible`), status pills, confirmation modal, loading/empty/error/stale states, WebSocket
  reconnect behaviour.
- **Engineering practice**: `CLAUDE.md` structure, one verify command, `CHANGELOG.md`, `SECURITY.md`,
  `THIRD_PARTY_NOTICES.md` discipline (licence before asset), `/healthz`, GHCR publishing on version tags
  only, multi-arch buildx with the Go toolchain pinned to the build platform.
- **Not reused**: MikroDash's application code (RouterOS-specific), Tabler CSS (not needed for our
  smaller surface), geo data, AI features.

### 2.2 Mapping

| ShellyScanner element | MikroDash-style web equivalent |
|---|---|
| Main window with toolbar, table, status bar | **App shell**: sidebar (Devices · Checklist · Charts · Firmware · Deferred · Settings · About), top bar (scan state, Rescan, Refresh, deferred badge) |
| Status line "N devices listed – M selected" | **Summary cards** above the table: total, online, offline, not logged, reboot required, updates available, stored (ghost) — counts only; plus "N listed / M selected" in the table card header |
| Devices table (17 columns, sort, filter, column chooser, default/detailed view) | **Table page** "Devices" in a card: sticky header, sortable columns, search box + column selector (All/Type/Device/Name/Keyword), column menu, view-mode toggle, same column names |
| Status icons (online, offline, login, updating, error, ghost, reboot-required) | **Status pills**: green online, red offline, amber not logged, blue pulsing reading, red outline error, grey stored, online + ↻ marker for reboot required |
| Cloud/MQTT "En/Con" columns | Two small badges (enabled / connected) |
| Command column (relays, rollers, lights, thermostat, inputs) | Inline controls in the cell: toggle switches, ▲ ■ ▼ + position slider, brightness slider + colour swatch opening a **light popover**, thermostat stepper |
| Toolbar actions acting on the selection | **Page toolbar** above the table (enabled per selection exactly like `MainView.rowsSelectionManager`): Info, Logs, Scheduler, Charts, Checklist, Web UI, Backup, Restore, Devices conf., Scripts, Reboot, Notes; right side: View mode, Export CSV, Print |
| Context menu (device / ghost) | Row **⋯ menu**: Info, Web UI, Devices conf., Backup, Restore, Notes, Reload/Login; ghosts: Reload, Notes, Remove from archive |
| Selection helpers | "Select ▾" menu (all, online, reboot required, Gen1, Gen2+, Wi-Fi, BLU, stored) |
| Device info dialog (tabs of JSON) | **Right-side detail panel** (or modal on mobile) with tabs per info request, collapsible JSON viewer, copy button, auto refresh |
| Logs dialogs | Detail-panel tab "Logs": snapshot (G1) or live stream with pause/clear/copy (G2+) |
| Devices conf. dialog (FW, Wi-Fi 1/2, login, MQTT, Others) | **Large modal with tabs**, same tab names and fields; result list with coloured per-device outcome |
| Restore flow (questions, password prompts, warnings, reboot offer) | **Restore wizard modal**: file (stored/uploaded) → checks & warnings → passwords/script choices → run → results + optional reboot |
| Checklist window | **Checklist page**: table with ✓/✗/number cells, same columns, row actions toolbar, filter |
| Charts window | **Charts page**: chart card + controls (type, range, series, pause, markers, export) |
| FW update panel | **Firmware page** (all devices) and the FW tab in Devices conf. (selection), with progress cells and the QR button |
| Deferred actions dialog + status-bar button | **Deferred page** (table: time, device, action, status, message, cancel) + sidebar/top-bar badge with waiting count and success/fail colour |
| Scripts & KVS dialog, script IDE | **Full-height modal** with Scripts / KVS tabs; script editor (CodeMirror) with run/stop, save, upload/download |
| Scheduler dialogs | Modal with rule list + cron editor (G2+), profile/rule editor (Wall Display), TRV rules |
| Notes editor | Small modal (note + keyword) |
| Application settings dialog | **Settings page** with tabs: General, Network, Archive, Script editor, Appearance (MikroDash), Security (optional password) |
| About dialog | **About page**: version, credits to usnasoft/ShellyScanner with link, MikroDash credit, licences, trademark and independence notice |
| Confirm dialogs (reboot, delete notes, etc.) | MikroDash confirmation modal; destructive actions use a red primary button with the action's name |

---

## 3. Licensing analysis and attribution plan

*Technical documentation, not legal advice.*

### 3.1 Licences found (checked in the repositories, not assumed)

| Project | Licence evidence | Notes |
|---|---|---|
| ShellyScanner | `COPYING` = GNU GPL v3 text; About text: "Shelly Scanner is distributed under GNU General Public License v3.0"; author Antonio Flaccomio (usnasoft) | Source files carry **no per-file copyright headers**; attribution therefore goes in README, `NOTICE`/`THIRD_PARTY_NOTICES.md`, About page, and headers of our files that are ported |
| usnalib2 | `COPYING` = GNU GPL v3 text | We do not need it (Swing framework + utilities). If any utility is ported (e.g. `IPv4Comparator`), it is treated like ShellyScanner code |
| MikroDash | `LICENSE` = MIT, "Copyright (c) 2026 MikroDash" | MIT is compatible with GPL-3.0; keep its notice with the reused CSS/TS |
| MikroDash fonts | each font under SIL OFL 1.1 (per its `THIRD_PARTY_NOTICES.md`) | If vendored, ship the OFL text |

The new project: **GPL-3.0-or-later** (decided, Q19). ShellyScanner's text is the plain GPL v3 without
an "or later" statement; ported code stays under its original terms, which GPL-3.0-or-later is
compatible with. Every dependency's licence goes into
`THIRD_PARTY_NOTICES.md` before it is added (MikroDash's rule), and CI checks the Go module list.

### 3.2 Classification per component

| Category | Meaning | Components (planned) |
|---|---|---|
| 1. Copied | ShellyScanner code copied verbatim | **None planned.** Different language; nothing is copied. Label strings for terminology are reused as short phrases/UI terms (e.g. "Devices conf.", "Restricted login") — treated as category 2 for attribution |
| 2. Translated / ported (derivative work) | Logic taken from the Java code and re-expressed in Go/TS | Model registry (IDs, type names, per-model parsing and quirks); FW version regex; restore order and per-model restore logic; backup ZIP layout; checklist semantics; deferred-task model; discovery follow-ups (range extender, BLU) |
| 3. Clean re-implementation from functional analysis | Behaviour reproduced from the Shelly API docs and observed behaviour, without following the Java code's structure | Protocol clients (Gen1 REST, Gen2+ RPC, digest auth per Shelly's published spec), poller, mDNS, WebSocket hub |
| 4. Entirely new | No counterpart in the original | Web UI, HTTP API, `/data` store and encryption, auth, Docker/CI, simulator, firmware index/cache/QR and server-side version comparison, chart ring buffer, i18n (EN/NL), future MCP/HA adapters |

Because ShellyLanMan is itself GPL-3.0, category 2 is fully permitted; the classification exists for
**honest attribution** and so the original author can see what came from where. Ported Go files get a
header like:

```go
// Portions derived from ShellyScanner (https://github.com/usnasoft/shellyscanner),
// Copyright (C) Antonio Flaccomio / usnasoft, licensed under GPL-3.0.
```

A `docs/PROVENANCE.md` table (package → category → Java source) is kept up to date from Phase 1.

### 3.3 Attribution and trademark

- README "Credits" section and About page: "Based on ShellyScanner by usnasoft (Antonio Flaccomio)",
  link to the repository and https://www.usna.it/shellyscanner/.
- README and About: "ShellyLanMan is an independent project. It is not ShellyScanner, and it is not
  affiliated with or endorsed by Shelly Group (formerly Allterco Robotics). Shelly® is a trademark of its
  owner." (exact owner name to be verified before release).
- MikroDash credited in `THIRD_PARTY_NOTICES.md` and the About page for design tokens/appearance code.
- Risk noted in §7: the project name itself contains the "Shelly" mark.

---

## 4. Firmware: how it works today, and the QR feature

### 4.1 How ShellyScanner does it (verified in code)

- **The device does all of it.** ShellyScanner only asks the device and tells it to update:
  - Gen1: `GET /ota/check` (device queries Shelly cloud), then `GET /ota` → `old_version`, `has_update`,
    `new_version`, `beta_version`, `status` (`updating`). Update: `/ota?update=true` or `/ota?beta=true`.
  - Gen2+: `Shelly.CheckForUpdate` → `{stable:{version,build_id}, beta:{…}}` plus `Shelly.GetDeviceInfo`
    for the current `ver`/`fw_id`. Update: `Shelly.Update {"stage":"stable"|"beta"}`. Progress over the
    device WebSocket (`ota_progress` %, `ota_success`, `scheduled_restart`).
  - BLU TRV: via the gateway, `BluTrv.CheckForUpdates` / `BluTrv.UpdateFirmware` (no stage, no URL).
  - Offline/battery devices: the update is queued as a deferred task; stored JSON supplies the
    current version.
- ShellyScanner **never downloads firmware** and never serves it.

### 4.2 Official sources (checked 2026-09-26)

| Source | Status | Content |
|---|---|---|
| Device-side APIs (`/ota`, `Shelly.CheckForUpdate`, `Shelly.Update`) | **Documented** (Gen2+ docs: `Shelly.Update` takes **either** `stage` **or** `url`) | The supported path |
| `https://api.shelly.cloud/files/firmware` | Public, **undocumented** | JSON map Gen1 `type` → `{url, version, beta_url?, beta_ver?}`; file URLs are **plain HTTP** (`http://firmware.shelly.cloud/gen1/<TYPE>.zip`) |
| `https://updates.shelly.cloud/update/<app>` | Public, **undocumented** | JSON `{stable:{version, build_id, url}, beta?, name, desc, alt:{<app variant>:{…}}}`; `url` on `https://fwcdn.shelly.cloud/gen2-ntest/<app>/<64-hex>` (the 64-hex path looks like a SHA-256 of the file — to verify). `alt` lists alternative firmware (e.g. Zigbee `S1PMG4ZB` next to Matter `S1PMG4`) |

Reliability: the two indexes are what Shelly's own tooling uses, but they are not a documented public
API and can change without notice. The feature must fail soft (clear message, no effect on the rest).

Mapping firmware ↔ device: Gen1 by `type` (`SHSW-1`), Gen2+ by **`app`** from `/shelly` — including for
Gen4, which ShellyScanner identifies by `model` but whose firmware is keyed by `app`. A device running an
alternative firmware (Zigbee vs Matter) reports its own `app`, and we use exactly that; no cross-variant
flashing.

### 4.3 Risks

- **Wrong image → bricked device.** Only ever map from the device's own reported `type`/`app`; no manual
  file picking in the first version.
- **Integrity.** Gen1 files come over HTTP: fetch the index over HTTPS and prefer an HTTPS file URL where
  the host serves one; verify size; for Gen2+ verify the SHA-256 against the URL hash if that holds.
  Whether devices verify signatures themselves is **unknown** (to research; do not rely on it).
- **Compatibility.** Some old Gen1 firmware may require intermediate versions; Gen2+ `Shelly.Update {url}`
  with a plain-HTTP LAN URL must be verified on hardware per generation.
- **Exposure.** The download endpoint must be reachable **without a UI session** (a phone or a device
  fetches it), so it uses short-lived signed URLs and serves only cached official files.
- **Licence of firmware files.** They are Shelly's. We download on the user's behalf for their own
  device and cache locally; nothing is bundled in the image or redistributed.

### 4.4 Design (decided scope: QR/download + server-side version comparison, stable only)

The use case (from Wim's existing setup, §4.5): devices whose internet OTA fails, typically because of
unstable Wi-Fi, are updated **locally through the device's own temporary access point**. The phone
first needs the right firmware file; the QR code gets it there in one scan.

1. The Firmware page stays exactly as the original (device-driven check, update to stable/beta,
   progress, deferral).
2. **Server-side version comparison (a).** The server reads the Shelly index for each device's
   `type`/`app` and shows "latest stable (Shelly index)" next to the device's own check. This fills the
   gap for devices that cannot run `Shelly.CheckForUpdate`/`/ota/check` themselves. Index results are
   cached (e.g. 6 h) and only fetched while the feature is used.
3. For a device where the index has a newer **stable** version, a **"Local download" (⚡)** action opens
   a modal with a **QR code**, the link as copyable text, the **device name, model and target version**
   (as a badge, so it can be compared with the device's own update page).
4. The link points **to ShellyLanMan**: `http://<server>/fw/<token>/<original-file-name>`. On first
   request the server downloads the file to `/data/firmware/`, verifies it and serves it; later requests
   come from the cache (bounded: latest stable per `type`/`app`, oldest evicted). The phone therefore
   needs the LAN, not the internet, at download time, and the file is guaranteed to be the one the server
   resolved.
5. Sources:
   - **Gen2+**: `https://updates.shelly.cloud/update/<app>` → use **`stable.url`** and
     **`stable.version` only** — never `alt.*`, never the JSON page itself. Request with the real host
     name (a proxy that forwarded a different `Host` header got a test channel with version 1.7.5 back).
   - **Gen1**: official index `https://api.shelly.cloud/files/firmware` → `data.<TYPE>.url/version`
     first; **fallback** to the community archive `http://archive.shelly-tools.de/archive.php?type=<TYPE>`
     (list of versions, sorted **alphabetically** → sort as versions, take the highest stable) and
     `http://archive.shelly-tools.de/version/<version>/<TYPE>.zip`. The UI shows which source was used.
   - **BLU TRV**: not supported (updated only through its gateway).
6. `<server>` comes from a "base URL for phones" setting, defaulting to the address the browser used;
   the modal warns if that is `localhost` or not reachable from the LAN.
7. **Not in scope (b)**: telling a device to update from our URL (`Shelly.Update {url}` / `/ota?url=`).
   Can be proposed later as a separate feature.

### 4.5 Reference: Wim's current `⚡FW` menu (from memory, 25 Sept 2026)

Wim already has a hand-made version of this on `shelly.wimmme.net/menu.html` (static site on
dockerhostvm:8998): per device a ⚡ link opening `qr.html?data=<firmware-url>&name=&model=&version=`,
rendered client-side with qrcodejs; for Gen2+ the page fetches the index live through a HAProxy proxy
(`fwproxy.wimmme.net`, needed for CORS and to fix the `Host` header); Gen1 links are static, built from
shelly-tools archive versions (v1.14.0 for all six Gen1 types in the house on 25 Sept). Lessons carried
over: point at `stable.url`, not the index page and not `alt`; version-sort Gen1 archive lists; show the
version badge; a server-side fetch avoids the CORS and `Host` issues entirely. Observation: app `Plus1`
is still offered `1.7.5` as stable by Shelly's index (possibly a renamed app ID in the 2.0 line — to
investigate in Phase 7). ShellyLanMan replaces this menu once the feature is done.

---

## 5. Test strategy

- **One command, locally and in CI**: `make verify` (or `tools/verify.sh`) runs everything **inside
  Docker** (`docker build --target test .`), so the host needs only Docker: `gofmt` check, `go vet`,
  `go test -race ./...`, `tsc --noEmit`, frontend unit tests, smoke tests. CI runs the same target and
  fails on any failure. New tests are discovered automatically (Go `_test.go`, `web/test/*.test.ts`).
- **Fixtures** in `testdata/`: `gen1/<TYPE>/`, `gen2/<app>/`, `gen3/<app>/`, `gen4/<model>/`,
  `blu/<model>/`, each holding the recorded responses for every call in `FEATURE_PARITY.md` §3 that the
  model supports (`shelly.json`, `settings.json`, `status.json`, `Shelly.GetConfig.json`, …), plus
  sample `.sbk` backups. A small `cmd/record` tool reads a real device and writes a
  fixture set with **MAC, IP, SSID, names, cloud IDs, keys and passwords scrubbed**; CI rejects fixtures
  that match credential/MAC patterns (the repo is public).
- **Simulator** `cmd/shellysim`: fake Gen1/Gen2+/BLU-gateway devices driven by fixtures, stateful for
  writes (relay/cover/light state, config, scripts, KVS, schedules, webhooks), with Basic and digest
  auth, `/debug/log` and RPC WebSocket (including `ota_progress` events), multiple devices on multiple
  ports (range extender), "sleeping" battery devices (going offline and back), slow and erroring
  devices, and **mDNS announcement** when run with host networking. It backs integration tests and
  doubles as a demo mode.
- **Unit tests**: parsers per model (fixture in → model out), version regex (with the examples from
  `FirmwareManager`), digest auth (vectors), restore ordering (recorded call sequence vs expected),
  backup ZIP layout, backup retention, settings
  encryption round-trip.
- **Integration tests**: HTTP API and WebSocket against the simulator (discovery → poll → control →
  config → backup/restore → firmware flow → deferred tasks).
- **Frontend smoke tests**: the page loads, the device table fills from the simulator, a toggle round-
  trips, a confirm dialog blocks a reboot. Playwright in CI (dev-only dependency).
- **Real hardware**: for every phase that touches devices, a short checklist for you (exact commands,
  what to look for), results recorded in `docs/hardware-tests.md`. CI never touches real devices.
- **Parity gate**: no row in `FEATURE_PARITY.md` gets ✅ without its tests.

---

## 6. Phased plan

| Phase | Deliverable | Exit criteria |
|---|---|---|
| 0 | This analysis + open questions | Your answers and approval |
| 1 Skeleton | Repo layout, Go server with `/healthz`, themed empty SPA (shell, sidebar, appearance), WebSocket hub, `/data` store with key generation and encrypted settings, first-run page, Dockerfile (multi-arch), compose file, test target, simulator stub, CI (test on PR; publish on tag), `CLAUDE.md`, README Quick Start, CHANGELOG, SECURITY, THIRD_PARTY_NOTICES, LICENSE | `make verify` green in CI; image runs with only a `/data` volume |
| 2 Discovery | mDNS spike + discovery, IP scan, offline mode, identification + registry, range extender, BLU via gateways, poller, statuses, archive, interface picker, per-device credentials + global default, login flow, polling modes (browser connected / presence) | Simulator e2e; hardware check with your devices |
| 3 Read-only info | Devices table with all columns, filters, sort, column chooser, views, device info panel, logs, meters, °C/°F, uptime formats | Fixture-based parsers for every owned model; hardware check |
| 4 Controls | Command column for every module kind, reboot | Simulator + hardware |
| 5 Configuration | Devices conf. (Wi-Fi, login, MQTT, others), checklist, deferred tasks | Simulator + careful hardware test on a spare device |
| 6 Backup/restore | `.sbk` backup/restore (single, multi), restore wizard, deferred backup/restore | Round-trip against simulator; retention setting; hardware (Grondwaterpomp) |
| 7 Firmware + QR | FW check/update/progress/deferred, then the QR/local download | Hardware (one device per generation) |
| 8 Advanced | Scripts + IDE, KVS, schedulers, charts (server ring buffer), export CSV/print, notes | Simulator + hardware |
| 9 Parity review | Walk `FEATURE_PARITY.md` against ShellyScanner side by side | Every row ✅ or an agreed ⚠️ |
| 10 Release | Production multi-arch image on GHCR on version tags, upgrade docs | Tagged release |

---

## 7. Risks and unknowns

| # | Risk / unknown | Impact | Mitigation |
|---|---|---|---|
| R1 | Go mDNS libraries are less proven than JmDNS / python-zeroconf | Discovery gaps | Phase 2 spike on real devices; fallback minimal implementation; IP scan always available |
| R2 | Bridge networking silently breaks mDNS | "No devices found" support issues | Host networking default; detection + banner |
| R3 | Polling every 2 s **24/7** (the original only polls while its window is open) loads devices, Wi-Fi and battery gateways far more than the desktop app ever did | Device instability, especially Gen1 | Decided (Q8): full rate only while a browser is connected, slow presence check otherwise |
| R4 | Generalising ~76 Gen2+ Java classes into a registry loses a per-model quirk | Wrong values/restores for some model | Map every Java class explicitly; fixtures per model; parity review |
| R5 | Restore is complex and model-specific (add-ons, profiles, dynamic components, BTHome) | Misconfigured devices | Replicate order exactly; call-sequence tests vs simulator; confirmation + spare-device hardware tests |
| R6 | Undocumented firmware indexes change | QR feature breaks | Fail soft; isolate in `internal/firmware` |
| R7 | Gen2+ acceptance of `Shelly.Update {url}` over plain HTTP, and device-side signature checks, are unverified | Local update path may not work | Hardware test before committing to (b) in §4.4 |
| R8 | Fixtures leak personal data into a public repo | Privacy | Scrubbing recorder + CI pattern check |
| R9 | I cannot test models you don't own | Untested parsers | Ask community for fixture contributions via `cmd/record`; mark "untested on hardware" |
| R10 | The project name contains the "Shelly" trademark | Name change later | Your call (Q20); keep name out of logos/branding lookalikes; clear disclaimer |
| R11 | Several people using the UI at once (the original is single-user) | Conflicting writes (notes, config) | Last-write-wins, documented; per-device operation lock while a write runs |
| R12 | Device passwords at rest with the key in the same volume | Limited protection | Documented honestly in `SECURITY.md`; optional UI password; never expose secrets over the API |
| R13 | TLS to Shelly's cloud from inside the image (CA roots) | Firmware index fetch fails | `ca-certificates` in the image; tested in CI with a network-free fallback |
| R14 | Original's behaviour I have not yet traced in full (per-model restores, scheduler hints, IDE autocomplete, EM data pages) | Parity gaps discovered late | Each phase starts with a focused read of the relevant Java classes, recorded in `FEATURE_PARITY.md` |

---

## 8. Decisions taken (answers of 2026-09-26)

Answered interactively by Wim. These override any proposal above that says otherwise.

| # | Question | Decision |
|---|---|---|
| Q1 | Backend technology | **Go + TypeScript** |
| Q2 | Frontend library | **Plain TypeScript**, no framework (as MikroDash) |
| Q3 | Host and architectures | **Linux + Docker Engine**; images for **amd64 and arm64** (no armv7). Target host: dockerhostvm (192.168.0.12, x86_64), next to the current `shellyscanner` container (bridge, port 8404) |
| Q4 | Networking default | **Host networking** in the compose file; bridge + IP scan documented as alternative |
| Q5 | Default port | **3082** (checked free on the target host) |
| Q6 | Repository / image / branch | `github.com/wimmme/shellylanman`, `ghcr.io/wimmme/shellylanman`, default branch **`main`** (rename the empty `master` before the first commit) |
| Q7 | Charts | **Server-side ring buffer** (in memory) so charts open with history — new behaviour, recorded as ⚠️ in `FEATURE_PARITY.md` G5. Retention/resolution to be proposed in Phase 8; note that sampling is slower while no browser is connected (Q8) |
| Q8 | Polling 24/7 | **Configured rate only while at least one browser is connected**; otherwise a slow presence check (proposal: 60 s) that keeps online status and deferred tasks working |
| Q9 | Device credentials | **Per device, with a global default**; stored encrypted in `/data` |
| Q10 | Deferred tasks | **Persisted** in `/data/deferred.json` (secrets encrypted) |
| Q11 | Migration from ShellyScanner | **No import** of `.arc` or `~/.shellyScanner`. `.sbk` byte-compatibility is **not required**; we keep the same ZIP layout anyway where it costs nothing, but may diverge |
| Q12 | Backup retention | **Configurable number of backups per device, default 10** (0 = unlimited); older ones deleted automatically |
| Q13 | Local mDNS scan | **Interface picker**: "full" (all interfaces) or one chosen interface |
| Q14 | Command line | **None.** The HTTP API covers backup/restore/list; no CLI subcommands. `-graphs` → API only |
| Q15 | Deviations D18, D3, T4, T17, L7, A5 | **All accepted** |
| Q16 | Details from "the other project" | Found in memory: Wim's `⚡FW` QR menu on `shelly.wimmme.net` (25 Sept 2026) — see §4.5 |
| Q17 | QR scope | QR/download **plus (a) server-side version comparison** with the Shelly index (for devices that cannot check themselves). **Not (b)** "update from ShellyLanMan". **Stable only** |
| Q18 | Own update check | **GitHub releases of ShellyLanMan, opt-in** (off by default) |
| Q19 | Licence | **GPL-3.0-or-later** |
| Q20 | Name / trademark | **Keep "ShellyLanMan"**, with a clear disclaimer; no Shelly logo or look-alike branding |
| Q21 | UI languages | **English + Dutch** from the start (string catalogue per language) |
| Q22 | Help | **Own short in-app help**; usna.it linked from the About page |
| Q23 | Contact original author | **Not needed.** Observations O1–O10 stay recorded as open points; handled per phase by analysing the code and testing on hardware |
| Q24 | Test device | **Grondwaterpomp** may be used for configuration/restore/firmware tests (from memory: not yet physically installed; model to be read during Phase 2). Other devices: read-only unless Wim says otherwise. Device inventory will come from Phase 2 discovery |
| Q25 | UI authentication | **One optional admin password**, off by default with a warning |

Future MCP note: the existing `shelly-mcp` (dockerhostvm:8933) currently gets its device list from
ShellyScanner's `menu.html`; ShellyLanMan's API is the natural future source for it (and for an
MCP server of our own, §2.8 of `ARCHITECTURE.md`).

## 9. Decisions taken during Phase 2 (2026-09-26)

Technical choices made while building discovery, within the scope agreed above.

| # | Decision | Why |
|---|---|---|
| P2-1 | **mDNS: own browse-only implementation** on `golang.org/x/net` (dnsmessage + ipv4), not a third-party mDNS library | Settles risk R1: it only has to browse `_http._tcp` and resolve SRV/A; ~300 lines, unit-tested with constructed packets, verified on the real network (43 instances, all 25 Shellies found and identified within 25 s). Shares UDP 5353 with the host's responder via SO_REUSEADDR/SO_REUSEPORT (`golang.org/x/sys`). |
| P2-2 | "Full mDNS scan" **skips container/VM bridge interfaces** (`docker*`, `br-*`, `veth*`, `virbr*`, `cni*`, `flannel*`, `podman*`, `vnet*`) | With host networking a Docker host has dozens of bridges (44 on dockerhostvm); no Shelly is behind them. They can still be picked in "Local mDNS scan". |
| P2-3 | **Scan-setting changes apply at once** (rescan) instead of "at next start" | ShellyScanner needs a restart because of JmDNS; restarting a server container is worse UX. Errors retry (30 s) and archive auto reload (45 s) are scheduled per (re)scan. |
| P2-4 | IDs: the MAC (upper case, no separators); unmanaged hosts without a MAC in their name get `addr:<ip:port>` | ShellyScanner uses an empty MAC there, which would merge unrelated devices. |
| P2-5 | Gen2+ reads use `GET /rpc/<Method>` with HTTP Digest (SHA-256) | Documented Shelly transport; one mechanism for all reads. Writes (Phase 4+) will use the same connection. |
| P2-6 | Devices are refreshed with `/status`+`/settings` (Gen1) or `Shelly.GetStatus`+`Shelly.GetConfig` (Gen2+) exactly like ShellyScanner, and the raw answers are kept for Phase 3 | The device table's remaining columns (Phase 3) parse the same payloads. |

## 10. Decisions taken during Phase 3 (2026-09-26)

| # | Decision | Why |
|---|---|---|
| P3-1 | **Per-model parsing as a table in `internal/parse`**, ported class by class: Gen2+ grouped in ~20 families (relay, dimmer, 2PM profiles, EM tri/mono, RGBW profiles, H&T, …), Gen1 one function per type, add-ons and BLU sensors separate | Faithful to each ShellyScanner class (meter types, order, labels, temperature source) without 100 files; every model owned by Wim is tested against its recorded fixture |
| P3-2 | Display preferences of ShellyScanner's General tab (uptime format, temperature unit, double-click action, default filter column) and the column layout are **per browser** | They are view preferences; several people may use one server |
| P3-3 | The **Command column is read-only** in Phase 3 (state text per module); controls come in Phase 4 | Read before write |
| P3-4 | Sensor Add-on peripherals are read with `SensorAddon.GetPeripherals` at every configuration refresh when an add-on is fitted (and always for the Plus UNI, whose add-on is integrated) | Same calls as ShellyScanner; one extra request per configuration refresh |
| P3-5 | Device info lists the generation's standard info requests; model-specific extras of a few classes (e.g. Matter, XMOD, EM data, KNX) are not listed yet | Those classes are not in this installation; added when their features are ported (FEATURE_PARITY I1 🔨) |
| P3-6 | Live logs are relayed by the server (`/ws/log/{id}`); "On/Off" connects/disconnects without writing to the device; enabling the websocket debug log itself is a configuration change (Phase 5, Checklist) | Read-only phase |

## 11. Decisions taken during Phase 4 (2026-09-26)

| # | Decision | Why |
|---|---|---|
| P4-1 | Each module carries a **`key`** (the component the command goes to: `switch:0`, `relay/0`, `cover:1`, `xt1:202:202:C`, …) and each device a **`layout`**: the array type the Java model returns from `getModules()` (`relay`, `roller`, `rgbcct`, `rgbw`, `rgb`, `thermostat`, `trvg1`, `cb`, `mixed`) | `DevicesCommandCellEditor` chooses its panels by that array type; carrying it keeps the web cell identical per model without re-deriving Java's class hierarchy in the browser |
| P4-2 | One endpoint `POST /api/v1/devices/{id}/command {key, action, value, rgb, white, event, confirm}`; the **service maps (kind, key, action) to the exact Shelly call** of the Java module: GET `/rpc/<Method>?…` where ShellyScanner uses `getJSON`, POST `/rpc` with the JSON-RPC `auth` object where it uses `postCommand`, Gen1 REST GETs | Same requests on the wire as the original; MCP/HA can use the same service later |
| P4-3 | After a successful command the device **status is read once at once** | The Java modules update their state from the command's answer (or assume it); one status read gives the same result for every module kind without per-kind answer parsing |
| P4-4 | **Reboot** (`POST /api/v1/devices/reboot {ids, confirm:true}`): per device refresh pauses, status "reading", reboot sent, 3 s pause, then an immediate refresh | `Devices.reboot`; the immediate refresh replaces "wait for the next tick" so the row does not stay on "reading" up to 60 s when no browser is connected |
| P4-5 | Input event buttons: the **server** GETs the URLs configured on the device (Gen1 action URLs; Gen2+ and BLU webhooks with `http://127.0.0.1`/`localhost` rewritten to the device or gateway), 10 s timeout, http/https only; the URLs never go to the browser | O5: the original's behaviour, kept; URLs come only from the device and run only on an explicit click |
| P4-6 | The circuit breaker toggle needs `confirm:true` in the API as well as the dialog in the UI; a locked breaker (`safety`) cannot be toggled | Same confirmation as ShellyScanner, enforced server-side too |
| P4-7 | The table does not redraw while a slider in a Command cell or the lights editor is being dragged; it catches up on release | Device updates arrive every 2 s; redrawing would drop the slider under the pointer |
| P4-8 | Where the original sends a request that looks wrong we send the corrected one and list it for Wim: Gen1 bulb on/off without index (O14), `Thermostat.SetConfig` with `=` instead of `:` in its JSON (O15) | The JSON one cannot be sent as-is by a JSON encoder; the bulb one is a one-character difference. Both flagged, not silently fixed |

## 12. Decisions taken during Phase 5 (2026-09-27)

| # | Decision | Why |
|---|---|---|
| P5-1 | **Settings dialog as a modal** opened from the device table ("Settings" with a selection), tabs Wi-Fi 1, Wi-Fi 2, Restricted login, MQTT, Others; the FW Update tab arrives with Phase 7 | Same structure as DialogDeviceSettings; each tab reads the devices when shown and reports one result line per device |
| P5-2 | API: `GET /api/v1/config/{section}?ids=` (common values, excluded devices, MQTT/login variant) and `POST /api/v1/config/{section}` (results). The service decides the variant (G1 / G2 / mixed MQTT panel), the prefix rule for several devices and which devices are queued, exactly as the Java panels | One place for the rules; the browser only draws the form |
| P5-3 | Wi-Fi changes need `confirm:true` in the API (the original's warning dialog) | Wrong values disconnect devices |
| P5-4 | After a login change the new credentials are stored encrypted for that device (disable: removed) and used at once | LoginManager*.set/disable update the app's authentication the same way |
| P5-5 | Deferred tasks: `/data/deferred.json`, parameters (passwords included) sealed with AES-256-GCM bound to the task id; keyed by device ID; not cancelled by a rescan (O22); at most 200 kept (oldest finished dropped); a task interrupted by a restart is marked failed; "Deferred" page with the waiting count on the sidebar | Q10 and a server that runs for months |
| P5-6 | Checklist page: from the device table for the selection (`#/checklist?ids=`) or from the sidebar for all devices; rows computed on the server with the same requests as CheckListView; right-click menu per column; Scripts edit waits for Phase 8 | The sidebar entry has no selection to work from |
| P5-7 | Gen1 "reboot required" after an eco-mode change is remembered by the server until the device is rebooted (Gen1 does not report it) | AbstractG1Device.setEcoMode sets rebootRequired |
| P5-8 | MQTT `-slow` becomes a setting (tenths of a second between devices) on the Network settings page | Q14: no command line |

## 13. Decisions taken during Phase 6 (2026-09-27)

| # | Decision | Why |
|---|---|---|
| P6-1 | Backups are kept on the server in `/data/backups/<device id>/<hostname>-<yyyymmdd-hhmmss>.sbk`, newest first, the last N per device (setting `backupKeep`, default 10, 0 = all); every backup can be downloaded | Q12; the browser has no folder to choose, the server does |
| P6-2 | The `.sbk` layout is the one of ShellyScanner (same entry names, scripts as `<name>.mjs`), so backups can be moved between both programs | Costs nothing (Q11) |
| P6-3 | JSON of backups is handled with an ordered model (`internal/ojson`) so Gen1 restores send parameters in the backup's order and numbers keep their literal text, like Jackson in the original | Some Gen1 firmware is order-sensitive |
| P6-4 | Restore source: a backup of the device itself, a backup of another device or an uploaded `.sbk` (base64 in the JSON request, 24 MB limit) | Replaces the file chooser |
| P6-5 | The restore wizard asks the questions of RestoreAction in the same order (other host → error → warnings → passwords login / Wi-Fi 1 / Wi-Fi 2 / AP / MQTT → script conflicts → enable scripts), then an explicit confirmation (brief: destructive actions); API needs `confirm:true`; after success the reboot offer when a setting needs it | Original flow + the brief's rule |
| P6-6 | Multi restore uses **the newest stored backup of each device** (the original: one file per host name in a chosen folder); the UI keeps the per-device "other host" question and warnings, passwords are not asked, scripts are overwritten and enabled; `POST /api/v1/restore/multi` is the non-interactive variant (nonInteractiveRestoreDevice: a question or error stops that device) | No folder on the server side; newest backup is what "restore all" means here — accepted by Wim 2026-09-27 |
| P6-7 | Backup of an off-line / not-logged-in / archived device and restore of an archived device are queued as deferred tasks (backup data and answers sealed); an archived device is checked from the file alone (GhostDevice.restoreCheck); the check of a reachable device that turns out to be off line is an error, as in the original | Same behaviour, persistent queue (P5-5) |
| P6-8 | Refresh of a device is paused while its backup or restore runs, and it is refreshed right after | model.pauseRefresh / activateRefresh |

## 14. Decisions taken during Phase 7 (2026-09-27)

| # | Decision | Why |
|---|---|---|
| P7-1 | **FW Update is the first tab of Devices settings** (as in ShellyScanner) and the same panel is the **Firmware page** (sidebar: all devices; `#/firmware?ids=`); only the page has the "Shelly index" column with the ⚡ local download | Original layout; the QR feature is new and stays out of the ported dialog |
| P7-2 | Firmware check and update run on the server, 35 devices in parallel (the original's thread pool); progress from the device's RPC WebSocket (`ota_progress`, `ota_success`, `scheduled_restart`) is pushed to the browsers as `firmware.row` events; an update is followed for at most 15 minutes | PanelFWUpdate / FMUpdateListener, but server-side so every open browser sees it |
| P7-3 | Archived and unmanaged devices get an "any" checkbox that queues a deferred update to stable; a device that is off line when the update is sent is queued as well (FW_UPDATE task) | createTableRow / updateDeviceFW |
| P7-4 | **Plus1 and `1.7.5`:** not a renamed app — Shelly's index offers 1.7.5 as stable for `Plus1` and the device's own `Shelly.CheckForUpdate` offers nothing newer; Plus (ESP32) devices stay on 1.7.x while Gen3/Gen4 are on 2.0.1 | Answer to the question in §4.5, checked on the device 2026-09-27 |
| P7-5 | **Shelly's Gen2+ firmware hosts (`updates.shelly.cloud`, `fwcdn.shelly.cloud`) use certificates of a private CA ("Allterco") without a subject alternative name**; no standard client (Go included) accepts them. We **pin the hosts' public keys** (SPKI SHA-256; certificates valid until 2031 / 2033) instead of trusting any certificate; a changed key makes the feature fail soft with a clear message | Keeps a real check on who answers; the CA itself is only inside device firmware — accepted by Wim 2026-09-27 |
| P7-6 | **File verification before serving:** Gen2+: SHA-256 of the file equals the 64-hex name in `stable.url` (verified on real files) and `manifest.json` name = device app, version = index version; Gen1 (plain HTTP only — `firmware.shelly.cloud` has no HTTPS): manifest `build_id` = index build and every part's `cs_sha256` matches | Wrong image → bricked device (§4.3) |
| P7-7 | Links: `http://<phone base>/fw/<token>/<file>`, the token sealed with the server key (AES-GCM) holding type/app, version and expiry (24 h); a link whose version is no longer the latest answers 410; served without UI session; file names `<TYPE>.zip` (Gen1) and `<app>-<version>.zip` (Gen2+, the CDN name is only a hash) | §4.4 item 4 and 6 |
| P7-8 | The ⚡ button appears only when the index has a **newer** stable version than the device reports (stable only, `alt.*` and beta never used); index answers cached 6 h, files: the 20 most recently used kept in `/data/firmware` | §4.4 scope |
| P7-9 | Dependency `github.com/skip2/go-qrcode` (MIT, planned in §1.5); QR rendered server-side as PNG (data URI, allowed by the CSP) | API/MCP can use it too |

## 15. Decisions taken during Phase 8 (2026-09-27)

| # | Decision | Why |
|---|---|---|
| P8-1 | Notes and keyword: only with the archive in use (as the original's Notes action); saved with the archive (every minute and at shutdown, not only when the program exits) | NotesEditor, P2 archive rules |
| P8-2 | CSV exports (table and charts) quote a value that contains the separator, a quote or a line break (O29); separator per browser | Device names can contain commas |
| P8-3 | Scripts dialog: Scripts and KVS tabs as in the original; the script editor is CodeMirror 6 (planned in §1.4), in its own bundle loaded only when the editor opens; editor settings (tab size, font size, indent, auto-close, dark) per browser; "Open"/"Save" use the browser's file picker and download | The original's own Swing editor cannot be ported; the browser has no file system |
| P8-4 | The scheduler runs the device calls from the browser through one server endpoint `POST /api/v1/devices/{id}/rpc` (Gen2+; for a BLU TRV through its gateway as `BluTrv.Call`) — it is also the original's "test method" button; the apply logic of G2SchedulerPanel / WDThermSchedulerPanel / TRVSchedulerDialog is ported in the browser | Same structure as the original (the dialogs call the schedule managers directly) |
| P8-5 | Charts: Chart.js with the zoom plugin (MIT) in their own bundle; readings kept on the server per device for 24 h (at most 20 000 per device) in memory, not on disk; the EM chart reads EMData / EM1Data once a minute like the original | Q7, Q8 |
| P8-6 | Keyboard shortcuts kept where the browser allows them: filter (Ctrl+F/E/S), editor, notes (Ctrl+S, Ctrl+K), charts (Ctrl+R/P/C); menu mnemonics and window focus shortcuts are not ported | A11 |
| P8-7 | Help: a short help per function on the About page, plus the "?" help in the scheduler, the script editor and the charts; the online manuals of usna.it are linked from About | Q22 |
| P8-8 | The `-graphs` stream (L6) is covered by `GET /api/v1/samples` and the `device.upsert` WebSocket events | Q14 |

## 16. Decisions taken during Phases 9 and 10 (2026-09-28)

| # | Decision | Why |
|---|---|---|
| P9-1 | A device that could not be read when discovered is retried every 2 minutes (after the first retry at 30 s) | O33, extension agreed by Wim |
| P9-2 | `GET /api/v1/devices?gen=` replaces `-gen` | L5, Q14 |
| P10-1 | Release check: GitHub releases of ShellyLanMan, setting never (default) / stable / all, once a day and after a settings change, banner with release notes link and "skip this version"; a `dev` build is never told to update | A8, Q18 |
| P10-2 | Images for `linux/amd64` and `linux/arm64` on GHCR from `vX.Y.Z` tags (workflow of Phase 1): tags `X.Y.Z`, `X.Y`, `latest`; the arm64 binary is cross-compiled (checked), the final Alpine stage runs under QEMU on GitHub | Brief phase 10 |
| P10-3 | Upgrade instructions in the README: `docker compose pull && up -d`, pinning a release line, saving the data volume before a major version | Brief phase 10 |

## 17. Decisions after v0.1.0 (2026-09-28, Wim's feedback)

| # | Decision | Why |
|---|---|---|
| F1 | The devices table is updated in place (`dom.patch`): unchanged rows and cells stay; row mouse handling is delegated and reads the rows as last drawn | Hover flicker and lost checkbox clicks during refreshes |
| F2 | Web server port is a setting (Settings → General, `PUT /api/v1/server` with `confirm`); the server opens the new port, saves it, and closes the old one after 3 s. `SHELLYLANMAN_LISTEN` wins and locks it; the health check reads the saved port | Wim asked; not in ShellyScanner (a desktop program has no port) |
| F3 | UI languages: en, nl, de, fr, es, it, bg, zh. ShellyScanner has only English labels (its Italian bundle holds URLs only), so the translations are our own | Wim asked; extends Q21 |
| F4 | About page: description, version and release-check state, uptime, system information, support links (Buy me a coffee, PayPal — plain links, no external scripts or images, CSP), release notes from the embedded CHANGELOG, dependencies (Go build info + `deps.json` from package-lock), licence and notices from the binary. ShellyScanner and MikroDash under Based on and Credits, both "deserve a coffee", no donation link for them | Wim asked, MikroDash as example without copying it |
| F5 | The no-auth banner is dismissed per browser (localStorage) | Wim asked |
| F6 | New logo (Wim's): transparent icon for sidebar/favicon/About, the full logo in the README | Wim asked |

## 18. MCP server (2026-09-28, Wim's go)

| # | Decision | Why |
|---|---|---|
| M1 | MCP server in the same binary (`internal/mcp`) at `/mcp`, Streamable HTTP, stateless, JSON responses only (no SSE, no sessions); protocol versions 2025-06-18, 2025-03-26, 2024-11-05. Written on the standard library, no SDK dependency | ARCHITECTURE §2.8; dependencies: stdlib first |
| M2 | Local only: tools call the service layer, which reaches devices on the LAN like the UI does; no Shelly cloud | Wim: "local, not with the Shelly cloud account" |
| M3 | Off by default; a bearer token is always required (32 random bytes, stored encrypted as a secret, shown once, replaceable); requests with a foreign browser `Origin` are refused (DNS rebinding) | The UI has no login; an MCP client is a program that can hold a token |
| M4 | Access "read" (default) or "control"; control tools are not even listed in read mode. Reboot and firmware update also need `confirm: true`; every control call is logged at Info | Safety pattern of Buggy1111/shelly-mcp (reference, MIT; no code taken) |
| M5 | Tool set: list/get device, readings (samples, ≤120 points), firmware check, checklist, backups, `shelly_rpc_read` limited to `Get*`/`List*`/`CheckForUpdate`; switch, light, cover, thermostat, backup; reboot, firmware update. No restore, settings, scripts or generic RPC writes for now | Start with what is safe and useful; more on request |
| M6 | Devices are named by id/MAC, IP, host name, exact name or a unique part of the name; ambiguous names are refused with the candidates | An assistant says "the kitchen", not a MAC |

## 19. After v0.3.0 (2026-10-01)

| # | Decision | Why |
|---|---|---|
| R1 | The publish workflow creates the GitHub release of a `vX.Y.Z` tag after the image is pushed, with that version's section of `CHANGELOG.md` as notes (`tools/release-notes.sh`); verify and CI fail when a released version has no notes. v0.1.0–v0.3.0 got their releases afterwards | The in-app release check (P10-1) reads GitHub releases; only tags and images existed, so it never reported an update |

## 20. Phase 11 — Home Assistant and MCP parity (2026-10-01, Wim's answers to `docs/phase-11-ha-mcp.md` §4)

| # | Decision | Why |
|---|---|---|
| P11-1 | Order: 11a MCP parity with the Shelly-MCP → 11b ShellyLanMan as an HA app → 11c HA integration (entities on the Shelly devices, Assist); discovery hand-off (B) later | Q1; parity pays off in Claude Code right away, 11c builds on 11a |
| P11-2 | MCP access levels read / control / **configure**; configure covers scripts, KVS, schedules, webhooks, virtual components and RPC writes | Q2 |
| P11-3 | Scenes stored in ShellyLanMan (`/data`), as in the Shelly-MCP | Q3, parity |
| P11-4 | The HA app and the HA integration live in a separate repository (`shellylanman-ha`); both repositories document how they work together, the API contract and version compatibility | Q4; Python, own releases, HACS layout |
| P11-5 | MCP tools beyond ShellyScanner's feature set are approved up to the Shelly-MCP's set (webhooks, virtual components, scenes, script eval, …), LAN only | Wim: "at least the functions of the Shelly-MCP" |
| P11-6 | 11a tool design: lists and reads of KVS, schedules, scripts, webhooks and components stay with `shelly_rpc_read` (fewer tools for assistants such as HA Assist); new read tools `shelly_get_status` / `shelly_get_config` (raw, Gen1 too, passwords masked), `shelly_list_components`, `shelly_script_code`, `shelly_energy_history` (EMData/EM1Data), `shelly_scenes` | Parity with fewer tools |
| P11-7 | `confirm: true` for every delete, script code upload and eval, RPC write and device login; `shelly_rpc_write` refuses read methods (use `shelly_rpc_read`) and `Shelly.SetAuth` (use `shelly_device_login`, which stores the new password like Settings → Login), and factory reset / Wi-Fi reset / delete-all also need `allow_data_loss: true` | Same gates as the Shelly-MCP; ShellyLanMan must keep reaching protected devices |
| P11-8 | Scenes live in the service layer (`scenes.json`): actions are a command on a device module (planned from the same arguments as `shelly_switch` / `shelly_light` / `shelly_cover` / `shelly_thermostat`, so Gen1 works too) or a Gen2+ RPC method that is a plain write (no restart, update, delete, code, credentials or system config); a run tries every action and reports ok / partial / failed; toggles give a warning | Parity; safe to run twice |
| P11-9 | Relay flip-back timer (`timer_s`: Gen1 `timer`, Gen2+ `toggle_after`) and light fade (`transition_s`: Gen1 `transition` in ms, Gen2+ `transition_duration`) added to the service's Command, so the UI could use them later | Parity (`switch_set` / `light_set` of the Shelly-MCP) |
| P11-10 | Not taken over from the Shelly-MCP: the Shelly cloud account (no cloud, idea parked in `docs/phase-11-ha-mcp.md`), a reboot delay, a separate `list_methods` (`shelly_rpc_read` Shelly.ListMethods) and `version` (in the MCP handshake) | Covered otherwise or out of scope |
| P11-11 | Fix: a protected Gen2+ device with firmware 2.0+ answers POST /rpc with an empty 401 and the challenge only in `WWW-Authenticate`. `Conn.Call` now answers that with an HTTP Digest header (POST, `/rpc`, nonce reused with a growing `nc`, as for GET) and keeps the JSON-body challenge for older firmware. ShellyScanner gets the header case from Jetty's `DigestAuthentication` (`AbstractG2Device`, authentication store); our port had only the body fallback, so every POST RPC to a protected 2.x device failed and the repeated failures set off the device's brute-force lock (429) | Found in the 11a hardware test (ShellyTestPlug, 2.0.1) |
| P11-12 | 11b: ShellyLanMan as a Home Assistant app (HA OS) from `shellylanman-ha`: thin image on the main image (entry script fixes `/data` ownership, then runs ShellyLanMan as uid 10001 with `su-exec`), `host_network` (mDNS), ingress on `172.30.32.1:8099` accepting only the Supervisor `172.30.32.2` (`SHELLYLANMAN_INGRESS`, `SHELLYLANMAN_INGRESS_FROM`), the LAN port as before; the frontend uses relative URLs; the no-auth banner is hidden under ingress; the MCP settings show the LAN address under ingress | `docs/phase-11-ha-mcp.md` §5 |
