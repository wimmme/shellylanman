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
- **Gen1**: the host name style, `shelly1-BA6201`, `shellyplug-s-80646F838136`, `shellyrgbw2-A894A1`
  (observed as the host names of the maintainer's devices; the AP name is the same by default —
  to verify on a device). The part before the last dash is a lower-case slug, not the type id
  (`SHPLG-S`): a table slug → type is needed, built from the host names of the fixtures and Shelly's
  firmware index, and checked against the original (ShellyScanner).
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

## 3. The password of the AP

- The default is an **open** network. Gen2+ can protect it: `WiFi.SetConfig {config:{ap:{pass,…}}}`
  (`pass` is write-only, `ap.is_open` says whether it is open); Gen1 has `/settings/ap?key=…`
  (to verify). The Shelly docs give no rules for the length or characters; Wi-Fi's WPA2 itself takes
  **8 to 63 printable ASCII characters**, which ShellyLanMan should enforce in the profile (and
  test on a device).
- A factory-new device has an open AP, so the AP password cannot be set *before* the setup: it is
  part of the profile and goes onto the device in step 5, through the LAN.
- The same password as the home Wi-Fi is *possible* but not a good default: the AP only serves
  setup and the range extender, and one leaked password would then open both; the home network
  may also have a passphrase the AP rules do not allow. Suggestion: a separate **AP password** in
  the profile, with a generated suggestion; "same as the Wi-Fi" only as a visible choice.
- The home Wi-Fi's password is **not** kept in the profile and never sent to the browser (README,
  *Security*: Wi-Fi passwords are write-only). The user types it in the device's page; the profile
  may hold the SSID. Same for the device login: ShellyLanMan stores it encrypted, as it does now.
- The wizard needs the AP password to build the join QR for a device that already has a protected
  AP; ShellyLanMan cannot read it back from the device. It would come from the profile (when the
  device was set up with it) or be typed. A QR with the password is generated on request, behind
  the login, and not kept in the browser.

## 4. The profile

Name pattern (`{model} {mac4}`-style), device login, MQTT, NTP, cloud on/off, and the checklist's
eco/LED/AP/roaming/logs/auto-update settings, plus the AP password (§3) and, as a hint only, the
home Wi-Fi's SSID. Applied with the code that applies these settings to known devices
(`service.ConfigApply`, the checklist), in step 5, to the device that appears on the LAN with a MAC
ShellyLanMan has not seen: `device.upsert` plus an "is new" mark; the user confirms before anything
is written. The device's new login is stored as its device credentials, so ShellyLanMan keeps
reaching it.

## 5. Firmware 2.0 and the RED requirements (`docs/roadmap.md` §1)

Devices shipped with 2.0+ may behave differently on the AP (a time-limited setup window, a
protected AP, HTTPS). Read a device that is new in that sense, with the maintainer's say-so, before
building the AP steps.

## 6. Open questions

1. A QR code that carries the AP password: acceptable (generated on request, behind the login), or
   should the user type it?
2. Gen1: does `/settings/ap` take a key, and what does a Gen1 AP name look like exactly?
3. A new device without the profile's Wi-Fi: is "the user types the Wi-Fi password in the device's
   page" good enough, or is a hybrid (the page on the phone sets it, when the phone still reaches
   ShellyLanMan) worth trying later?
