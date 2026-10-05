# Phase 15 — optional UI password

Status: decided (DECISIONS §24, P15-1..7); being built.

## 1. Background

ShellyLanMan's UI and API have no login: anyone who can reach the port can use
them. That was a decision from the start (DECISIONS Q25: **one optional admin
password, off by default, with a warning**), recorded in `SECURITY.md` as
"planned"; the Settings page was meant to get a *Security* tab for it (DECISIONS
§3, UI mapping). ShellyScanner has no counterpart: it is a desktop program.

What exists today:

- A warning in the log at start and a banner in the UI (dismissable per browser,
  F5) when no password is set; hidden under Home Assistant ingress.
- `status.authEnabled` in `GET /api/v1/status`, always `false`.
- Cross-origin protection (state-changing requests and `/ws` refuse a foreign
  `Origin`), CSP, security headers.
- MCP at `/mcp` with its own bearer token (off by default, access levels); the
  Home Assistant app's token-less MCP listener on loopback only.
- The Home Assistant integration reads `/api/v1/…` without credentials, and with
  the MCP token for the credentials endpoint and Assist.

Wim's request (2026-10-05): a login page, opt-in in Settings, default off; one
profile — no user name, only a password. Mainly for a standalone ShellyLanMan
(Docker); inside Home Assistant the sidebar is already behind Home Assistant's
login.

## 2. What a password protects

| Path | With a password set |
|---|---|
| The UI (`/`, static files) | the login page until logged in |
| `/api/v1/…` | 401 without a session (the UI shows the login page) |
| `/ws` | refused without a session |
| `/healthz` | open (Docker health check; says nothing about the version) |
| `/mcp` | unchanged: its own bearer token |
| Home Assistant ingress (sidebar) | no ShellyLanMan login: Home Assistant's login already applies (proposal, Q1) |
| The app's loopback listeners | unchanged (only Home Assistant on the same host reaches them) |
| `/api/v1/…` with a valid MCP token | allowed (proposal, Q2) — so the integration keeps working against a protected standalone ShellyLanMan |

## 3. Design

- **Storing the password:** only a hash — PBKDF2-HMAC-SHA256 from Go's standard
  library (`crypto/pbkdf2`, Go 1.24+), random 16-byte salt, 600 000 iterations
  (OWASP 2023+). No new dependency (the planned `golang.org/x/crypto` argon2id is
  not needed). Kept in `settings.json` like the other secrets; never sent to the UI.
- **Session:** after login a random 32-byte session id in a cookie
  `slm_session` — `HttpOnly`, `SameSite=Strict`, `Path=/`, `Secure` when the
  request came over HTTPS (also behind a reverse proxy: `X-Forwarded-Proto` from a
  trusted proxy). Sessions live in memory and in `/data/sessions.json` (hashed ids),
  so a restart does not log everyone out.
- **Lifetime:** see Q3.
- **Login page:** its own small page (no app shell): password field, *Log in*,
  the error "Wrong password", in the browser's language. Works under the CSP.
- **Log out:** in the sidebar / ☰ menu when a password is set.
- **Settings → Security:** *Protect ShellyLanMan with a password* — set (twice),
  change (current + new twice), switch off (current password). Changing or
  switching off ends all other sessions. Minimum length: Q6.
- **Brute force:** wrong passwords per client address are slowed down (after 5:
  1 s, 2 s, 4 s … up to 60 s) and logged as a warning; a right password resets it.
- **Forgotten password:** Q4.
- **The banner and the log warning** disappear when a password is set.
- **API clients and MCP:** unchanged except Q2.
- **`SECURITY.md`:** "planned" becomes how it works; still: do not expose it to the
  internet without a reverse proxy with TLS.

## 4. Tests

Unit: hashing and verification, cookie flags, session expiry, rate limiting, each
path of §2 with and without a session/token/ingress, password change ends other
sessions. Browser (`tools/screenshots/check-login.py`): login page under the CSP,
wrong password, login, log out, Settings → Security. Then on the HA test instance:
the sidebar without a second login, and the integration with the MCP token.

## 5. Questions

1. **Home Assistant ingress:** no ShellyLanMan login in the sidebar (Home
   Assistant's login is enough), only on the app's LAN port?
2. **The integration against a protected ShellyLanMan:** accept the MCP token on
   `/api/v1/…` (so the integration needs the MCP token as soon as a password is
   set), or a separate API token?
3. **Session lifetime:** 30 days since the last use (a wall tablet stays logged in),
   or until the browser closes, or a choice ("Stay logged in")?
4. **Forgotten password:** a command in the container
   (`docker exec shellylanman shellylanman reset-password`), or an environment
   variable for one start (`SHELLYLANMAN_RESET_PASSWORD=1`)?
5. **Brute-force slowdown** as in §3 — fine?
6. **Minimum length** 8 characters, no other rules?

## 6. Plan

| Step | What | Size |
|---|---|---|
| 15.1 | Password hash + sessions in the store/service, rate limit; tests | M |
| 15.2 | Middleware for §2, login/logout API, status `authEnabled`; tests | M |
| 15.3 | Login page, log out, Settings → Security (8 languages); browser test | M |
| 15.4 | Reset (Q4), `SECURITY.md`, README, FEATURE_PARITY (new), CHANGELOG | S |
| 15.5 | HA test instance: sidebar and integration; release on Wim's go | S |

## 7. Status (2026-10-05)

15.1–15.4 built and tested: `internal/auth` (unit tests), `httpapi/login.go`
(every path of §2 with and without session, token and ingress; slowdown; reset),
the login page and Settings → Security in the browser under the CSP
(`tools/screenshots/check-login.py`, all ok). One difference from §3: the login is
drawn by the app itself (the page and its files are open, the API is not), which
keeps it under the same CSP and in the browser's language.

Open (15.5): the Home Assistant integration sends the MCP token only to `/mcp` and
for credentials, not on its other API calls; with a password set it needs the
token on every call — see the question to Wim about the app's own integration.
