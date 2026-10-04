# Phase 13 — Option B: from ShellyLanMan to the Shelly integration (analysis, 2026-10-04)

Option B (`docs/phase-11-ha-mcp.md` §3.3, DECISIONS P11-15): ShellyLanMan knows every
Shelly on the LAN; Home Assistant's own **Shelly integration** should get them without
the user adding them one by one. P11-15 asks for a thorough, documented analysis of
what the Shelly config flow allows **before anything is built**. This is that
analysis. Nothing is built yet; §6 holds the questions.

Sources: Home Assistant core `homeassistant/components/shelly` at tag `2026.9.4`
(`config_flow.py`, `manifest.json`; line numbers below refer to that tag), and
read-only checks on `ha-test` (HA 2026.9.4) on 2026-10-04.

## 1. How the Shelly integration adds devices

- **One config entry per device** (`integration_type: device`); the entry's unique id
  is the device's MAC (`config_flow.py:440`, `:722`).
- **Discovery is built in.** `manifest.json` subscribes to zeroconf `_http._tcp`
  with name `shelly*` and to `_shelly._tcp`, and to Bluetooth (`Shelly*`, manufacturer
  2985). Home Assistant starts a flow per device it hears.
- **Zeroconf flow** (`async_step_zeroconf`, `:1154`): takes host and port from the
  mDNS record, reads the device (`_async_get_info`), skips devices already configured
  (by MAC, updating the host if it moved), then
  - device with a password → **credentials** step (`:600`): password (Gen2+, user is
    always `admin`) or user + password (Gen1), typed by the user;
  - otherwise → **confirm_discovery** (`:1212`): one click "Submit"; the entry is
    created.
  A device that does not answer at that moment is aborted (`cannot_connect`) without a
  log line; Home Assistant tries again only when the device announces itself again.
- **User flow** (`async_step_user`, `:469`): lists the devices zeroconf/Bluetooth see
  now and not yet configured or offered, plus "manual"; **user_manual** (`:565`) takes
  host, port and *verify SSL*, then the same credentials step. Gen1 on a custom port
  is refused (`custom_port_not_supported`); devices behind a range extender are
  reached on `ip:port` (Gen2+).
- Sleeping (battery) devices must be awake to be added; BLU devices belong to the
  BTHome integration, not to Shelly.

## 2. What `ha-test` shows (2026-10-04, read only)

| | Count |
|---|---|
| Shellys known to ShellyLanMan | 26 |
| Configured in the Shelly integration | 1 (BrandstofcelSwitch) |
| **Already discovered by Home Assistant, waiting for "Submit"** (zeroconf, `confirm_discovery`) | **19** |
| Known to ShellyLanMan, **not offered** by Home Assistant | **6**: LampenHallBovenSwitch (Plus 1), LedKeuken (Plus RGBW), LampKeuken, LampSalon, LampKeukenTafel (Dimmer G3), ShellyTestPlug (Plug S G3) |

No password-protected Shelly in this installation. The core log has no line about the
six (aborted discoveries are not logged).

So in practice the gap is not "Home Assistant does not find Shellys": it finds most of
them but wants **one click per device** (19 here), and it **misses some** (6 here),
with no way for the user to see why.

## 3. What option B can add

1. **Add many at once**: confirm, after one explicit choice by the user, the discovery
   flows Home Assistant already has (19 clicks → 1).
2. **Offer the ones Home Assistant missed**: ShellyLanMan knows their address
   (mDNS, IP scan, its retries, range-extender clients).
3. **Say what is missing**: which Shellys are not in Home Assistant yet, and why
   (not answering, needs a password, sleeping).

## 4. Ways to do it (all in the ShellyLanMan integration, `shellylanman-ha`)

All go through the Shelly integration's own flows: its checks, its entry data and
its dedupe by MAC stay in charge; ShellyLanMan never writes Shelly config entries
itself.

**4.1 Confirm the existing discoveries** — for each Shelly the user ticks:
`hass.config_entries.flow.async_configure(flow_id, {})` on the `confirm_discovery`
flow Home Assistant already started. Uses only the public flow manager; the step is
a plain confirm without fields. Flows in the `credentials` step are left to the user
(see Q3).

**4.2 Offer a missed Shelly** — two mechanisms:

| | A: discovery flow | B: manual flow |
|---|---|---|
| How | start a Shelly flow with source `zeroconf` and a `ZeroconfServiceInfo` built from ShellyLanMan's data (host, port, name `shelly…-<mac>`) | start the Shelly `user` flow and answer its steps as the user would: device list → "manual" → host, port, verify SSL |
| Result | the device appears under *Discovered* like any other; the user confirms there (or with 4.1) | the entry is created at once (after the choice in our dialog); a password leads to the credentials step |
| Pro | Home Assistant's normal UI and wording; nothing is added without a click in Home Assistant; no secrets | the honest source (it is the user's action); works for Gen2+ behind a range extender (`ip:port`) |
| Con | the "zeroconf" source is synthetic (not from mDNS); `ZeroconfServiceInfo` fields may change between releases; a reviewer of the HACS default list may frown | depends on the Shelly flow's step ids and fields (`user`, `device` = `manual`, `user_manual`); those change more often than discovery; each release must be tested |

**4.3 Where the user does it** — the integration's **Configure** (options flow): a
step *Add Shellys to Home Assistant* listing ShellyLanMan's devices that are not in
the Shelly integration, with per device: discovered by Home Assistant / missed /
needs password / not answering; multi-select; a result step (added / waiting for a
password under *Discovered* / failed with reason). Optionally a **repair issue**
"N Shellys known to ShellyLanMan are not in Home Assistant" that opens that step. The
ShellyLanMan web UI does nothing itself (it has no Home Assistant credentials); in
the app it can link to the integration.

**4.4 Not proposed**

- Adding everything automatically, without a choice: against "nothing without asking"
  and not what Home Assistant users expect.
- Writing Shelly config entries directly: bypasses the Shelly integration's checks.
- A change to the Shelly integration in Home Assistant core (an
  `integration_discovery` step): the clean way in the long run, but outside this
  project's control and timeline.

## 5. Risks and limits

- **Coupling to the Shelly flow.** 4.1 relies on `confirm_discovery` (stable since
  years); 4.2-A on `async_step_zeroconf` and `ZeroconfServiceInfo`; 4.2-B on the user
  flow's steps. CI already runs the integration tests on the oldest supported and the
  newest Home Assistant; option B needs tests that run the real Shelly flow
  (`pytest-homeassistant-custom-component` loads core integrations) against a mocked
  device (aioshelly), on both versions.
- **Passwords.** ShellyLanMan stores device credentials encrypted; handing them to
  Home Assistant means a new API that returns secrets (to the integration's token)
  and a copy in Home Assistant's config entries. Safer default: the user types the
  password in Home Assistant's credentials step (Q3).
- **Sleeping devices** are added when awake; the list says "sleeping, add later".
- **Two integrations, one device**: since Home Assistant 2026.8 the Shelly device and
  ShellyLanMan's device stay two linked devices (phase 12 §8) — option B does not
  change that.
- **HACS default list**: the review looks at what an integration does to others;
  4.1 and 4.2-B use only public, user-initiated paths; 4.2-A is the debatable one.

## 6. Questions for Wim

1. **Scope:** both 4.1 (confirm Home Assistant's own discoveries in one go) and 4.2
   (offer the ones it missed)? Or only 4.1 first?
2. **Mechanism for missed Shellys:** A (they appear under *Discovered*, confirmed in
   Home Assistant) or B (added from our dialog through the manual flow)?
   Recommendation: **B** — the user's choice is made in our dialog anyway, it is the
   honest source, and it also covers range-extender clients.
3. **Passwords:** the user types them in Home Assistant (recommended), or ShellyLanMan
   passes its stored credentials after an explicit opt-in per run?
4. **Where:** options flow *Add Shellys to Home Assistant* plus a repair issue when
   devices are missing — agreed?
5. **The six missed on `ha-test`:** first find out why Home Assistant did not offer
   them (Shelly debug logging on `ha-test` for a while, read only) before building, so
   the dialog can name the reason?

## 7. Plan once the questions are answered

| Step | What |
|---|---|
| 13.1 | DECISIONS P13-x; the reason for the six (debug logging on `ha-test`) |
| 13.2 | ShellyLanMan API: what the integration needs per device (address, port, auth, battery, gen) — mostly there in `/api/v1/devices` |
| 13.3 | Integration: options-flow step with the list and states; result step |
| 13.4 | 4.1 confirm existing discoveries |
| 13.5 | 4.2 offer missed devices (A or B) |
| 13.6 | Repair issue (if agreed) |
| 13.7 | Tests with the real Shelly flow and mocked devices, on both Home Assistant versions; on `ha-test` with Wim: add a few Shellys, check entities, remove again |
| 13.8 | Docs (README of both repos, COMPATIBILITY), release on Wim's go |

## 8. Answers and design (2026-10-04) — DECISIONS §22

Q1 both, Q2 B (manual flow), Q3 pass passwords on, Q4 Configure + repair issue,
Q5 find out why six were missed.

**Passwords on a trusted path only.** ShellyLanMan's REST API has no login on the LAN,
so `GET /api/v1/devices/{id}/credentials` answers only

- on the LAN port with `Authorization: Bearer <MCP token>`, MCP on, access level
  *configure* (the token is the one secret ShellyLanMan already shares with Home
  Assistant; *configure* already allows setting device passwords), or
- on the Home Assistant app's loopback listener (`127.0.0.1:8097`, app option
  `mcp_local`), which exists only in the app and is reachable only on its host —
  the same trust as the token-less MCP there.

The integration tries the token first, then — when its own URL is a loopback
address (the app) — the loopback listener; otherwise the user types the password in
Home Assistant. Gen2+ credentials always use the user `admin`.

## 9. Progress and findings (2026-10-04)

| Step | Commit | Notes |
|---|---|---|
| 13.2 | `98f214e` | `protected` in the device list; the credentials endpoint (Go tests: no token, wrong token, access *control*, *configure*, unknown device, loopback listener) |
| 13.3–13.6 | `shellylanman-ha` `251ac8b` | `shelly_add.py` (states, adding through the Shelly flows), `add_flow.py` (list + result, options flow), `repairs.py` (fixable issue), strings en/nl; tests against Home Assistant's real Shelly flow on 2026.9.4 and 2026.10 |

**Q5, first look** (Shelly + zeroconf debug logging on `ha-test` since 02:5x, read
only): within an hour three of the six (LampenHallBovenSwitch, LedKeuken,
ShellyTestPlug) were announced by mDNS and Home Assistant offered them. They had not
been heard before; ShellyTestPlug even disappeared from mDNS for a while (03:30,
weak Wi-Fi). The three Dimmer G3s are still missing; logging continues. So "missed"
is mostly "not heard yet" — exactly what the list now shows and fixes.

Open: 13.7 on `ha-test` with real devices (needs Wim's go: the Shelly integration
connects to the devices it adds), docs of the main README, release 0.7.0.
