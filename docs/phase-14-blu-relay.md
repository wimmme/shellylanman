# Phase 14 — BLU devices behind the Cloud Relay, and "Identify BLU devices" (analysis, 2026-10-05)

Wim's request (2026-10-04, after Antonio's question): support BLU devices that a
gateway only relays to the Shelly Cloud (not set up as BTHome devices on it), and add
a button with a wizard that identifies them. Wim agreed with the proposals: those
devices get rows of their own in the device table; the button sits on those rows
and in the checklist's Bluetooth dialog. This is new behaviour beyond ShellyScanner
(which lists these devices only in its "BLE devices" dialog, without name or model).

Background and measurements: `docs/blu-cloud-relay.md`.

## 1. What exists

- `internal/service/blu.go`: on Pro and Gen3+ mains-powered gateways that are online,
  `discoverBLU` reads `Shelly.GetComponents?dynamic_only=true` and makes rows for
  `blutrv:*` (gen `blu`) and `bthomedevice:*` (gen `bth`), with the model from the
  component's `attrs.model_id` (`model.BLUTypeName`), sensors from
  `BTHomeDevice.GetKnownObjects`, readings through `Shelly.GetComponents?keys=…`.
  A device seen by several gateways gets one row with alternative parents.
- The checklist's Bluetooth cell reads `BLE.CloudRelay.ListInfos` (count; dialog with
  name and MAC — the name is always empty for relayed devices).
- Relayed devices (Wim's RC Button 4 on two gateways) have **no row**.

## 2. What the gateways tell about a relayed device

`BLE.CloudRelay.ListInfos` (paged): MAC, `last_seen`, the last advertisement's
service data (`sdata`, BTHome under `fcd2`) and manufacturer data (`mdata`);
`name` and `model` stay empty (measured, firmware 2.0.1). From the BTHome data:
battery, button events, temperature, humidity, illuminance, motion, window,
rotation — whatever the device sends — and now and then the model id (object
`0xF0`, at power-on and every 6 hours). Name and model for sure: the
`device_discovered` event of `BTHome.StartDeviceDiscovery` while the device is in
pairing mode (measured: `SBBT-004CUS`, model id 7).

## 3. Design

### 3.1 Rows for relayed BLU devices (server)

- During `discoverBLU` and with each configuration refresh of a gateway (every few
  status refreshes, not every 2 s), read `BLE.CloudRelay.ListInfos` (all pages).
- Each MAC that has no BTHome/TRV row becomes a row: gen `bth`, marked *relayed*,
  parent = the gateway that heard it last (others as alternative parents, as today),
  `lastSeen` from `last_seen`, status **online** once heard (as other BLU rows: the
  gateway heard it), offline when no gateway lists it any more.
- Readings from the BTHome data (a small decoder for the BTHome v2 objects Shelly BLU
  devices use): battery, temperature, humidity, illuminance, motion, window,
  rotation; button events shown as "last: button 4 long press".
- Model: the identified model (3.3) if known; else the model id from object `0xF0`
  when it was seen (kept); else an estimate from the objects ("BLU, 4 buttons — RC
  Button 4 or Wall Switch 4"). Host name like other BTHome rows (`B…-<mac>`); the
  name is the user's note/keyword field, since the device has no name of its own
  outside the Shelly Cloud.
- Read only: no control, no settings, no backup for these rows (there is nothing on
  the device to read or write through the gateway). Info shows the gateway's
  `ListInfos` entry and the decoded data; Notes work.
- Archived like other devices (they come back as "archived" after a restart until a
  gateway hears them).

### 3.2 The wizard "Identify BLU devices"

1. **Start** — from a relayed BLU row (button *Identify*) or from the checklist's
   Bluetooth dialog. The wizard picks the gateway: one that can run a BTHome
   discovery (Gen2 Pro, Gen3, Gen4) and heard the device best; the user can change it.
2. **Pairing mode** — instructions for the device: hold its button for more than 10 s
   until the LED flashes blue; for the RC Button 4 and the Wall Switch 4 (estimate:
   four buttons) hold **two** buttons. A 90 s countdown runs while the gateway scans
   actively (`BTHome.StartDeviceDiscovery`, nothing is added on the gateway).
3. **Live result** — every device that answers appears at once: model, short name,
   MAC, signal. A device in the table is marked "identified".
4. **Done** — the model is stored in ShellyLanMan's archive (nothing on the device);
   the row shows the exact model from then on. The device leaves pairing mode by
   itself.

Server side: `POST /api/v1/blu/identify {gateway, duration}` starts it; ShellyLanMan
opens the gateway's RPC WebSocket (`ws://<gw>/rpc`, authentication as for RPC POSTs
on a protected gateway), calls `BTHome.StartDeviceDiscovery`, and passes each
`device_discovered` and the final `discovery_done` to the browsers as events
(`blu.discovered`, `blu.identify.done`). One identification at a time.
The MCP server and the Home Assistant integration can read the new rows like any
device (read only); the wizard itself stays in the UI.

### 3.3 Tests

- Decoder: BTHome v2 packets (the measured RC Button 4 packets, an H&T, a Door/Window,
  a Motion, the `0xF0`/`0xF1` info packet, unknown objects).
- Service with the simulator: a gateway fixture with `BLE.CloudRelay.ListInfos`
  (the measured answer) → one relayed row; a second gateway with the same MAC →
  one row with an alternative parent; a BTHome component for the same MAC → the
  BTHome row wins.
- Wizard: the simulator's RPC WebSocket sends `device_discovered` after
  `BTHome.StartDeviceDiscovery` (the sim already supports notifications) → events
  reach the hub, the model is stored and the row updated.
- Browser (`tools/screenshots/`): the wizard opens, counts down, shows a discovered
  device.
- On hardware with Wim: his RC Button 4 through LampKeukenTafel (as on 2026-10-04).

## 4. Questions

1. **Online** for a relayed BLU: "heard by a gateway" (as other BLU rows), with the
   last-heard time in the tooltip — or offline after some hours without a packet
   (buttons can stay silent for days)? Proposal: as other BLU rows.
2. **Name**: relayed devices have no name of their own outside the Shelly Cloud.
   Use the note/keyword as the name in the table, or add a small "name" field to the
   archive for BLU rows? Proposal: a name field in the archive (shown as the name),
   editable in Notes.
3. **Scope now or later**: rows (3.1) and the wizard (3.2) together in one release
   before going public, or rows first?

## 5. Plan

| Step | What | Size |
|---|---|---|
| 14.1 | BTHome v2 decoder (`internal/parse`), tests | S |
| 14.2 | Relayed rows: ListInfos on gateways, merge with BTHome/TRV rows, archive, model (identified / `0xF0` / estimate) | M |
| 14.3 | UI: rows, Info for relayed rows, tooltips; MCP/HA unchanged (they read devices) | S |
| 14.4 | Gateway RPC WebSocket + `POST /api/v1/blu/identify` + events | M |
| 14.5 | Wizard UI (8 languages) | M |
| 14.6 | Simulator fixtures, tests, browser test; hardware test with Wim | M |
| 14.7 | FEATURE_PARITY (new, beyond ShellyScanner), DECISIONS P14-x, CHANGELOG, release | S |

## 6. Status

14.1–14.7 done in code, tests, simulator and hardware (2026-10-05, see
`docs/hardware-tests.md`): unit tests (decoder, relayed rows, identify with authentication), `tools/screenshots/check-blu.py` (browser, all ok).
Open: the release.
