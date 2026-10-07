# CLAUDE.md

Guidance for Claude Code (and humans) working in this repository.

**ShellyLanMan** is a web application for discovering, monitoring and managing
Shelly devices on the LAN, based on ShellyScanner (Java/Swing, GPL-3.0). Go
server + plain TypeScript frontend, one Docker image, one `/data` volume.

## Where to look

| Question | Where |
|---|---|
| The brief and working rules | `docs/brief.md` |
| How the original works, the new architecture, Java → new mapping | `ARCHITECTURE.md` |
| Every feature, where it is in the Java code, the Shelly API it uses, status | `FEATURE_PARITY.md` |
| Decisions already taken (technology, networking, QR scope, …) | `DECISIONS.md` §8 — it overrides earlier proposals |
| Long-term ideas, not decided (Shelly firmware 2.0 / EU RED, …) | `docs/roadmap.md` |
| Where code came from | `docs/PROVENANCE.md` |
| How we work: checks, simulator and screenshots, releases, the HA test instance | `docs/WORKFLOW.md` |
| Wim's own hosts, accounts and test systems (git-ignored, local only) | `CLAUDE.local.md` |
| ShellyScanner source | https://github.com/usnasoft/shellyscanner (clone it; the code is the truth) |
| Shelly API | the `shelly-api-docs` MCP server, or https://shelly-api-docs.shelly.cloud |

## Commands

Everything runs in Docker; the host needs nothing else.

```sh
sh tools/verify.sh                   # all checks + production image (what CI runs)
docker build --target test .         # checks only
docker compose up -d --build         # run locally on :3082
go run ./cmd/shellysim testdata/gen1/SHPLG-S testdata/gen2/Plus1   # simulated devices
go run ./cmd/record -host <ip> -out testdata/<gen>/<model>        # record a fixture (GET only)
```

No Docker on the dev machine: run the same commands on a Docker host with
`tools/remote.sh` (`REMOTE_DIR=build/sll-dev`; hosts and keys in `CLAUDE.local.md`).
It copies the tree and runs there; generated files must be fetched back explicitly.
Screenshots with simulated devices: `sh tools/screenshots/run.sh`.

## Architecture in one screen

```
web/src (TS SPA) ──REST /api/v1──┐  ┌──WS /ws (server→browser events only)
                                  ▼  ▼
internal/httpapi   thin adapter: routes, JSON, origin check, security headers
internal/hub       WebSocket fan-out; client count drives polling rate (Q8)
internal/service   (from Phase 2) all behaviour lives here — UI, MCP, HA are clients
internal/mcp       MCP server at /mcp (AI assistants): thin layer over the service
internal/store     /data: secret.key, settings.json (AES-256-GCM secrets)
internal/sim       simulated Shelly device from testdata fixtures
internal/fixture   fixture naming + scrubbing (public repo!)
```

## Standing rules (from the brief)

- **The ShellyScanner source code is the functional truth.** Unclear behaviour:
  find the code → trace the call chain → identify the Shelly API call(s) → work
  out why → document → only then implement. Do not assume generations share an
  API model (Gen1 REST, Gen2+ RPC, BLU via gateways).
- **STOP → ANALYSE THE ORIGINAL → DOCUMENT → ASK OR IMPLEMENT.** Do not invent
  features. ShellyScanner parity was the original scope; since 2026-10-07 Wim also
  wants features beyond it (`docs/roadmap.md` §2, the MCP server, the Home Assistant
  app, …). Those still start as Wim's idea or his explicit yes, get an analysis and a
  decision in `DECISIONS.md` and a row in `FEATURE_PARITY.md`, and design questions are
  asked. Do not add features nobody asked for.
- **Ask instead of assume** on design questions. Unclear or buggy-looking
  original behaviour goes into `FEATURE_PARITY.md` §5 and is asked, not silently
  "fixed".
- **Read before write.** Nothing writes to a device until reading is solid.
  **Never write to a real device without asking**, except the agreed test
  devices **Grondwaterpomp** (Shelly Plug S Gen1, 192.168.0.86) and
  **ShellyTestPlug** (Shelly Plug S Gen3, 192.168.0.150, firmware updates allowed)
  in phases that need writes — and even then say what you will do first.
- **Destructive actions** (reboot, restore, firmware update, factory reset)
  always need explicit confirmation in the UI and `confirm: true` in the API.
- **No cloud dependency, no telemetry.** Nothing leaves the LAN except what the
  original does (firmware checks) and the opt-in release check.
- **Tests are not optional.** Every feature has tests; `FEATURE_PARITY.md` rows
  are ticked only with tests. CI never touches real devices. Fixtures are
  recorded with `cmd/record` and reviewed before commit.
- **Dependencies:** Go stdlib first. Each dependency needs a reason and its
  licence in `THIRD_PARTY_NOTICES.md` before it is added. Current: `coder/websocket`,
  `golang.org/x/net`, `golang.org/x/sys`, `skip2/go-qrcode`; frontend: CodeMirror 6 (script editor only), Chart.js + chartjs-plugin-zoom + hammerjs (charts page only).
- **Attribution:** ported code gets the "Portions derived from ShellyScanner"
  header and a `docs/PROVENANCE.md` row, in the same commit.
- **Frontend:** plain TypeScript, no framework. Text via `textContent` only
  (`h()` in `web/src/dom.ts`), never `innerHTML` with device data. No inline
  styles/scripts (CSP); a library that injects a `<style>` needs the per-load nonce
  (`<meta name="csp-nonce">`, as CodeMirror's `EditorView.cspNonce`). A UI change
  that the unit tests cannot see is checked in a browser under the real CSP
  (`tools/screenshots/`, e.g. `check-editor.py`). Every UI string in every catalogue in `web/src/i18n/`
  (en, nl, de, fr, es, it, bg, zh; a test enforces equal keys and placeholders).

## Workflow

- Small commits, one logical change each; update `CHANGELOG.md` in the same
  commit. Commit freely; **never push, tag or create/modify the GitHub repo
  without being asked.** A `vX.Y.Z` tag publishes an image.
- End of each phase: summarise what was done, what was tested (including on
  hardware), what is open — then wait for Wim's go.
- When you need Wim to test on his devices: give exact commands and what to
  look for.
- Deploy with `docker compose up -d`, not `docker restart` (restart keeps the
  old image).
