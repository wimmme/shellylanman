# Phase 11 — Home Assistant and MCP parity (analysis, 2026-10-01)

> Status: **Q1–Q4 answered (`DECISIONS.md` §20). 11a (MCP parity) built (P11-6 … P11-10).
> Q5: both ways, the token-less path only in app mode on 127.0.0.1, to be proven on a
> test HA OS first. Q6: test VM `ha-test` on Hyper-V. 11b done: app v0.4.0 from `wimmme/shellylanman-ha` (P11-12, P11-13). 11c not built.**

Parked idea (2026-10-01): an optional, off-by-default Shelly cloud link for other users
(devices at another site, last state of sleeping devices). Not in Phase 11; decide when
users ask for it.

Wim's direction (2026-10-01):

- Home Assistant runs on **HA OS**; HA Container is not a target (it has no apps).
- **A** (ShellyLanMan as an HA app) and **C** (ShellyLanMan entities next to the
  official Shelly integration) matter most; **B** (discovery hand-off) is nice to have.
- MCP is used from Claude Code today. **HA Assist** is where most of the gain is.
- ShellyLanMan's MCP must offer **at least the functions of the Shelly-MCP** he uses
  now (container `shelly-mcp` on dockerhostvm). This approves MCP tools beyond
  ShellyScanner's feature set.

---

## 1. MCP: Shelly-MCP compared with ShellyLanMan

The Shelly-MCP has 44 tools; ShellyLanMan has 14. Comparison per group:

| Group | Shelly-MCP | ShellyLanMan now | Gap | Service layer has it? |
|---|---|---|---|---|
| Identity / list | `list_devices`, `get_info`, `discover` (mDNS + **cloud**), `version` | `list_devices` (filters), `get_device` | rescan tool; version | yes (scan) |
| Status / config | `get_status` (normalized + raw, per component), `get_config` (credentials masked), `list_components`, `list_methods` | `get_device`, `rpc_read` (Gen2+ only) | raw status/config **for Gen1 too**, credential masking, components | yes (reads); masking is new |
| Energy | `energy_live`, `energy_history` (totals + by-minute) | `get_readings` (24 h in memory) | live per channel with PF/freq/totals; EMData history | partly (charts read EMData/EM1Data) |
| Relay | `switch_set` (with `toggle_after_s`), `switch_toggle` | `shelly_switch` on/off/toggle | auto-revert timer | timer: new parameter |
| Light | `light_set` incl. `transition_s` | `shelly_light` (gain, rgb, white, temp) | transition | small |
| Cover | `cover_move` | `shelly_cover` | — | — |
| Thermostat | — | `shelly_thermostat` | (ours is extra) | — |
| KVS | list / get / set / delete | read via `rpc_read` | write tools | **yes** (S11) |
| Schedules | list / create / update / delete | read via `rpc_read` | write tools | **yes** (S7, `POST …/rpc`) |
| Scripts | list / get_code / create / put_code / delete / start / stop / **eval** | read via `rpc_read` | write tools, eval | **yes** (S10); eval new |
| Webhooks | list / create / update / delete | read via `rpc_read` | all | read for backup only; writes new |
| Virtual components | list / add / delete | — | all | **new** (not in ShellyScanner) |
| Scenes | list / get / create / delete / run (stored in the server) | — | all | **new** (stored in `/data`) |
| Generic RPC write | `rpc_write` (`confirm`; factory reset etc. need a second flag) | — | write | yes (`POST …/rpc`) |
| Device login | `system_set_auth` | — | set password | **yes** (S3 restricted login) |
| Reboot / firmware | `system_reboot` (delay), `system_update` | `shelly_reboot`, `shelly_firmware_update` | reboot delay | — |
| Backup | — | `shelly_backup`, `shelly_list_backups`, `shelly_checklist`, `shelly_firmware_check` | (ours are extra) | — |

Notes:

- **Cloud:** the Shelly-MCP merges the Shelly cloud account list. ShellyLanMan does not
  do that (standing rule: no cloud dependency). LAN devices are covered by our own
  discovery, which already finds more (BLU via gateways, range extender).
- **Safety:** our pattern stays: access *read* / *control*, `confirm: true` for
  destructive actions. Proposal for the new tools (in line with the Shelly-MCP):
  `confirm` for deletes, `script_put_code`, `script_eval`, `rpc_write`, `set_auth`;
  `rpc_write` refuses factory reset / wipe / Wi-Fi reset unless a second flag is set.
  Credentials are masked in every config the MCP returns (Gen1 `/settings` holds the
  Wi-Fi and MQTT passwords in clear text).
- **Possible third access level** "configure" (scripts, KVS, schedules, webhooks,
  virtual components, RPC writes) next to "control" (switching): an assistant that
  switches lights does not need to rewrite scripts. Question Q2.

## 2. Home Assistant: what was checked

Sources: HA developer docs and the current `home-assistant/core` and
`home-assistant/supervisor` sources (2026-10-01).

| Fact | Consequence |
|---|---|
| HA's **MCP client** integration (`mcp`) connects with **Streamable HTTP** first and falls back to SSE (`coordinator.py`). The docs page still says SSE only — the code is newer | Our `/mcp` transport fits; no SSE needed |
| The MCP client sends a bearer token **only from OAuth**; on a 401 it starts OAuth discovery (`.well-known/oauth-authorization-server`). There is no field for a fixed token | Our fixed token cannot be entered in HA. Either an OAuth server in ShellyLanMan (large), an unauthenticated path for HA only, or our own integration (see §3.3) |
| The MCP client has `async_step_hassio`: an **app can announce its MCP server** through Supervisor discovery (`service: mcp`, `config: {url}`) and HA offers it with one click | App + discovery gives Assist the tools without typing a URL — but without a token (row above) |
| Supervisor discovery accepts **any service name** (`supervisor/discovery/validate.py`: `service: str`) | Our app can also announce itself to our own integration (`service: shellylanman`) — to be verified with a custom integration on HA OS |
| The official **Shelly integration** registers its devices with `connections={(CONNECTION_NETWORK_MAC, mac)}` (`shelly/coordinator.py`) | Entities of our integration with the same MAC connection land **on the same device card** in HA |
| **Ingress**: app on its ingress port, requests only from `172.30.32.2`, user already authenticated by HA, `X-Ingress-Path` header gives the base path; WebSockets supported | Login problem solved inside HA; the SPA must work under a path prefix |
| An integration can register an **LLM API** (`helpers/llm.py`: `async_register_api`, `llm.API`, `llm.Tool`); HA's own MCP integration does exactly that to hand MCP tools to conversation agents (`mcp/__init__.py`: `ModelContextProtocolAPI`) | Our integration can give Assist the ShellyLanMan tools itself, with our token |
| HA 2026.2 renamed add-ons to **apps** | Naming only |

## 3. Proposal

### 3.1 Phase 11a — MCP parity (first; no HA needed)

Add the missing tool groups of §1 on top of the service layer, with simulator tests and
a hardware test on ShellyTestPlug / Grondwaterpomp. Order: reads (status/config raw +
masking, components, energy), KVS, schedules, scripts, webhooks, virtual components,
RPC write, set_auth, scenes. Goal: Wim can switch Claude Code from the Shelly-MCP to
ShellyLanMan and stop the `shelly-mcp` container.

### 3.2 Phase 11b — ShellyLanMan as an HA app (A)

- App repository with `repository.yaml` and the app's `config.yaml`: the existing
  multi-arch image from GHCR, `host_network: true` (mDNS, like the ESPHome app),
  `ingress: true` with `panel_icon`, `/data` from the app's data folder (included in HA
  backups), options for log level.
- Server: an **ingress mode** — trust requests from `172.30.32.2` as authenticated,
  honour `X-Ingress-Path`; the SPA uses relative URLs (`web/src/api.ts`, the `/ws`
  URL and the absolute links in `index.html`). The normal port stays available for
  the MCP and the integration.
- Supervisor discovery: announce `shellylanman` (for our integration) and possibly
  `mcp` (see §3.3).

### 3.3 Phase 11c — HA integration (C, Assist, optionally B)

A HACS custom integration `shellylanman` (Python), set up by app discovery or by URL +
token:

- **Entities on the existing Shelly devices** (MAC connection), only what HA's Shelly
  integration does not have: checklist items as diagnostic sensors/switches (eco mode,
  LED, roaming, BLE, AP, range extender, Wi-Fi static, logs, auto firmware update),
  last backup (sensor) and a backup button, ShellyLanMan status (online / ghost /
  error / login). Live updates from our `/ws`. No relays, lights or meters — the
  official integration keeps those, so ShellyLanMan stays out of the data path for
  automations.
- **Assist:** the integration registers an **LLM API** in HA with the ShellyLanMan MCP
  tools (it holds the token, so no OAuth is needed). Conversation agents then offer
  "ShellyLanMan" next to "Assist". Recommended over the alternative — opening `/mcp`
  without a token for HA Core in app mode — because that relies on the source address
  on a host-network container.
- **B (later):** a repair/notification listing Shelly devices that HA does not know
  yet, with a button that starts the official Shelly integration's config flow.

Testing: `pytest-homeassistant-custom-component` in CI (Docker, like the rest), plus
a test on Wim's HA OS.

## 4. Questions for Wim

| # | Question | Proposal |
|---|---|---|
| Q1 | Order: 11a (MCP parity) → 11b (app) → 11c (integration)? | Yes: 11a pays off right away in Claude Code, and 11c builds on the tools of 11a |
| Q2 | A third access level "configure" for scripts, KVS, schedules, webhooks, virtual components and RPC writes? | Yes |
| Q3 | Scenes: stored in ShellyLanMan (`/data`) like the Shelly-MCP, or leave them out (HA has scenes)? | Include them for parity, small |
| Q4 | Where do the app and the integration live: in this repository (`ha/app`, `custom_components/shellylanman`) or a separate `shellylanman-ha` repository? HACS expects `custom_components/` at the root | A separate repository: other language (Python), own releases, HACS layout |
| Q5 | Assist through our own LLM API (recommended) or through HA's MCP integration with an unauthenticated path for HA Core? | Our own LLM API |
| Q6 | May the app/integration test write to your HA OS (install app, add integration)? Which HA instance and version? | — |

---

## 5. Phase 11b — design (2026-10-01)

Checked against `home-assistant/supervisor` (`docker/app.py`, `apps/validate.py`,
`api/ingress.py`, `api/discovery.py`) and the test system `ha-test` (HA OS 18.3,
HA 2026.9.4).

| Fact | Consequence |
|---|---|
| A `host_network` app is reached by ingress at the Docker gateway (`172.30.32.1`) on `ingress_port`; requests come from the Supervisor (`172.30.32.2`) | ShellyLanMan opens a second listener on `172.30.32.1:8099` in app mode, accepting only `172.30.32.2` — not reachable from the LAN |
| Ingress forwards the browser's headers (`Host`, `Origin`, cookies) plus `X-Ingress-Path` and `X-Remote-User-*`, and strips the `/api/hassio_ingress/<token>` prefix | The same-origin check keeps working unchanged; the SPA must use relative URLs (it already routes with `#/…`); esbuild chunks are already relative |
| `/data` is a bind mount owned by root; the Supervisor does not override the image's user (our image runs as uid 10001) | The app image is a thin layer on the main image: an entry script fixes `/data` ownership, then drops to uid 10001 (`su-exec`) — ShellyLanMan itself never runs as root |
| `image:` + `version:` in `config.yaml` → the Supervisor pulls `image:version` (multi-arch manifest) | App image `ghcr.io/wimmme/shellylanman-ha:<version>` (amd64, aarch64), built from `ghcr.io/wimmme/shellylanman:<same version>` in the `shellylanman-ha` repository |
| Options arrive in `/data/options.json`; the app's own settings stay in ShellyLanMan's `settings.json` in the same `/data` | Only a log level as app option; everything else in ShellyLanMan's Settings page |
| `/data` is included in HA backups | Settings, archive, scenes and backups go along with HA backups |
| In app mode the UI is behind HA's login | The "UI authentication is off" banner is not shown through ingress |
| The LAN port (3082 by default) stays open | For the MCP from Claude Code, the integration (11c) and `*.wimmme.net`; the port is a ShellyLanMan setting as before |

Not in 11b (they belong with the integration, 11c): Supervisor discovery
(`shellylanman`, `mcp`) and the token-less MCP listener on `127.0.0.1` (Q5).

Testing before a release: the app is installed as a **local app** on `ha-test`
(`/addons/shellylanman`, built on the device from a development image), then from the
`shellylanman-ha` repository once the images are on GHCR.

---

## 6. Phase 11c — design (2026-10-01, for review)

A HACS custom integration `shellylanman` in `shellylanman-ha`
(`custom_components/shellylanman`, `hacs.json` at the root next to the app). Python,
only Home Assistant's own libraries (aiohttp); tests with
`pytest-homeassistant-custom-component` in Docker, CI in that repository.

### 6.1 Facts checked

| Fact | Source | Consequence |
|---|---|---|
| An app lists its discovery services in `config.yaml` (`discovery:`) and posts `{service, config}` to the Supervisor with its token; Core then starts the config flow of the integration **with that domain**, custom integrations included | `supervisor/api/discovery.py`, `core/components/hassio/discovery.py` | The app announces `shellylanman` (URL of ShellyLanMan) and `mcp` (Q5); both appear in HA as "discovered" |
| HACS: one integration per repository under `custom_components/<domain>/`, `manifest.json` with domain, documentation, issue_tracker, codeowners, name, version; brand images in `custom_components/<domain>/brand/` (Core: `Integration.has_branding`) | hacs.xyz, `core/loader.py` | The app folder can stay next to it; icon from the app |
| The official Shelly integration registers devices with `connections={(mac, …)}` | `core/components/shelly/coordinator.py` | Our entities land on the same device card |
| Integrations can register an LLM API (`llm.async_register_api`, `llm.API`, `llm.Tool`) — HA's MCP client does exactly that | `core/helpers/llm.py`, `core/components/mcp` | "ShellyLanMan" selectable as an LLM API for conversation agents (Assist) |
| ShellyLanMan's REST API has no login on the LAN; the MCP server needs its token | ShellyLanMan | Set-up asks for the URL; the MCP token only for Assist |

### 6.2 What the integration does

1. **Set-up**: discovered from the app (one click), or by hand with the URL
   (`http://host:3082`) and, optionally, the MCP token for Assist.
2. **Live state** from ShellyLanMan's WebSocket (`device.upsert`), the checklist read
   every 30 minutes and on request (it costs one RPC per device).
3. **Entities per Shelly device** (diagnostic category, attached by MAC to the device
   of the official Shelly integration, or a device of their own when that one is not
   set up) — proposal for Q7/Q8:
   - `sensor` ShellyLanMan status (online / offline / login / error / ghost) — enabled
   - `sensor` last configuration backup (timestamp) and `button` "Back up configuration" — enabled
   - `binary_sensor` per checklist item: eco mode, LED off (Gen1), logs on, Bluetooth,
     access point, roaming, Wi-Fi static IP, range extender, automatic firmware update — **disabled by default**, the user switches on what he wants to watch
   - not duplicated: relays, lights, meters, firmware update (the Shelly integration has them)
4. **A device "ShellyLanMan"** for the service itself: number of devices on line /
   off line / needing attention, version, a "Rescan" button (Q9).
5. **Assist**: an LLM API "ShellyLanMan" whose tools are the MCP server's tools
   (listed live, so the access level set in ShellyLanMan applies) — with the token,
   or token-less in app mode (below).

### 6.3 ShellyLanMan side (main repository)

- In app mode (`SUPERVISOR_TOKEN` present): announce discovery `shellylanman`
  (`{"url": "http://127.0.0.1:<port>"}` — Home Assistant Core runs on the host network)
  and `mcp` (`{"url": "http://127.0.0.1:<mcp-port>/mcp"}`).
- Q5 implemented: in app mode an extra **MCP listener on `127.0.0.1` only, without
  token**, at the access level set in ShellyLanMan; off unless MCP is enabled; an app
  option can switch it off. Residual risk documented: other host-network apps on the
  same machine can reach it.
- App `config.yaml`: `discovery: [shellylanman, mcp]`.

### 6.4 Questions

| # | Question | Proposal |
|---|---|---|
| Q7 | Checklist entities read-only, or also switches (eco mode, LED, roaming, …) that change the device from HA? | Read-only first; switches later if wanted |
| Q8 | The entity list of §6.2 point 3, and which are enabled by default | As proposed |
| Q9 | A "ShellyLanMan" device with counters and a Rescan button? | Yes |
| Q10 | Discovery handoff to the official Shelly integration (B) now or later? | Later (separate phase) |
