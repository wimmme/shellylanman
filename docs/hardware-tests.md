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
