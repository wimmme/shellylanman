# Working on ShellyLanMan — standard procedures

How things are done in this project, so they need not be worked out again. The
machine-specific part (host names, addresses, accounts of the maintainer's own test
systems) is not in this public repository: Claude Code reads it from
`CLAUDE.local.md` (git-ignored) in the maintainer's checkout.

## 1. Two repositories

| Repository | Holds | Release |
|---|---|---|
| `wimmme/shellylanman` | the application (Go server, TypeScript UI, MCP server) | tag `vX.Y.Z` → image `ghcr.io/wimmme/shellylanman:X.Y.Z`, `X.Y`, `latest` and a GitHub release with the CHANGELOG section |
| `wimmme/shellylanman-ha` | Home Assistant app (`shellylanman/`) and integration (`custom_components/shellylanman`, HACS) | tag `vX.Y.Z` → image `ghcr.io/wimmme/shellylanman-ha:X.Y.Z`; HACS takes the integration from the repository |

Both carry the same version number. How they depend on each other:
`shellylanman-ha/COMPATIBILITY.md`.

## 2. Checks

Everything runs in Docker; the development machine needs nothing else.

```sh
sh tools/verify.sh                  # everything CI runs, plus the production image
docker build --target test .        # checks only (typecheck, web tests, gofmt, vet, go test -race)
```

Without Docker on the development machine, run them on a Docker host over SSH:

```sh
REMOTE=user@dockerhost SSH_OPTS="-i ~/.ssh/key" REMOTE_DIR=build/sll-dev sh tools/remote.sh sh tools/verify.sh
```

`tools/remote.sh` copies the working tree (without `.git`) to `REMOTE_DIR` on the host
and runs the command there; nothing comes back by itself (`scp` what you need). Never
run `docker compose up` in that directory: its compose file uses the production
container name and would replace a running installation.

Integration tests of `shellylanman-ha` (Home Assistant's test harness, in Docker):

```sh
sh tools/test.sh               # the oldest supported Home Assistant (pinned)
sh tools/test.sh 0.13.368      # another pytest-homeassistant-custom-component version
```

## 3. Simulated devices and screenshots

`cmd/shellysim` plays recorded devices from `testdata/`. For a whole installation with
named devices and the README screenshots (dark and light):

```sh
sh tools/screenshots/run.sh    # on a Docker host; PNGs in ./screenshots-out
```

It builds the tree, starts ShellyLanMan on port 3199 with an IP scan of
`127.0.0.2–14`, eleven simulators on those loopback addresses (each with its own MAC
and a name, `tools/screenshots/sims.sh`) and takes the pictures with Playwright
(`tools/screenshots/shoot.py`). It touches no real device and removes its containers.
All in one look, the Home Assistant palette in dark mode: Devices (wide, with a
dimmer's slider), Checklist, Firmware, Charts, script editor, device info, logs
(the "Porch light" simulator plays `tools/screenshots/log/_log.jsonl`), About, and a
phone with its menu, and the Identify BLU devices wizard. Copy the PNGs to `docs/images/` and check them before committing.

`SCRIPT=<file> sh tools/screenshots/run.sh` runs another script from that directory
against the same setup. `check-editor.py` is such a browser test of the script
editor (scripts in `tools/screenshots/script/`, served by the "Heat pump"
simulator): styled under the CSP, following the theme, a slow device (4 s) and a
failing read (HTTP 500). Run it after changes to the CSP, the editor or its
libraries; unit tests do not render under the real CSP. The same goes for
`check-blu.py` (relayed BLU rows and the Identify wizard, "Living room" relays a
BLU device, `tools/screenshots/blu/`) and `check-nav.py` (full and minimal sidebar) and `check-login.py` (the UI password:
Settings → Security, login, log out, switching off) and `check-log.py` (the Log page: live lines, level filter, search, pause, clear, copy) and `check-apwizard.py` (the firmware wizard through the access point).

Slow or failing devices: an optional `_behaviour.json` in a fixture directory
delays or fails GET requests by URI, e.g.
`{"/rpc/Script.GetCode?id=2": {"delayMs": 4000}, "/rpc/Script.GetCode?id=3": {"status": 500}}`.

The OpenAPI description (`GET /api/v1/openapi.json`) is checked by `go test` against the
routes. To look at it or to run an external validator over it:

```sh
OPENAPI_OUT=openapi.json go test -run DumpOpenAPI ./internal/httpapi
pip install openapi-spec-validator && python -c "import json; from openapi_spec_validator import validate; validate(json.load(open('openapi.json')))"
```

## 4. Translations

Every UI string is in all eight catalogues `web/src/i18n/*.json` (a test checks equal
keys and placeholders). The files are `json.dumps(…, ensure_ascii=False, indent=2)`
with a final newline, so a small script that loads, changes and dumps them keeps the
format; insert new keys next to related ones.

## 5. Releasing

Only on the maintainer's explicit go. Order: ShellyLanMan first (its image is the
base of the app), then `shellylanman-ha`.

**ShellyLanMan**

1. `CHANGELOG.md`: turn `[Unreleased]` into `## [X.Y.Z] - YYYY-MM-DD` with a short
   summary paragraph above the sections; leave an empty `[Unreleased]` above it.
2. `README.md`: the release-line example `ghcr.io/wimmme/shellylanman:X.Y`.
3. `sh tools/verify.sh` (also checks that the release notes can be extracted).
4. Commit `release: vX.Y.Z`, tag `vX.Y.Z`, push `main`, then the tag.
5. Wait for the *Publish image* and *Test* workflows (`gh run watch`); the publish
   workflow creates the GitHub release from the CHANGELOG section.

**shellylanman-ha**

1. `shellylanman/config.yaml` `version`, `shellylanman/Dockerfile`
   `SHELLYLANMAN_VERSION`, `custom_components/shellylanman/manifest.json` `version`.
2. `shellylanman/CHANGELOG.md` (app) and `CHANGELOG.md` (integration): new section.
3. `sh tools/test.sh` on both Home Assistant versions.
4. Commit `release: vX.Y.Z …`, tag, push `main` and the tag; wait for the *App* and
   *Integration* workflows (on `main` and on the tag). The Integration workflow makes
   the GitHub release of the tag after its checks (HACS needs releases), with notes
   from both changelogs (`tools/release-notes.sh`).

**Afterwards:** install it on the Home Assistant test instance (§6) and record what
was checked in `docs/hardware-tests.md` (append, never rewrite).

**Release notes — one shape everywhere** (they are read in Home Assistant's app
store, on GitHub and in the About page):

- `CHANGELOG.md` (ShellyLanMan): `## [X.Y.Z] - date`, one or two sentences that say
  what the release is about, then *Added* / *Changed* / *Fixed* sections with bullets
  (Keep a Changelog).
- `shellylanman/CHANGELOG.md` (the app, shown by Home Assistant): `## X.Y.Z`, the line
  `Runs ShellyLanMan X.Y.Z.`, bullets with what a user notices (one line or two each),
  and `All changes: [ShellyLanMan changelog](…)`.
- `CHANGELOG.md` of the integration: `## X.Y.Z`, the line `Works with ShellyLanMan
  … or newer; Home Assistant … or newer.`, then bullets.
- Bullets, not paragraphs; user's words, not code names (except setting names in
  *italics* and API values in `code`).

## 6. Home Assistant test instance

A Home Assistant OS VM with the app and the integration; reached over SSH (an
*Advanced SSH & Web Terminal* app, protection mode on, so no `docker` there) and
through Home Assistant's WebSocket API with a long-lived token.

- **App update:** Supervisor API through the WebSocket command `supervisor/api`
  (the REST `/api/hassio` is closed to tokens): `POST /store/reload`, then
  `POST /store/addons/<slug>/update` with `{"backup": false}`; check
  `GET /addons/<slug>/info` (`version`, `state`). The app's log:
  `ha apps logs <slug>` over SSH (with `sudo -n -i`).
- **Integration update** (when it is not installed through HACS): copy
  `custom_components/shellylanman` to `/homeassistant/custom_components/` over SSH,
  then `ha core restart`.
- **Read-only checks:** WebSocket `config/device_registry/list`,
  `config_entries/get`, `system_log/list`.
- Since Home Assistant 2026.8 a device belongs to one config entry: ShellyLanMan's
  device for a Shelly and the Shelly integration's are two linked devices.

## 7. Real devices

Nothing writes to a real device without asking, except the agreed test devices in
phases that need writes — and even then say what will be done first (`CLAUDE.md`).
Fixtures are recorded read-only with `cmd/record` and reviewed (scrubbed) before they
are committed: this repository is public.

## 8. Documentation habits

- Decisions go to `DECISIONS.md` as a new numbered section; phase analyses to
  `docs/phase-NN-*.md`; hardware results to `docs/hardware-tests.md`. Append to these;
  never rewrite earlier sections.
- Every user-visible change has a `CHANGELOG.md` line in the same commit.
- Ported code: "Portions derived from ShellyScanner" header and a `docs/PROVENANCE.md`
  row in the same commit.
