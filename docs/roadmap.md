# Roadmap — long-term items

Things to keep in mind and do later. Nothing here is decided or started. An item that
is new beyond ShellyScanner needs the maintainer's explicit approval first
(`CLAUDE.md`, standing rules); the order below is a suggestion, not a plan.

## 1. Shelly firmware 2.0 and the EU Radio Equipment Directive (RED)

*Added 2026-10-07.* Sources: Shelly knowledge base, *What you need to know: Shelly and
EU RED* (https://kb.shelly.cloud/knowledge-base/kbuca-what-you-need-to-know-shelly-and-eu-red);
Shelly API docs, *Setting up HTTPS on your Shelly device* (`docs/General/CustomHTTPSCertificates.mdx`).
The first is a summary of a web page, read once: check it again before building on it.

### 1.1 What changes (firmware 2.0+, ESP32 devices, EN 18031-1)

Delegated Regulation (EU) 2022/30 is mandatory since 2025-08-01. Gen1 (ESP8266) is
not covered. For devices on firmware 2.0 and newer:

- **HTTPS and WSS enforced**: plain HTTP on port 80 redirects to HTTPS on 443, `ws://`
  to `wss://`; TLS 1.2; per-device certificate from Shelly's private CA. This is the
  state of devices **shipped** with 2.0+. Devices only **updated** to 2.0+ have no
  factory certificate and keep serving plain HTTP, unless `enhanced_security` is switched
  on by hand (`Shelly.GetDeviceInfo`).
- **Progressive lockout** after failed logins (10 s → 30 s → 60 s → 5 min), rate limiting
  on every authentication endpoint.
- **15-minute setup window** after a power cycle or factory reset; after that no
  unauthenticated access until a physical reset.
- **Bluetooth off** after the setup; switching it on again opens a 15-minute pairing
  window; pairing moves from "Just Works" to encrypted.
- A `provision` property in `Shelly.GetDeviceInfo` (`pending`, `locked`; `complete` seen).
- Certificates are managed with `Shelly.PutHTTPServerCert`, `…Key`, `…CABundle` (mutual
  TLS) and `Shelly.PutUserCA` (outbound); a reboot is needed after each change.

### 1.2 What the maintainer's own devices show (read only, 2026-10-07)

- The Gen3 and Gen4 devices run **2.0.1** with `enhanced_security: false`,
  `provision: "complete"`, no authentication: plain HTTP keeps working, nothing breaks today.
- Nearly every Gen2+ device reports `sys.restart_required: true`, also with 7 days of
  uptime. This looks like the firmware's own flag after the update (cleared only by a
  reboot), not a ShellyLanMan fault — a **hypothesis**. It fits the first firmware
  update through ShellyLanMan (2026-10-06): after loading, the device seemed to need a
  reboot; a manual reboot fixed it, RPC already reported the new version. Look at it
  again at the next update (item 4).

### 1.3 Where ShellyLanMan is exposed

ShellyLanMan talks to devices over `http://` and `ws://` only
(`internal/shelly/rpc.go`, `shelly.go`, `stream.go`, `internal/service/firmware.go`,
`internal/service/info.go`). For a device shipped with 2.0+:

1. **Connecting**: Go follows the HTTP redirect, but the TLS check fails on Shelly's CA;
   a WebSocket does not follow a redirect. Discovery (IP scan, mDNS), polling, script
   logs and the firmware update's wait for the device would not work.
2. **Lockout**: a device that is polled with missing or wrong credentials locks itself,
   and with it the user's own browser. How often ShellyLanMan retries after a 401
   (polling, rescan, BLU relays, deferred tasks, MCP) is not checked yet.
3. **Setup window**: a new device left without a password for more than 15 minutes is
   unreachable. A stored device password becomes the norm, not the exception.
4. **Bluetooth**: unknown whether "off after setup" also ends receiving BTHome. It decides
   whether the Checklist's Bluetooth column, relayed BLU rows and *Identify BLU devices* keep working.
5. **Firmware from this server** (QR code, local firmware): the device fetches over
   `http://` from ShellyLanMan; unknown whether 2.0+ still allows that.

There is a precedent in the code for item 1: `internal/firmware/firmware.go` pins a certificate.

### 1.4 Suggested order

| # | Item | New beyond ShellyScanner? |
|---|---|---|
| 1 | **Find out first, no code**: read and test a device shipped with 2.0+ (or one with `enhanced_security` on): what the redirect does, Bluetooth/BTHome, the lockout, firmware from this server. Real devices only with the maintainer's say-so | no |
| 2 | **Do not hammer on 401**: back off after an unauthorised answer, show that the device asks for a password. Check every place that retries | no (robustness) |
| 3 | **HTTPS/WSS support**: `https://` and `wss://` to devices, trust the device certificate on first use and remember it (as the firmware download does), try port 443 in discovery; maybe import Shelly's CA chain once published | yes — ask |
| 4 | **The firmware update's "reboot needed"**: compare what the UI shows with `Sys.GetStatus` (`restart_required`, `available_updates`) and how `internal/service/firmware.go` decides *done*; see the 2026-10-06 observation | no |
| 5 | **Show `provision`** in the Checklist, and a hint "no password yet, the setup window ends" | yes — ask |

Out of scope unless asked: managing certificates or a private CA on the devices
(`Shelly.PutHTTPServerCert` and friends) — that is a write to the device's security
configuration.

## 2. Ideas beyond ShellyScanner

*Added 2026-10-07, agreed in principle by the maintainer; each still needs its analysis and
decisions (`DECISIONS.md`) before it is built. Order is a suggestion.*

### 2.1 An update arrow on the devices list

*Done 2026-10-07 (DECISIONS P19-1).*

A device with a newer firmware gets a small **↑** in its status, like the **↻** for "reboot
needed" (`web/src/pages/devices.ts`): in the status label, a summary chip, a selection
filter, a tooltip. ShellyScanner shows a red dot for this.

- The device says it itself: Gen2+ `Sys.GetStatus` → `available_updates` (stable and
  beta), Gen1 `/status` → `has_update`. Neither is read today (`internal/parse/gen2.go`
  reads `restart_required` only). It costs no extra request and nothing leaves the LAN.
- Open: stable only or beta too (the Firmware page shows both); how it relates to the
  Firmware page, which compares with Shelly's index on the server.

### 2.2 OpenAPI description of the REST API

*Done 2026-10-07 (DECISIONS P19-2, P19-3).*

`/api/v1` described as OpenAPI, so other programs and AI tools can use it.

- Reachable from other machines on the LAN like the rest of the API: the same address and
  port. With a UI password set, calls need the session or the MCP token as a bearer
  token (DECISIONS P15-3), the same as every other `/api/v1` call.
- Open: serve the description itself open or behind the login; how it is kept equal to the
  handlers (a test, as the README's compose file is); a page that shows it, or the JSON only
  (an interactive viewer is a dependency and needs the CSP's nonce).

### 2.3 Provisioning a new device through its access point, with profiles

*Analysed 2026-10-07: two wizards, `docs/phase-20-ap-wizards.md`, DECISIONS §29.*

A new or reset Shelly opens its own Wi-Fi access point (`192.168.33.1`). ShellyLanMan
sets it up from a stored **profile**: Wi-Fi, login, name pattern, MQTT, NTP, cloud off, …

- The hard part is the network: the machine that talks to the access point must be on
  that Wi-Fi, which a Docker host on the LAN usually is not. Options to analyse: the
  server host joins the access point; or the **browser** on a phone or laptop that is on
  the access point does it (Shelly sends CORS headers; a page served over HTTPS may not
  call `http://192.168.33.1`); or a hybrid with the profile fetched first.
- Ties in with the firmware 2.0 setup window of 15 minutes (section 1): a new device must
  get its password quickly.
- Open: what a profile holds; naming (`shellyplus1-<mac>`-style names to a chosen name);
  the checklist after provisioning.

### 2.4 Scheduled backups — only if it is easy

Backups on a schedule, kept per device (`backupKeep` exists). The maintainer sees little
use; do it only if it is a small step, otherwise leave it.
