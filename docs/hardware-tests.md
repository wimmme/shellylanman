# Hardware tests

Results of testing against real devices (Wim's installation). CI never touches
real devices; these runs are manual, per phase. Device-identifying data is left
out on purpose (the repository is public).

## Phase 2 — discovery (2026-09-26)

Setup: `shellylanman:dev` image on dockerhostvm (Linux, x86_64), `network_mode: host`,
port 3099, fresh `/data` volume; scan mode "Full mDNS scan".

| Check | Result |
|---|---|
| mDNS browsing | 43 `_http._tcp` instances in 25 s; only `eth0` and `tailscale0` used (44 Docker bridges skipped) |
| Devices found and identified | **25 of 25** Shellies that were powered: Gen1 (Shelly 1L ×4, Shelly I3, Shelly RGBW2, Shelly UNI, PlugS ×3, Shelly 1), Gen2 (Shelly +1, Shelly +UNI, Shelly +RGBW ×2), Pro (Shelly Pro RGBWW PM, Shelly Pro 3EM — a 3EM-63 reporting app `Pro3EM`), Gen3 (Shelly Dimmer G3 ×3, Shelly Dimmer 0/1-10V G3, Shelly Mini PM G3 ×2, Shelly i4 G3), Gen4 (Shelly Mini 1PM G4). Type names match ShellyScanner. |
| Duplicate mDNS names (Gen2+ announce under id and custom name) | one row per device (MAC) |
| Status | all on line; refresh every 2 s with the browser open |
| Archive | 25 devices written to `/data/archive.json` within 5 s |
| Restart | after 2 s: 11 archived + 14 on line; after 22 s: 24 on line; the last one (a Plus RGBW PM known to drop off at times) came back via mDNS/auto reload within ~2 min |
| IP scan (192.168.0.1–254) | finished in 20 s, the same 25 devices found and on line |
| Resources | 7.5 MiB memory, ~0% CPU with no browser connected (presence mode, 60 s) |
| UI | first-run dialog, live device table sorted by IP, summary cards, Settings → Network |
| BLU | no BLU devices present behind the Pro/Gen3/Gen4 gateways — BLU paths tested only with the simulator |
| Protected devices | none in this installation — authentication tested only with the simulator |
| Range extender | none in use — tested only with the simulator |

Still to test on hardware when available: a password-protected Gen1 and Gen2+
device, a BLU device behind a gateway, a range extender and a battery device
waking up. Rows D8 (range extender), D9 (BLU) and D18 (protected devices) of
`FEATURE_PARITY.md` stay 🔨 until then.

## Phase 3 — read-only device information (2026-09-26)

Same setup (dockerhostvm, host networking, port 3099).

| Check | Result |
|---|---|
| Table columns for all 25 devices | RSSI, SSID, cloud/MQTT enabled+connected, uptime, internal temperature, measurements, logs mode, source and command state filled for every generation; Pro devices on Ethernet show RSSI 0, as in ShellyScanner |
| Values against the devices | spot checks: PlugS (power, temperature), Mini 1PM G4 (output, voltage, temperature), Mini PM G3 (W, V, I, f) match the device's own API |
| Energy meter | Pro 3EM (triphase): phases a/b/c with W, VA, PF, V, I, f and the total set |
| Profiles | Plus RGBW PM ×2 and Pro RGBWW PM in "light" profile: 4 / 5 lights with W, V, I each |
| Device info | tabs of the Pro RGBWW PM load live JSON |
| Live logs | Dimmer 0/1-10V G3 (websocket debug enabled): log stream relayed, level filter applied |
| Filter | filtering by text while devices keep updating |

Not yet on hardware: Sensor Add-on and BLU readings, battery devices' stored data,
the Gen1 log files of a device with debug enabled, protected devices' live log.

## Phase 4 — device controls (2026-09-26)

Same setup. Writes only to the agreed test device **Grondwaterpomp** (PlugS Gen1,
192.168.0.86); all other devices were only looked at.

| Check | Result |
|---|---|
| Relay toggle (Grondwaterpomp) | `POST …/command {"key":"relay/0","action":"toggle"}` → device `ison` false → true, source `http`; the table showed the new state in the same request (status read after the command). Toggled back to the original state (off) |
| Reboot (Grondwaterpomp) | without `confirm` → 428; with `confirm` → 202, row "reading", back on line after ~6 s with uptime 4 s |
| Layouts of the real devices | relays → `relay`; Dimmer G3 (single light) → slider panel; RGBW2 white ×4, Plus RGBW PM ×4, Pro RGBWW PM ×5 → one line per channel with the lights-editor button on the last, as the Java cell editor |
| Input actions | Shelly i3: 8 Gen1 actions per input, all disabled → only labels (more than 5 events: disabled ones hidden); i4 G3: `Webhook.List` empty → labels only |
| Input indicator | Dimmer G3 (.76) and Shelly 1L (.88) with input on: ON/OFF text highlighted |
| Lights editor | opens with "All channels" and one panel per channel (Plus RGBW PM), no console errors |

Not tested on hardware (no such device here, or writes not allowed): covers, RGB/RGBW/
RGBCCT colour commands, thermostats (Wall Display, XT1, BLU TRV, Gen1 TRV), circuit
breaker, camera, executing input actions, Gen2+ commands with authentication (covered
by simulator tests with Digest and JSON-RPC auth).

## Phase 5 — configuration (2026-09-27)

Same setup. Writes only to **Grondwaterpomp** (PlugS Gen1, .86), each change set
back to its original value; the other devices were only read.

| Check | Result |
|---|---|
| Forms for all 25 devices (read only) | Wi-Fi 1/2, login, MQTT ("mix"), others: values merged as the Java panels (empty where devices differ, "Keep" for mixed DHCP/static) |
| Checklist for all 25 devices (read only) | values per generation match the devices (socket logs on .76/.158/Deurbel, Ethernet Pro devices "-" for wi-fi1, roaming thresholds, scripts count, auto FW "✗"); rows fill in one by one — a slow device (+UNI, 7 s for `Shelly.GetConfig`) no longer delays the others |
| NTP | `time.google.com` → `pool.ntp.org` → back; device settings changed each time |
| Cloud | set to disabled (unchanged) — call accepted |
| MQTT (Gen1 panel) | form shows the Gen1 extras (reconnect 60/2, clean session, keep alive 60, QoS 0, retain off, update period 30); applying the same values succeeded, settings unchanged |
| Restricted login | enabled with a temporary password: the device answered 401 without credentials, ShellyLanMan kept reading it (new credentials stored); disabled again → open |
| Checklist actions | LED off on/off, logs on/off (row now shows the new state at once), roaming off/on (threshold kept), eco on/off → "reboot required" until the device was rebooted |
| UI | settings dialog tabs, checklist page and deferred page render without console errors |

Not tested on hardware: Wi-Fi changes (the device's Wi-Fi password cannot be read
back, so a test could not restore it — simulator tests only), Gen2+ writes (no Gen2+
test device), deferred tasks on a real device going off line and back (simulator
tests), BLE info of BLU devices (none here), auto firmware update schedule (Gen2+).

## Phase 6 — backup and restore (2026-09-27)

Same setup (test container on dockerhostvm, port 3099). Backups and restore checks
only read; the only restore was on **Grondwaterpomp** (PlugS Gen1, .86), with its own
backup.

| Check | Result |
|---|---|
| Backup of all 25 devices (Gen1 PlugS/1/1L/i3/RGBW2/UNI, Plus 1/RGBW PM/UNI, Pro RGBWW PM/3EM, Gen3 Dimmer/0-10V/i4/Mini PM, Gen4 Mini 1PM) | 25 × success in ~20 s per batch; Gen1 files 2 entries, Gen2+ 7 entries (+UNI with peripherals), scripts included |
| Restore check of every device with its own backup (read only) | no errors; i4 G3 and +UNI ask about the colliding script name, +UNI asks the protected AP password — as the Java checks would |
| Restore wizard on Grondwaterpomp | source list (own / other device / file, download link), confirmation, "Success"; device stayed on line |
| Backup after restore compared with the one before | identical except time and `lat`/`lng`: `tzautodetect=true` makes the device locate itself again after the `/settings` write (O26, same in the original) |

Not tested on hardware: Gen2+ restore (no Gen2+ test device for writes — simulator
tests cover G2 order, scripts, schedules, KVS, webhooks, login last), Wi-Fi/login/MQTT
password questions with real values, BLU backup/restore (no BLU devices here), battery
devices from stored data, multi restore (simulator and API tests).

## Phase 7 — firmware (2026-09-27)

Test container on dockerhostvm (port 3099). No firmware update was sent to any device.

| Check | Result |
|---|---|
| FW rows of all 25 devices (read only: `/ota/check` + `/ota`, `Shelly.CheckForUpdate` + `Shelly.GetDeviceInfo`) | Gen1 all 1.14.0 with beta 1.14.1-rc1 offered; Gen2 Plus1 1.7.5 (nothing newer); Plus/Pro/Gen3/Gen4 2.0.1; LampSalon (Dimmer G3) 2.0.0 with stable 2.0.1 offered and preselected |
| Shelly index, live (pinned TLS) for every device | 25 answers; types and apps match (Gen4 app `Mini1PMG4`); only LampSalon newer (2.0.0 → 2.0.1) — same as the device's own check |
| Local download for LampSalon | link + QR created; file fetched once (3.8 MB, 0.3 s), SHA-256 and manifest (`DimmerG3 2.0.1`) verified, cached; second download from cache |
| Live index/download test (`SHELLYLANMAN_LIVE=1 go test -run TestLive ./internal/firmware`) | SHPLG-S 1.14.0 (Gen1, HTTP), Plus1 1.7.5, MiniPMG3 2.0.1, Mini1PMG4 2.0.1 downloaded and verified |
| UI | Firmware page with index column and ⚡, QR modal, FW Update as first settings tab; no console errors |

Not tested on hardware (Wim: leave the LampSalon update for now): an actual firmware update with progress and restart (every
test-allowed device is up to date; the only candidate, LampSalon, is not a test
device — simulator tests cover progress, restart and recheck), BLU TRV update,
flashing a downloaded file through a device's access point.

## Phase 8 — advanced functions (2026-09-27)

Test container on dockerhostvm (port 3099). Only reads on the devices; no scheduler,
script or KVS change was applied.

| Check | Result |
|---|---|
| Scripts dialog on LedKeukenOnderSchakelaar (i4 G3) and Deurbel (+UNI) | script list (aioshelly_ble_integration, running), code read (Script.GetCode), KVS tab empty |
| Script editor (i4 G3, before the log fix) | toolbar, code, caret position; the output pane could not connect because the device's websocket debug log is off — opening the editor now switches it on like the original (O30), so it was not opened again on a non-test device |
| Scheduler on LampenHallBovenSwitch (Plus1) | no jobs → one empty job line with the cron editor, calls, hints menu; closed without applying |
| Charts: Laadpaal (Pro 3EM) + Grondwaterpomp | graph types offered as the original (temperature, RSSI, power, power sum, apparent power, voltage, current, frequency, energy); power series per phase, history from the server since the start of the container |
| About page | help sections and the credits of the bundled libraries shown |

Not checked in the browser: notes, CSV export and print (unit tests for the CSV text). Not tested on hardware: writing schedules, scripts or KVS (no Gen2+ test device for
writes — simulator tests cover the calls), Wall Display and BLU TRV schedulers (no such
devices here), the EM chart on a real Pro 3EM over a long period.

## Phase 9 — parity review (2026-09-27)

New test device **ShellyTestPlug** (Plug S Gen3, 192.168.0.150, firmware 1.2.3 at start).

| Check | Result |
|---|---|
| Discovery of the new plug | found while it was still starting: "unmanaged, error" (timeout on `/shelly`) until Reload — same as the original (O33); after Reload: Plug S G3, relay, W/V/I |
| Local mDNS scan (one interface, eth0) | 25 instances found; set back to full scan |
| Reboot (Gen3) | offline ~70 s, back on line, `restart_required` cleared |
| Firmware check | before the reboot the plug offered nothing; after it: stable 2.0.1 (preselected); Shelly index: 2.0.1, ⚡ offered |
| Firmware update 1.2.3 → 2.0.1 from the Firmware page | `Shelly.Update` sent, first `ota_progress` (1 %) shown as "load 1%"; the plug then reported nothing more and a second request answered "Already in progress"; after 20 minutes still 1.2.3 — the download on the plug appears stuck (weak Wi-Fi, −73 dBm) |
| Firmware update, second attempt the next day (13 h later, `Shelly.Update` sent directly to the plug) | accepted (`result: null`), but again no progress; still 1.2.3 after 9 minutes — the plug does not complete its own download |
| Devices row menu (right-click) | Device info, Web UI, Settings, Backup, Restore, Notes, Reload; closes on click |
| KVS set / list / delete | ok |
| Schedule create (disabled) / list / delete through the RPC endpoint | ok |
| Script create, put code (1609 characters, two segments), read back, delete | ok |
| Backup, restore own backup (scripts overwrite + enable like backup), backup again | success; the two backups are identical except `rev` counters |

## Phase 10 — release (2026-09-28)

| Check | Result |
|---|---|
| GitHub Actions "Test" on every push | green (last 8 runs) |
| arm64 build (cross-compiled `build` stage on dockerhostvm) | static ARM aarch64 binary, 10.4 MB; the final stage needs QEMU, which is not installed on dockerhostvm — it runs on GitHub at the first tag |
| Release check on the test container | setting off: no request; stable: checked, no release yet, no error; back to off |
| Retry of failed devices every 2 minutes | simulator test (device down when discovered, back later → on line) |

## MCP server (2026-10-01)

Live container on dockerhostvm (port 3082), MCP calls over Streamable HTTP with the
bearer token. Access set to "control" for the test and back to "read" afterwards; both
test devices left as found (ShellyTestPlug off, Grondwaterpomp on).

| Check | Result |
|---|---|
| Read tools in Claude Code (`shelly_list_devices`, `shelly_checklist`, `shelly_rpc_read`) | answers for all devices; checklist cells as the UI |
| `tools/list` in read / control mode | 7 read tools / 14 tools |
| `shelly_switch` on ShellyTestPlug (Gen3) and Grondwaterpomp (Gen1) | on/off confirmed by reading the devices directly (`Switch.GetStatus`, `/relay/0`) |
| `shelly_backup` of both, `shelly_list_backups` | `.sbk` files written and listed |
| `shelly_reboot` / `shelly_firmware_update` without `confirm` | refused, nothing sent (logged) |
| `shelly_firmware_update` stable with `confirm` on ShellyTestPlug (already 2.0.1, 2.1.0-beta1 offered) | the device answers "FW stage stable not found", reported as a failed result line |
| `shelly_reboot` with `confirm` on ShellyTestPlug | down ~30 s, back up; ShellyLanMan showed "error" for about 2 minutes before "online" — the plug itself: after the reboot it answered ping only after minutes and slowly, its web UI stayed unreachable (1 m from the access point) |
| Ambiguous name (`"Lampen"`) | refused with the 7 candidates |

Seen on the way: after the container restart ShellyTestPlug stayed a ghost (archive)
until a Reload, although it answered on its address — the mDNS scan did not find it.
Not tested on hardware: `shelly_light`, `shelly_cover`, `shelly_thermostat` (no test
device of those kinds; simulator tests cover them).

## Phase 11a — MCP parity (2026-10-01)

Live container on dockerhostvm, MCP access "configure" for the test, "read" afterwards.
Writes on ShellyTestPlug (Plug S Gen3, 2.0.1) only, each undone; Grondwaterpomp read only.

| Check | Result |
|---|---|
| `shelly_get_config` Grondwaterpomp (Gen1 /settings) | full settings; this firmware returns no passwords (AP key empty), masking covered by unit tests |
| `shelly_get_status` / `shelly_list_components` (TestPlug) | `switch:0` status; components incl. `script:1`, `script:2` (existing, left alone) |
| `shelly_energy_history` Laadpaal (Pro 3EM, 2 h) | EMData per minute, 3 lines |
| `shelly_switch` with `timer_s: 5` | on, off again after 5 s (`source: timer`) |
| KVS set (object value) / read / delete | ok |
| Schedule create (disabled) / update / delete | ok, `Schedule.List` checked |
| Script create / put code / append / read code / start / eval (`sllNext()` → 42) / stop / delete | ok; scripts 1 and 2 unchanged |
| Webhook create (disabled) / update / delete; virtual `boolean:200` add / list / delete | ok |
| `shelly_rpc_write` `Switch.SetConfig` name set and back to `null`; `Shelly.FactoryReset` without `allow_data_loss` | ok; refused |
| `shelly_device_login` set → protected reads and KVS writes → remove | **failed at first**: POST RPC to the protected 2.0.1 plug answered 401 without a body challenge, so every call failed and the plug's brute-force lock answered 429; password removed with the stored credentials, fixed (P11-11), then the whole sequence passed |
| Scene with a switch action and an RPC action: set / list / run / delete | `ok` per step, `scenes.json` empty afterwards |

Seen on the way: Grondwaterpomp restarted by itself during the test (uptime 319 s at
12:16) and came back **off** (its default state); ShellyLanMan sent it no command.

## Phase 11b — Home Assistant app (2026-10-01)

Test system `ha-test` (HA OS 18.3, HA 2026.9.4, Hyper-V). The app installed as a local
app (`/addons/shellylanman`: the 0.3.0 image with a 0.4.0-dev binary, the
`shellylanman-ha` entry script), built on the device.

| Check | Result |
|---|---|
| Install, start | started (`state: started` once the image health check is healthy); ShellyLanMan logs the ingress listener `172.30.32.1:8099` from `172.30.32.2`, LAN port 3082, mDNS on `eth0` |
| `HEALTHCHECK NONE` in the app image | the Supervisor kept the app in `startup` (it treats `{"Test":["NONE"]}` as a health check): the image's own health check is kept |
| Port 8099 from the LAN | not reachable (bound to the Docker gateway) |
| Through ingress (`https://ha-test.wimmme.net/api/hassio_ingress/<token>/`, ingress session) | page, `app.js`, `app.css`, logo, fonts 200; `/api/v1/status` `"ingress": true`; PUT settings with the same Origin 200; WebSocket `/ws` 101; without a session 401 |
| LAN port 3082 | status `"ingress": false` |
| Panel in the Home Assistant sidebar (Wim, browser) | **"refused to connect"** at first: Home Assistant shows ingress apps in an iframe and ShellyLanMan sent `frame-ancestors 'none'` / `X-Frame-Options: DENY`; through ingress it now sends `'self'` / `SAMEORIGIN` (the LAN port still forbids framing) |
| Release: ShellyLanMan v0.4.0, `shellylanman-ha` v0.4.0 (image `ghcr.io/wimmme/shellylanman-ha:0.4.0`, public) | repository added on `ha-test`, app installed and started (`state: started`), runs v0.4.0, ingress listener up; sidebar panel switched on; local dev app removed |

## Phase 11c — Home Assistant integration (2026-10-01)

`ha-test` (HA 2026.9.4) with the app as a local development build (0.5.0-dev) and the
integration copied to `/homeassistant/custom_components/shellylanman`.

| Check | Result |
|---|---|
| App start | ingress listener, local MCP `127.0.0.1:8097`; Supervisor discovery list: `shellylanman {"url": "http://127.0.0.1:3082"}` and `mcp {"url": "http://127.0.0.1:8097/mcp"}` |
| Discovered flows after a Core restart | `shellylanman` at `hassio_confirm` (one click) → entry created, loaded; `mcp` at `user` (HA 2026.9.4 has no `async_step_hassio` in its MCP integration) → URL entered → entry created via the token-less listener, loaded |
| Entities | status sensor per Shelly device (20 on line), ShellyLanMan device: on line 20 / off line 0 / attention 0, version, rescan button |
| Backup button (Grondwaterpomp) | **500 at first**: the client expected a list, ShellyLanMan answers `{"results": [...]}` (the test mock had the wrong shape); fixed, test corrected and a failure test added; then 200 and the last-backup sensor updated |
| Integration tests | 13 passed on Home Assistant 2026.9.4 and 2026.10.0b0 |

Seen on the way: the Supervisor's own "Version" sensor of the app device and ours share
the name, so ours became `sensor.shellylanman_versie_2` (cosmetic). Enabling MCP on a
fresh instance returns the new token in the API answer; on the test instance it was
rotated afterwards.
| Release 0.5.0 on `ha-test` | the store offered the app update 0.4.0 → 0.5.0; after it ShellyLanMan v0.5.0 with the local MCP listener; integration (released code) installed; after a Core restart `shellylanman` (one click) and `mcp` (URL, HA 2026.9) are offered as discovered |
| `shellylanman-ha` CI | tests on HA 2026.9.4 and the newest release, `hassfest` (after sorting the manifest keys) and HACS validation (after adding repository topics) green |

## Phase 12 and release 0.6.0 (2026-10-03)

| Check | Result |
|---|---|
| Pages with 11 simulated devices (`tools/screenshots/run.sh`, headless Chromium) | shared selection Devices → Checklist → Firmware with *Show all devices*; tooltips of grey buttons give the reason (read from the page); Firmware rows filled per device; ☰ menu on a 390 px phone (made opaque after the first look) |
| `ha-test` device registry (read only, before the release) | ShellyTestPlug also listed as `addr:192.168.0.150:80` (unidentified, error) → fixed in `acd978a`; BrandstofcelSwitch twice (ShellyLanMan and Shelly integration, same MAC, not merged): Home Assistant 2026.8+ keeps one config entry per device |
| Release 0.6.0 on `ha-test` | app 0.5.0 → 0.6.0 through the Supervisor (store reload, update); log shows `announced to Home Assistant` for `shellylanman` and `mcp`; integration 0.6.0 copied, Core restarted: 27 ShellyLanMan devices, the `addr:` device removed, no `shellylanman` entries in the system log |
| CI | `shellylanman` Test and Publish image green; `shellylanman-ha` App and Integration green on `main` and `v0.6.0` |
| Release 0.6.1 on `ha-test` | app 0.6.0 → 0.6.1 through the Supervisor, log `starting ShellyLanMan version=v0.6.1`; integration files 0.6.1 copied (no code change, so no Core restart); Charts opened from the menu chart the ticked devices (simulator, headless Chromium) |
| Release 0.6.2 on `ha-test` | app 0.6.1 → 0.6.2; the app's LAN port answers with `style-src 'self' 'nonce-…'` and the same nonce in `index.html`; integration files 0.6.2 (no code change, no Core restart). Script editor checked in headless Chromium with the simulator (`check-editor.py`): unstyled without the nonce (overflow visible, first line at y=2022, CSP refusal), styled with it, light and dark |
| Release 0.6.3 on `ha-test` | app 0.6.2 → 0.6.3; integration files 0.6.3 (no code change, no Core restart). Editor colours checked with `check-editor.py`: light editor in the light app, dark in the dark app, without any setting |
| `Script.GetCode` of an empty script (ShellyTestPlug, Plug S Gen3, fw 2.0.1; Wim's go, 2026-10-04) | `Script.Create` → id 3; `GET /rpc/Script.GetCode?id=3` → HTTP 200 `{"data":"", "left":0}` (POST the same); `Script.Delete` → `null`; scripts 1 and 2 untouched. So "empty" is an answer, not an error: 0.6.4's error handling does not block new scripts |
| Release 0.6.4 on `ha-test` | app 0.6.3 → 0.6.4; integration files 0.6.4 (no code change, no Core restart). Editor behaviour checked with `check-editor.py` (simulator): loading after 0.04 s, one window for two double-clicks, uploads off while loading, code after 4.5 s, failing read → error + Retry, no editor |

## Phase 13 — adding Shellys to Home Assistant (2026-10-04, `ha-test`, Wim's go)

| Check | Result |
|---|---|
| The list (Configure → Add Shellys) | 23 Shellys with the reason: 17 discovered by Home Assistant, 6 not offered; right after a Core restart two online ones showed "not answering" from the last 30-second poll → the list now refreshes first (`shellylanman-ha` `67c9ff6`), after which none did |
| Three first (2 discovered, 1 missed) | Grondwaterpomp (Gen1) and LedBerging (Gen2) through Home Assistant's discovery, LampKeuken (Dimmer G3, not offered) through the manual flow: 3/3 added in 34 s, entries loaded with their own entities |
| The rest | 21/21 added in 33 s (ShellyTestPlug left out: Wim debugs it); 25 Shelly entries in total (19 zeroconf, 6 user), all loaded; no integration errors in the log; the repair issue stays at 1 (ShellyTestPlug) |
| Why six were missed (Q5) | with Shelly/zeroconf debug logging, three of them were announced and offered within the hour; ShellyTestPlug left mDNS for a while (weak Wi-Fi). "Missed" = not heard yet. (Debug logging ended with the Core restarts.) |
| Release 0.7.0 on `ha-test` | app 0.6.4 → 0.7.0, integration files 0.7.0. Credentials paths on the real app: loopback listener `127.0.0.1:8097` answers (404 "no credentials stored" — this installation has none); LAN port without token 403, also from another machine; port 8097 not reachable from the LAN (connection refused). CI of both repositories green, including hassfest and HACS validation |

## Phase 14 — relayed BLU devices and Identify (2026-10-05, dev build on a test port, Wim's go)

| Check | Result |
|---|---|
| Relayed row | IP scan of the gateway only (LampKeukenTafel, Dimmer G3, 192.168.0.117): the RC Button 4 it relays appears as its own row, *BLU online*, "Blu Wall Switch 4 / RC Button 4 ?", battery 100 %, buttons 1–4; Backup, Restore, Logs grey |
| Identify | Wizard from the row: gateway preselected, two-button hint; Wim held two buttons > 10 s → "Blu RC Button 4 · SBBT-004CUS · 7c:c6:b6:a5:c9:3d · −53 dBm · in the list: yes"; the row is "Blu RC Button 4" afterwards. Nothing written to device or gateway |
| Release 0.8.0 on `ha-test` | app 0.7.0 → 0.8.0 through the Supervisor (log `starting ShellyLanMan version=v0.8.0`); integration files 0.8.0 (no code change, no Core restart). Within a minute the RC Button 4 relayed by LampKeukenTafel is a row of its own (27 devices, "… RC Button 4 ?": this archive has not identified it) and a device in Home Assistant through the integration; no integration errors in the log. *Publish image* first failed on a 502 from Docker Hub, green on the rerun |

## Phase 15 — optional UI password (2026-10-05, Wim tested the dev build; release 0.9.0 on `ha-test`)

| Check | Result |
|---|---|
| Dev build on a test port (Wim) | Settings → Security, login, *Stay logged in*, wrong password, log out, switching off: "looks good" |
| Release 0.9.0 on `ha-test` | app 0.8.0 → 0.9.0 (log `version=v0.9.0`, warning that no password is set); integration files 0.9.0 and a Core restart (code changed) |
| Temporary password on the app's LAN port | set through the API: `{"authEnabled":true}`; `/api/v1/devices` without session 401, with the session cookie 200; status `loggedIn:false` without session |
| The integration next to the app with that password | 27 status sensors, none unavailable; a forced refresh (`homeassistant.update_entity`) fine; no ShellyLanMan errors in Home Assistant's log — its calls went to the app's loopback listener |
| Switched off again | `{"authEnabled":false}`; `ha-test` back as before |
| Release 0.9.1 on `ha-test` | app 0.9.0 → 0.9.1 (log `version=v0.9.1`), integration files 0.9.1 (no code change, no Core restart). Changing / switching off without the current password under ingress is covered by `TestIngressNeedsNoLogin`; in the sidebar itself it is for Wim to try (the ingress path needs a Home Assistant browser session) |
| Release 0.9.2 on `ha-test` | app 0.9.1 → 0.9.2 (log `version=v0.9.2`), integration files 0.9.2 (no code change, no Core restart). The sidebar switch was tried by Wim on a dev build first ("super") |

## Ports: one setting (2026-10-06, `ha-test`, local dev app `local_shellylanman_dev`, 0.9.3-dev)

| Check | Result |
|---|---|
| Ingress port from the Supervisor (`ingress_port: 0`) | Supervisor chose 63804; ShellyLanMan listened on `172.30.32.1:63804` (Wim's production install had failed on a fixed 8099 taken by another app) |
| Option `port` on a taken port (8123, Home Assistant itself) | app state *error*; log: `web UI: port 8123 is already used by another program on this host; set another one in Home Assistant: Settings → Apps → ShellyLanMan → Configuration → port` |
| Options `port: 3092`, `mcp_local_port: 8197` | listening on 3092 and `127.0.0.1:8197`; `GET /api/v1/server` `{"port":3092,"source":"app","ingress":"172.30.32.1:63804","mcpLocal":"127.0.0.1:8197"}`; `GET /api/v1/status` `localUrl` `http://127.0.0.1:8197`; app *started*, so the health check follows the port (`/data/listen.port`) |
| Found on the way | the Supervisor writes `options.json` over several lines: the start script's option parser fixed |
| Release 0.9.3 | both repositories released (image, app image, GitHub releases); the dev app `local_shellylanman_dev` stays on `ha-test` for Wim to look at; the production install is Wim's |
