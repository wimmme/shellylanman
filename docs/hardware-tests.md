

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

