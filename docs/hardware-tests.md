

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
