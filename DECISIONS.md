

## 16. Decisions taken during Phases 9 and 10 (2026-09-28)

| # | Decision | Why |
|---|---|---|
| P9-1 | A device that could not be read when discovered is retried every 2 minutes (after the first retry at 30 s) | O33, extension agreed by Wim |
| P9-2 | `GET /api/v1/devices?gen=` replaces `-gen` | L5, Q14 |
| P10-1 | Release check: GitHub releases of ShellyLanMan, setting never (default) / stable / all, once a day and after a settings change, banner with release notes link and "skip this version"; a `dev` build is never told to update | A8, Q18 |
| P10-2 | Images for `linux/amd64` and `linux/arm64` on GHCR from `vX.Y.Z` tags (workflow of Phase 1): tags `X.Y.Z`, `X.Y`, `latest`; the arm64 binary is cross-compiled (checked), the final Alpine stage runs under QEMU on GitHub | Brief phase 10 |
| P10-3 | Upgrade instructions in the README: `docker compose pull && up -d`, pinning a release line, saving the data volume before a major version | Brief phase 10 |
