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
