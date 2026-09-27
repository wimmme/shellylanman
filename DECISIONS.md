

## 15. Decisions taken during Phase 8 (2026-09-27)

| # | Decision | Why |
|---|---|---|
| P8-1 | Notes and keyword: only with the archive in use (as the original's Notes action); saved with the archive (every minute and at shutdown, not only when the program exits) | NotesEditor, P2 archive rules |
| P8-2 | CSV exports (table and charts) quote a value that contains the separator, a quote or a line break (O29); separator per browser | Device names can contain commas |
| P8-3 | Scripts dialog: Scripts and KVS tabs as in the original; the script editor is CodeMirror 6 (planned in §1.4), in its own bundle loaded only when the editor opens; editor settings (tab size, font size, indent, auto-close, dark) per browser; "Open"/"Save" use the browser's file picker and download | The original's own Swing editor cannot be ported; the browser has no file system |
| P8-4 | The scheduler runs the device calls from the browser through one server endpoint `POST /api/v1/devices/{id}/rpc` (Gen2+; for a BLU TRV through its gateway as `BluTrv.Call`) — it is also the original's "test method" button; the apply logic of G2SchedulerPanel / WDThermSchedulerPanel / TRVSchedulerDialog is ported in the browser | Same structure as the original (the dialogs call the schedule managers directly) |
| P8-5 | Charts: Chart.js with the zoom plugin (MIT) in their own bundle; readings kept on the server per device for 24 h (at most 20 000 per device) in memory, not on disk; the EM chart reads EMData / EM1Data once a minute like the original | Q7, Q8 |
| P8-6 | Keyboard shortcuts kept where the browser allows them: filter (Ctrl+F/E/S), editor, notes (Ctrl+S, Ctrl+K), charts (Ctrl+R/P/C); menu mnemonics and window focus shortcuts are not ported | A11 |
| P8-7 | Help: a short help per function on the About page, plus the "?" help in the scheduler, the script editor and the charts; the online manuals of usna.it are linked from About | Q22 |
| P8-8 | The `-graphs` stream (L6) is covered by `GET /api/v1/samples` and the `device.upsert` WebSocket events | Q14 |
