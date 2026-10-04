# BLU devices behind `BLE.CloudRelay`: where to get name and model (2026-10-04)

Question from Antonio Flaccomio (ShellyScanner): can BLU devices that are not set up
as BTHome devices on a gateway be supported through `BLE.CloudRelay.ListInfos`? He
never found a way to get their "name" and "model".

## What the API says

`BLE.CloudRelay.ListInfos` (docs/ComponentsAndServices/BLE.mdx) returns per relayed
device `name`, `model`, `sdata`, `mdata`, `last_seen`, and documents:

- `name` — "Device name from the advertisement taken from localname. **Will be
  populated only during active scans**"
- `model` — "Internal model id of the device. **Will be populated only during active
  scans**, otherwise will show `0` which is invalid model id"

The local name and the Shelly manufacturer data (MFID 0x0BA9, block `0x0B` = model
id, docs-ble/common.md) are in the **scan response**, which a passive scan never
requests. The Cloud Relay scans passively, so `name` stays `null` and `model` `0`.

Since firmware 1.5.0 the *Enhanced Scan Manager* merges the scan requests of all
clients (Cloud Relay, BTHome, scripts) and applies "the most aggressive
combination" — so while **any** client runs an active scan, the shared scan is
active and the Cloud Relay sees scan responses too.

## Measured on Wim's LAN (read only, 2026-10-04)

`GET /rpc/BLE.CloudRelay.ListInfos` on 13 Gen2+/Pro/Gen3 devices: two gateways
(a Plus RGBW PM and a Dimmer G3) relay the same device:

```json
{"7c:c6:b6:a5:c9:3d": {"name": null, "model": 0,
  "sdata": {"fcd2": "RAC8AWQ6ADoAOgA6AQ=="}, "mdata": {}, "last_seen": 1791147133}}
```

`fcd2` is the BTHome service UUID: such a "cloud" BLU device still advertises in
BTHome format, it is only not added as a BTHome device on the gateway. Decoded:
`44` (BTHome v2, not encrypted, trigger based) · packet id 188 · battery 100 % ·
four button objects (`0x3A`), the fourth pressed → a 4-button BLU (Wall Switch 4 or
RC Button 4).

## Ways to get name and model

1. **Active scan for a moment** (exact name and model): `BTHome.StartDeviceDiscovery`
   (Gen2 Pro, Gen3, Gen4) starts a 30 s *active* scan without changing the
   configuration. It emits `device_discovered` events with `local_name` and
   `shelly_mfdata.model_id` (RPC notifications, so a WebSocket is needed), and —
   through the merged scan — `ListInfos` should show `name` and `model` while it runs.
   A script with `BLE.Scanner.Start({active: true})` would do the same, but needs a
   script on the device.
   **Tried on a Dimmer G3 (Wim's go, 2026-10-04):** `StartDeviceDiscovery` with 30 s
   answered `null`, `ListInfos` stayed `name: null, model: 0` throughout, and
   `discovery_done` reported `device_count: 0`. The reason: the BLU device did not
   advertise during those 30 s (`last_seen` unchanged) — a BLU button only sends when
   pressed (and its 6-hourly packet); ten seconds after the scan a press arrived. So
   an active scan finds a button only if it is pressed while the scan runs.
   **Second try, 60 s, Wim pressing the buttons:** five presses arrived during the
   active scan (packet ids 0xD3…0xE3, different buttons in the BTHome data), yet
   `ListInfos` kept `name: null, model: 0, mdata: {}` and the BTHome discovery
   reported `device_count: 0`. So the button-press advertisements of this 4-button
   BLU are **not scannable**: there is no scan response to ask for, and a normal
   press does not reveal name or model even to an active scan. The scan response
   (local name, Shelly manufacturer data with the model id, the "discoverable" flag)
   is only there while the device is discoverable — in pairing mode (button held
   > 10 s, docs-ble/common.md "Pairing") and, for some models, shortly after power-on.

**Conclusion for battery BLU devices behind the Cloud Relay:** name and model can be
read with an active scan only while the device is in pairing mode (ask the user to
hold its button during a short `StartDeviceDiscovery`); otherwise use the passive
ways below — the model id from BTHome object `0xF0` (power-on and every 6 h), or an
estimate from the BTHome objects. The name from the advertisement is the model's
short name anyway (e.g. `SBBT-004CEU`); a user-given name exists only in the Shelly
Cloud, which ShellyLanMan does not use.
2. **Passively, from the BTHome data in `sdata`**: BLU devices send object `0xF0`
   (device type id = the same model id) and `0xF1` (firmware) at power-on and every
   6 hours. `ListInfos` keeps only the last advertisement, so it shows up only now and
   then — but for devices that advertise rarely (buttons) that packet is often the
   last one. Cache it when seen.
3. **Passively, an estimate from the BTHome objects** of any packet: four `0x3A`
   buttons → Wall Switch 4 / RC Button 4; temperature + humidity → H&T; window +
   illuminance + rotation → Door/Window; motion + illuminance → Motion; one button →
   Button 1. Not exact (models with the same objects), but always available.

Model ids (docs-ble/common.md): 0x0001 BLU Button1 SBBT-002C · 0x0002 DoorWindow
SBDW-002C · 0x0003 HT SBHT-003C · 0x0005 Motion SBMO-003Z · 0x0006 Wall Switch 4
SBBT-004CEU · 0x0007 RC Button 4 SBBT-004CUS · 0x0008 TRV SBTR-001AEU · 0x0009 Remote
SBRC-005B · 0x000B Weather Station SBWS-90CM · 0x000C H&T Display ZB SBHT-103C.

A sensible combination: 3 always, 2 when seen (cached), 1 on the user's request
("identify BLU devices" button), which also gives the name.
