

## 16. Decisions taken during Phases 9 and 10 (2026-09-28)

| # | Decision | Why |
|---|---|---|
| P9-1 | A device that could not be read when discovered is retried every 2 minutes (after the first retry at 30 s) | O33, extension agreed by Wim |
| P9-2 | `GET /api/v1/devices?gen=` replaces `-gen` | L5, Q14 |
| P10-1 | Release check: GitHub releases of ShellyLanMan, setting never (default) / stable / all, once a day and after a settings change, banner with release notes link and "skip this version"; a `dev` build is never told to update | A8, Q18 |
| P10-2 | Images for `linux/amd64` and `linux/arm64` on GHCR from `vX.Y.Z` tags (workflow of Phase 1): tags `X.Y.Z`, `X.Y`, `latest`; the arm64 binary is cross-compiled (checked), the final Alpine stage runs under QEMU on GitHub | Brief phase 10 |
| P10-3 | Upgrade instructions in the README: `docker compose pull && up -d`, pinning a release line, saving the data volume before a major version | Brief phase 10 |

## 17. Decisions after v0.1.0 (2026-09-28, Wim's feedback)

| # | Decision | Why |
|---|---|---|
| F1 | The devices table is updated in place (`dom.patch`): unchanged rows and cells stay; row mouse handling is delegated and reads the rows as last drawn | Hover flicker and lost checkbox clicks during refreshes |
| F2 | Web server port is a setting (Settings → General, `PUT /api/v1/server` with `confirm`); the server opens the new port, saves it, and closes the old one after 3 s. `SHELLYLANMAN_LISTEN` wins and locks it; the health check reads the saved port | Wim asked; not in ShellyScanner (a desktop program has no port) |
| F3 | UI languages: en, nl, de, fr, es, it, bg, zh. ShellyScanner has only English labels (its Italian bundle holds URLs only), so the translations are our own | Wim asked; extends Q21 |
| F4 | About page: description, version and release-check state, uptime, system information, support links (Buy me a coffee, PayPal — plain links, no external scripts or images, CSP), release notes from the embedded CHANGELOG, dependencies (Go build info + `deps.json` from package-lock), licence and notices from the binary. ShellyScanner and MikroDash under Based on and Credits, both "deserve a coffee", no donation link for them | Wim asked, MikroDash as example without copying it |
| F5 | The no-auth banner is dismissed per browser (localStorage) | Wim asked |
| F6 | New logo (Wim's): transparent icon for sidebar/favicon/About, the full logo in the README | Wim asked |
