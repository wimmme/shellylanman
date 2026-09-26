# ShellyLanMan — Decisions (Phase 0 proposals)

> Status: **questions answered 2026-09-26 (§8); awaiting final go for Phase 1.** Sections 1–7 hold the
> analysis and proposals; where §8 differs, §8 wins.
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
