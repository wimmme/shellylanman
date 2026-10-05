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
- **UI password: optional, off by default.** Without it anyone who can reach the
  port can use it; a warning is logged at start and shown in the UI. Set one in
  *Settings → Security*:
  - one password, no user name; stored only as a PBKDF2-HMAC-SHA256 hash
    (600 000 iterations, random salt), at least 8 characters with a capital;
  - the UI, `/api/v1` and `/ws` then need a session: a random 256-bit id in an
    `HttpOnly`, `SameSite=Strict` cookie (`Secure` over HTTPS, also behind a proxy
    that sends `X-Forwarded-Proto: https`); `/data/sessions.json` keeps only hashes
    of the ids; a session ends after 30 days without use, on log out, and for every
    browser when the password changes;
  - after five wrong passwords from one address each try waits longer (1 s, 2 s,
    4 s … up to 60 s); wrong tries are logged with the address;
  - programs (the Home Assistant integration) use the MCP token instead; a read-only
    token can only read;
  - under Home Assistant ingress no password is asked: Home Assistant's own login
    applies there;
  - `/healthz` stays open and tells nothing;
  - forgotten: start once with `SHELLYLANMAN_RESET_PASSWORD=1`.
- **Cross-origin protection:** state-changing API requests and the WebSocket are
  refused when the browser's `Origin` does not match the host. Behind a reverse
  proxy with a different public name, list that name in `SHELLYLANMAN_ORIGINS`.
- **Content Security Policy:** the UI loads nothing from other origins; no CDN,
  no telemetry. No inline scripts; inline styles only with a nonce that is new for
  every page load (the script editor's stylesheet carries it).

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

## How this repository is checked

Secret scanning with push protection, Dependabot alerts, security updates and
weekly version updates (Go modules, npm, Docker base images, GitHub Actions),
CodeQL code scanning (Go, TypeScript, Python, workflows), and the checks of
`tools/verify.sh` on every push.
