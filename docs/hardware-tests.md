

## Phase 8 — advanced functions (2026-09-27)

Test container on dockerhostvm (port 3099). Only reads on the devices; no scheduler,
script or KVS change was applied.

| Check | Result |
|---|---|
| Scripts dialog on LedKeukenOnderSchakelaar (i4 G3) and Deurbel (+UNI) | script list (aioshelly_ble_integration, running), code read (Script.GetCode), KVS tab empty |
| Script editor (i4 G3, before the log fix) | toolbar, code, caret position; the output pane could not connect because the device's websocket debug log is off — opening the editor now switches it on like the original (O30), so it was not opened again on a non-test device |
| Scheduler on LampenHallBovenSwitch (Plus1) | no jobs → one empty job line with the cron editor, calls, hints menu; closed without applying |
| Charts, notes, CSV export, print | UI checked in the browser; charts start with the server's history |

Not tested on hardware: writing schedules, scripts or KVS (no Gen2+ test device for
writes — simulator tests cover the calls), Wall Display and BLU TRV schedulers (no such
devices here), the EM chart on a real Pro 3EM over a long period.
