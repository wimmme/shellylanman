# Security policy

## Reporting a vulnerability

Please do not open a public issue for security problems. Use GitHub's private
vulnerability reporting on https://github.com/wimmme/shellylanman/security.
Include what is affected, how to reproduce it, and the impact you see.

Only the latest release receives fixes.

## How ShellyLanMan is meant to be run

ShellyLanMan is a LAN tool. It talks to your Shelly devices over plain HTTP
(that is how Shelly devices work locally) and can change their configuration.

- **Do not expose it to the internet.** If you need remote access, put it behind
  a reverse proxy with TLS **and** authentication.
- **UI authentication is off by default.** Anyone who can reach the port can use
  it. A warning is logged at start and shown in the UI. (An optional admin
  password is planned.)
- **Cross-origin protection:** state-changing API requests and the WebSocket are
  refused when the browser's `Origin` does not match the host. Behind a reverse
  proxy with a different public name, list that name in `SHELLYLANMAN_ORIGINS`.
- **Content Security Policy:** the UI loads nothing from other origins; no CDN,
  no telemetry.

## Secrets at rest

`/data/secret.key` (32 random bytes, created on first start) encrypts secret
values in `/data/settings.json` with AES-256-GCM, each bound to its name.

Be clear about what that does: the key lives in the same volume. It protects a
copy of `settings.json` on its own — pasted into an issue, or in a partial
backup — not someone who has the whole `/data` volume or root on the host.
Protect the volume like you protect your Wi-Fi password.

Device backups (`/data/backups`, from Phase 6) contain device configuration
such as Wi-Fi SSIDs, MQTT settings, scripts and KVS values, exactly as
ShellyScanner's backups do.

## Outbound connections

None in normal operation. Planned, and only when you use the feature: Shelly's
firmware index and firmware files (firmware page), and an opt-in check for new
ShellyLanMan releases on GitHub.
