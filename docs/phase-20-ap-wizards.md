# Phase 20 — two wizards for a device's own access point (analysis)

*2026-10-07. From `docs/roadmap.md` §2.3 and the maintainer's answers. Nothing is built.*

A Shelly that is new or reset opens its own Wi-Fi access point (AP), and the phone does the
work there: the server cannot reach `192.168.33.1`, a phone that joined the AP can. Two jobs
follow the same pattern and become **two wizards**, built from the same parts:

| | Update firmware through the AP | Set up a new Shelly (provisioning) |
|---|---|---|
| Needs | the firmware file for the model | a profile (and the home Wi-Fi's password, typed by the user) |
| 1 | the file on the phone (QR/link), **before** leaving the home Wi-Fi | choose the profile |
| 2 | join the device's AP (QR) | join the device's AP (QR) |
| 3 | open `http://192.168.33.1` (QR) | open `http://192.168.33.1` (QR) |
| 4 | in the device's page: upload the file | in the device's page: enter the home Wi-Fi (SSID, password) |
| 5 | ShellyLanMan waits until the device is back and shows the new version | ShellyLanMan sees a new device on the LAN, applies the profile, shows the result |

The first wizard exists in part: a QR/link that gives the phone the firmware file of a device
ShellyLanMan already knows (DECISIONS §4.4). What is new: it works from a model (a device that
is not in the list), the AP/URL QR codes, and the wait at the end.

Shared parts: reading an AP name (below), the two QR codes, and "wait for the device".

## 1. Names of access points

- **Gen2+**: `Shelly<App>-<MAC>`, the MAC as 12 upper-case hex digits: `ShellyPlus2PM-A8032AB636EC`,
  `ShellyPro4PM-F008D1D89064` (Shelly API docs, `WiFi.GetConfig`/`WiFi.Scan` examples). The default
  AP name is the device id; it can be changed (`ap.ssid`, up to 29 characters), and then it says
  nothing. `<App>` is the `app` of `Shelly.GetDeviceInfo` (`Plus2PM`, `DimmerG3`, `Mini1PMG4`), which
  ShellyLanMan's model list is keyed by: the model follows from the name, case-insensitively.
- **Gen1** (Shelly's knowledge base, *Device identification*, confirms `shelly1`, `shelly1l`, `shelly1pm`,
  `shellyswitch25`, `shellyht`, `shellytrv`, `shellymotion2`; the other pages do not say): the host name style, `shelly1-BA6201`, `shellyplug-s-80646F838136`, `shellyrgbw2-A894A1`
  (observed as the host names of the maintainer's devices; the AP name is the same by default —
  to verify on a device). The part before the last dash is a lower-case slug, not the type id
  (`SHPLG-S`): a table slug → type is needed, built from the host names of the fixtures and Shelly's
  firmware index, and checked against the original (ShellyScanner).
- **Seen on the maintainer's devices (read only, 2026-10-07):** a Gen3 Plug S (`app: PlugSG3`) has the AP
  `ShellyPlugSG3-54320467CBD4` = `Shelly` + app + `-` + MAC, and `is_open: true`. A Gen1 Plug S
  (`/settings` → `wifi_ap`: `ssid` `shellyplug-s-80646F838136`, `key`, `enabled`) has an AP named like its host
  name, `shellyplug-s-<MAC12>`, with the type `SHPLG-S`: so the slug is the host name's part before the MAC,
  and `wifi_ap.key` exists. The AP of both is **switched off** (the checklist's AP column): see §6.
- The wizard asks for the AP name as the phone shows it, or the MAC from the label, and shows
  *recognised as: Shelly Plus 2PM*; when it cannot tell (renamed AP, unknown model) the user picks
  the model from a list. For provisioning the model is only a nicety (the profile does not depend on
  it); for the firmware wizard it is needed.

## 2. The QR codes

- Join the AP: `WIFI:T:nopass;S:<ssid>;;` (open) or `WIFI:T:WPA;S:<ssid>;P:<password>;;`, with `\ ; , : "`
  escaped. Phone cameras on Android and iOS offer to join.
- Open the page: the URL `http://192.168.33.1`.
- Everything the phone needs from ShellyLanMan is loaded **before** it joins the AP: once it is on
  the AP it has no internet and does not see ShellyLanMan.

## 3. The AP stays open

Decided: the wizards work with the AP **as it is from the factory, open**. No AP password in the
profile or the wizards, so the join QR is always `WIFI:T:nopass;…`, there are no length or
complexity rules to meet, and no password in a QR code. (Gen2+ could protect the AP with
`WiFi.SetConfig {config:{ap:{pass,…}}}`; WPA2 would then need 8–63 printable ASCII characters. That is
left alone, and a protected AP of a device is the user's own business: the join QR would not know
its password.)

- The home Wi-Fi's password is **not** kept in the profile and never sent to the browser (README,
  *Security*: Wi-Fi passwords are write-only). The user types it in the device's page; the profile may
  hold the SSID. The device login is stored encrypted, as ShellyLanMan does now.

## 4. The profile

Name pattern (`{model} {mac4}`-style), device login, MQTT, NTP, cloud on/off, and the checklist's
eco/LED/AP/roaming/logs/auto-update settings and, as a hint only, the home Wi-Fi's SSID. Applied with the code that applies these settings to known devices
(`service.ConfigApply`, the checklist), in step 5, to the device that appears on the LAN with a MAC
ShellyLanMan has not seen: `device.upsert` plus an "is new" mark; the user confirms before anything
is written. The device's new login is stored as its device credentials, so ShellyLanMan keeps
reaching it.

## 5. Firmware 2.0 and the RED requirements (`docs/roadmap.md` §1)

Devices shipped with 2.0+ may behave differently on the AP (a time-limited setup window, a
protected AP, HTTPS). The maintainer's devices are updated ones, not shipped with 2.0: `Shelly.GetDeviceInfo`
shows `provision: "complete"`, `enhanced_security: false`, an open AP; so what a *factory-new* 2.0 device
does on its AP is still unknown and is looked at when one is at hand.

## 6. What the wizards must handle

- **The AP is off** on a device whose AP was switched off in the checklist (both test devices): the
  firmware wizard offers "switch the AP on" for a Gen2+ device ShellyLanMan can still reach
  (`WiFi.SetConfig ap.enable`) and says what to do for one it cannot reach. **Not for Gen1**: a test of
  `/settings/ap?enabled=true` on the Gen1 plug took it off the LAN (2026-10-07, DECISIONS P20-10), so
  the wizard only says how to switch it on in the device's page.
- A device on 2.0.1 reports `sys.restart_required: true` after an update (see the roadmap): the
  wizard's "wait for the device" must not mistake that for "not finished".

## 7. Answered

1. No AP password (§3).
2. Checked on a Gen1 and a Gen3 device (§1); the slug table is still to build.
3. The user types the Wi-Fi password in the device's page; a hybrid may be tried later.

## 8. Built

*The firmware wizard, 2026-10-07 (DECISIONS P20-7 to P20-9).* `model.ParseAPName` and the Gen1 slug table
(`internal/model/apname.go`), `service.APGuide` / `LocalDownloadModel`, `GET /api/v1/ap/guide`,
`GET /api/v1/ap/models`, `POST /api/v1/firmware/local`, and the wizard in the browser
(`web/src/panels/apwizard.ts`, logic in `aplogic.ts`). Tested with unit tests, the route test of the
OpenAPI description, and `tools/screenshots/check-apwizard.py` under the real CSP. The provisioning
wizard and the profiles follow; they reuse the name reading, the QR codes and the wait.

*The provisioning wizard and the profiles, 2026-10-07 (DECISIONS P20-11, P20-12).* `store.Profile`,
`service/profiles.go` (`SaveProfile`, `ApplyProfile`, `ProfilePlan`), `/api/v1/profiles`, Settings →
Profiles (`web/src/pages/settings-profiles.ts`, logic in `profilelogic.ts`) and
`web/src/panels/provwizard.ts`. Tested with unit tests (service, API, web logic) and
`tools/screenshots/check-provision.py`; not yet with a real, factory-new device (a new device's
behaviour on its access point under firmware 2.0 is still to be looked at, section 5).
