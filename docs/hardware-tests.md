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
