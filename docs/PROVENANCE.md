# Provenance

Where each part of ShellyLanMan comes from, for honest attribution
(`DECISIONS.md` §3.2). Categories: **1** copied from ShellyScanner · **2** ported
(logic translated from the Java code — derivative work) · **3** clean
re-implementation from the Shelly API documentation and observed behaviour ·
**4** new. Files in category 2 carry a "Portions derived from ShellyScanner"
header. Update this table in the same commit as the code.

| Path | Category | Source / notes |
|---|---|---|
| `cmd/shellylanman` | 4 | — |
| `cmd/record` | 4 | request list mirrors ShellyScanner's info requests (`AbstractG1Device`/`AbstractG2Device.getInfoRequests`) — a list of public API calls, not code |
| `cmd/shellysim`, `internal/sim` | 4 | — |
| `internal/fixture` | 4 | — |
| `internal/store` | 4 | — |
| `internal/hub` | 4 | — |
| `internal/httpapi` | 4 | — |
| `internal/version`, `internal/web` | 4 | — |
| `web/src/appearance.ts` | adapted from MikroDash (MIT) | factor tables, brightness formula, neutral-level rule |
| `web/public/app.css` (tokens block) | from MikroDash (MIT) | palettes and design tokens |
| `web/public/app.css` (components), other `web/src/*` | 4 | — |
| `web/public/fonts/*` | third party (OFL-1.1) | via MikroDash |
| `web/src/pages/devices.ts` column headings | terminology from ShellyScanner (`LabelsBundle.properties`) | short UI terms |

No ShellyScanner code has been copied or ported yet (Phase 1 is infrastructure).
