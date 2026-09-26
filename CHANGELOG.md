# Changelog

All notable changes. Format based on [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added — Phase 2: discovery
- Device discovery: full mDNS scan (all interfaces, container bridges skipped), local
  mDNS scan on a chosen interface, IP scan of up to 10 ranges, offline mode (archive
  only). mDNS is a small browse-only implementation on golang.org/x/net, verified on
  a real network (43 instances, 25 Shellies of all generations).
- Identification of every ShellyScanner 1.3.4 model (27 Gen1, 33 Gen2, 28 Gen3, 15
  Gen4, BLU TRV and BTHome devices); unknown models as "Generic Gn", unreachable
  shelly* hosts as "Generic" with the error.
- Follow-up discovery: devices behind a range extender (ip:port) and BLU devices
  through Pro, Gen3 and Gen4 gateways, with ShellyScanner's rules for devices seen by
  several gateways.
- Status per device (on line, off line, not logged, updating, error, archived,
  reboot required), refreshed every 2 s (configuration every 5th time) while a
  browser is open and once a minute otherwise.
- Protected devices: Gen1 Basic and Gen2+ SHA-256 Digest authentication; a Login
  dialog and default credentials, stored encrypted, per device or global.
- Device archive (`/data/archive.json`): archived devices shown when not found,
  auto reload after 45 s, notes and keywords kept, remove from archive, clear.
- Devices page with live table, summary cards, Refresh and Rescan; Settings →
  Network and Archive; banner when no device answers mDNS (bridge networking).
- API: `/api/v1/devices`, `/scan`, `/credentials`, `/network/interfaces`, `/archive`.

### Added — Phase 1: skeleton
- Go server (`cmd/shellylanman`) on port 3082: `/healthz`, REST API under
  `/api/v1` (`about`, `status`, `settings`), WebSocket `/ws` with origin check,
  embedded frontend, security headers (CSP), graceful shutdown, `-healthcheck`
  for Docker.
- `/data` store: `secret.key` generated on first start, `settings.json` with
  AES-256-GCM-encrypted secrets, atomic writes.
- Themed web shell in TypeScript: sidebar (Devices, Checklist, Charts, Firmware,
  Deferred, Settings, About), connection indicator, first-run dialog, "UI
  authentication is off" banner, English and Dutch, per-browser appearance
  (17 palettes, dark/light, contrast and brightness, 7 fonts, font size) adapted
  from MikroDash.
- Simulated Shelly device (`cmd/shellysim`, `internal/sim`) serving recorded
  fixtures; `cmd/record` to record and scrub fixtures from real devices; first
  fixtures: Plug S (Gen1) and Plus 1 (Gen2).
- Multi-stage Dockerfile (Alpine runtime, non-root, amd64 + arm64), minimal
  `docker-compose.yml` with host networking, `tools/verify.sh`, GitHub Actions
  (checks on every push/PR, image publishing on version tags).
- Phase 0 analysis: `ARCHITECTURE.md`, `FEATURE_PARITY.md`, `DECISIONS.md`.
