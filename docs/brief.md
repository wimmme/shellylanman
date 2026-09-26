# Project brief: ShellyLanMan

> Paste this as the first message of a new Claude Code session in an empty repository.
> After Phase 0 its standing rules move into `CLAUDE.md`.

## Your role

Act as a **senior software engineer and architect** on this project. That means: you question
requirements that don't add up, you weigh trade-offs out loud, you prefer boring and proven over
clever, you think about the maintainer who comes after you, and you **ask instead of assume**.
A good question now is cheaper than a wrong assumption baked into the architecture.

## What we are building

**ShellyLanMan**: a **web application for discovering, monitoring and managing Shelly devices on the local network**,
based on **ShellyScanner** (https://github.com/usnasoft/shellyscanner, GPL-3.0, by usnasoft),
a Java/Swing desktop application I use and like.

Today I run the original inside a Docker container with a VNC desktop so I can reach it from a browser.
That works, but it is clumsy: the UI is a remote desktop, and updating means rebuilding a desktop image.
The goal is a **native web application, shipped as a single lightweight Docker image**, that I can update
with `docker compose pull && docker compose up -d`. No GUI, no VNC, no Java runtime required by the user.

## Guiding principles (in priority order)

1. **Maintainable and lightweight.** Mainstream technology with a large community, few dependencies,
   each one justified. Readable over clever, no unnecessary abstractions. Small image, low resource use,
   fast startup. This project will be maintained by me, by Claude, and hopefully by a community.
2. **Functionally faithful to ShellyScanner.** Same features, same terminology, same information per device,
   same workflows. Existing users and the original author should recognise it as ShellyScanner on the web.
   No functionality may be lost without it being explicitly recorded and agreed. This is "based on
   ShellyScanner", not a line-by-line port: where a cleaner design serves principle 1, prefer that.
3. **Visually modelled on MikroDash** (see below), not on the Swing UI.
4. **Docker-first.** One container, one `/data` volume, no `.env` required to get started.
5. **Architecture that leaves room for later integrations** (MCP server, Home Assistant), without building them now.

## Core working principle: the source code is the truth

- Analyse the **ShellyScanner source code**, not just its README or website. The code is the functional truth.
- When the original does something whose reason is unclear: **find the code → trace the call chain →
  identify the Shelly API endpoint(s) → work out why → document it → only then implement the equivalent.**
- **Do not assume Shelly generations share an API model.** Gen1 (HTTP REST), Gen2/Gen3/Gen4 (RPC) and BLU/BLE
  devices behave differently; so do individual device types. Verify against the original code and the
  official Shelly API documentation.
- Do not invent functionality that isn't in the original. When in doubt:
  **STOP → ANALYSE THE ORIGINAL → DOCUMENT → ASK OR IMPLEMENT.**

## Technology: compare, recommend, let me decide

I have no fixed preference. Evaluate at least **Go, Java (e.g. Spring Boot or a lighter framework such as
Javalin), Node.js/TypeScript and Python** for the backend, on:

simplicity · community size · maintainability · image size and memory footprint · Docker and multi-arch
(amd64, arm64, ideally armv7) · HTTP client and server · WebSocket · mDNS/multicast discovery · concurrency
(polling many devices) · testability · code readability · fit for a small self-hosted app ·
ease of community contributions · reuse of the original's logic.

Weighting: maintainability and footprint first, community second, closeness to the original's language third.
Keeping Java would help the original author contribute, but only if the cost in footprint and complexity is
acceptable. If not, choose the better technology; the original then gets full credit for code and reference.
Go + TypeScript (as MikroDash uses) is an obvious candidate, but treat it as a hypothesis, not a decision.

Frontend: **TypeScript**, modern HTML/CSS, a light library only if it clearly pays for itself.
Live updates over **WebSocket**. SQLite only if persistent structured storage is actually needed.
Build entirely inside a multi-stage Dockerfile; the host needs only Docker.

Give a short comparison table, your recommendation and reasoning, and let me decide.

## Look and feel: MikroDash

https://github.com/SecOps-7/MikroDash (MIT) is the reference for **look, feel and quality**.
I like its appearance, themes and styling and want that here:

- Layout: sidebar navigation, card-based pages, summary cards above tables, sortable/filterable tables,
  live updates without refresh, status pills and badges.
- **Theming**: named colour palettes (dark and light), contrast/brightness adjustment, font choice and font size,
  persisted per browser.
- Polish: responsive layout, clear loading, empty and error states, confirmation dialogs for destructive actions.

Study its `web/` folder and screenshots. MIT is compatible with GPL-3.0: reusing its CSS, design tokens or
patterns is allowed with attribution in `THIRD_PARTY_NOTICES.md`. Tell me what you plan to reuse.

**Functionally**, ShellyScanner stays the UX reference: keep its terminology, information hierarchy,
device table columns, status indicators, actions, filters, dialogs and configuration screens.
Propose how each of these maps onto MikroDash's visual language (e.g. the device table becomes a MikroDash-style
table page; toolbar actions become page or row actions; dialogs become modals or detail panels).

## Method and quality: also MikroDash

MikroDash was built with Claude Code and is the bar for engineering practice. Study its README, `CLAUDE.md`,
`AI_CONTEXT.md`, `CONTRIBUTING.md`, `Dockerfile` and `docker-compose.yml`, and adopt:

- `CLAUDE.md` with standing rules for AI-assisted work.
- Single `/data` volume; credentials encrypted at rest with a key auto-generated on first run;
  first-run wizard in the browser instead of environment variables.
- Health endpoint, multi-arch image published to GHCR on version tags only, `CHANGELOG.md`, `SECURITY.md`,
  `THIRD_PARTY_NOTICES.md`, upgrade instructions.
- Optional authentication (off by default for LAN use, with a startup warning), WebSocket origin check,
  reverse-proxy guidance.

## Networking and Docker

This is a LAN tool. Normal operation must not depend on the internet.

- Document how the original **discovers devices** (mDNS, IP range scan, anything else), finds IP addresses,
  identifies devices, re-detects them, and handles offline devices; and what its **offline scan** does.
- Docker networking affects multicast, mDNS and broadcast. Evaluate **bridge vs. host networking**
  (and any multicast configuration or mDNS reflector options) and make a **deliberate, documented design choice**,
  including what does and does not work in each mode.
- Document **exactly what is stored in `/data`**.
- Minimal `docker-compose.yml` and `docker run` example in the README, in the style of MikroDash's Quick Start.

## Firmware, and the one new feature

First fully understand the existing firmware update functionality:
how ShellyScanner checks and triggers updates per generation, which official Shelly firmware sources and
URLs/APIs exist and how reliable they are, how firmware maps to device type, whether the device downloads
firmware itself or it must be served locally, and the security and compatibility risks.

Only then, the **one new feature: local firmware download via QR code.** For a device with a newer stable
firmware available, the UI shows a QR code pointing to a URL *on this server* from which the latest stable
firmware for that device can be downloaded (the server fetches and caches it). Purpose: updating devices that
cannot reach the internet themselves, or from a phone. Propose how this fits the existing process.
<!-- TODO Wim: add the details / link from the other project here. -->

## Future integrations: keep in mind, don't build

- **MCP server.** Eventually the application should expose an MCP server, so an AI assistant can query and
  manage Shelly devices directly, without a generic shell MCP server.
- **Home Assistant.** The existing Shelly integration in Home Assistant requires too much manual adding and
  configuring per device. Eventually this project could be the single place that discovers and manages devices,
  and feed Home Assistant from there.

Implication: all functionality lives in a **clean service layer with a well-defined API**; the web UI is just one
client, so an MCP server or HA integration can later be a thin layer on top. In Phase 0, explain how your
architecture accommodates both, and propose which HA approach fits best (custom integration against our API,
MQTT discovery, or other).

## Testing

Testing is not optional. From the first commit:

- A setup that makes adding a test **easy and obvious**: one command runs everything, locally and in CI.
  CI fails on failing tests.
- **Device fixtures**: recorded payloads per generation and device type (Gen1, Gen2, Gen3, Gen4, BLU where
  relevant) under `testdata/`.
- A **simulated Shelly device / mock Shelly API** (small fake HTTP/RPC server, with mDNS announcement where
  feasible) so discovery, polling, configuration, backup/restore and firmware flows run end to end without hardware.
- Unit tests for parsing and logic, integration tests for API and WebSocket, smoke tests for the frontend.
- **Real-hardware compatibility testing** on my devices, across generations, for each phase that touches devices.
  Tests in CI never touch real devices.
- No feature is ticked off in `FEATURE_PARITY.md` without tests.

## Licensing and attribution

Check the licences explicitly; do not assume. This is technical documentation, not legal advice.

- ShellyScanner is **GPL-3.0**. Distinguish clearly, and record per component, between:
  1. code copied directly;
  2. code **translated/ported** to another language (still a derivative work);
  3. clean re-implementation from functional analysis;
  4. entirely new code.
  Do not assume "same functionality" legally equals "derived code", nor the reverse.
- The new project is licensed **GPL-3.0**. Keep original copyright notices where code is copied or ported;
  credit usnasoft and link the original in the README and the UI's About dialog.
- ShellyScanner depends on **usnalib2** (https://github.com/usnasoft/usnalib2): check its licence.
- MikroDash (MIT) material: attribute in `THIRD_PARTY_NOTICES.md`.
- Check the licence of every dependency of the new project.
- The project name is **ShellyLanMan** (repository, Docker image, UI title). Do not present it as an official
  ShellyScanner or Shelly (Allterco) product: state in the README and About dialog that it is an independent
  project based on ShellyScanner, and that Shelly is a trademark of its owner (as MikroDash does for MikroTik).

## Making it easy for the original author to join

- Clear names, simple structure, documented design decisions, testable code, a good README.
- In `ARCHITECTURE.md`, a **mapping table from each Java component to its new counterpart**
  (e.g. device model → new device model; discovery → discovery service; devices table → web device table;
  each dialog → its web equivalent).

## How we work

### Phase 0 — Analysis and design. Write no application code.

Analyse ShellyScanner (full source, dependencies, packages/modules, UI) and MikroDash. The inventory must
cover at least, and anything else found in the code:

architecture · packages/modules · device model · supported generations and device types · discovery ·
network communication and every Shelly API call · device status information · device controls ·
configuration functions · backup/restore · firmware update · scripts · KVS · BLE/BTHome · schedules ·
graphs · CSV/export · filters and sorting · device notes · settings · offline scan · CLI · persistence.

Deliver three documents:

- **`ARCHITECTURE.md`** — how the original is built; the proposed new architecture (including the service layer,
  and how MCP and HA fit later); repository layout; Docker and networking model; `/data` contents;
  Java → new component mapping.
- **`FEATURE_PARITY.md`** — matrix of every feature: *feature · where in the Java code · Shelly API used ·
  generations · web equivalent · status*. For anything that can't transfer directly to a web app:
  why, how Java does it, the web approach, and browser limitations.
- **`DECISIONS.md`** — technology comparison and recommendation, UI mapping onto MikroDash, licensing analysis
  and attribution plan, firmware/QR approach, test strategy, phased plan, **risks and unknowns**.

Then:

- **List your open questions for me.** Anything you would otherwise have to assume goes on this list.
- **Stop and wait.** Do not build until I have answered and approved.

### After approval

Read before write: nothing writes to a device until reading is solid.

1. **Skeleton** — repository, backend, themed empty frontend, WebSocket, `/data`, Dockerfile, compose file,
   health endpoint, test framework with simulated device, CI building and testing the image.
2. **Discovery** — discovery, device identification, device list, online/offline status.
3. **Read-only device information** — device and firmware info, Wi-Fi, cloud state, uptime, temperature,
   meters and other status the original shows.
4. **Device controls.**
5. **Configuration.**
6. **Backup/restore.**
7. **Firmware**, then the QR feature.
8. **Advanced functionality** — scripts, KVS, BLE/BTHome, schedules, graphs, export, notes, filters, and the rest.
9. **Parity review** — systematic comparison against the original using `FEATURE_PARITY.md`.
10. **Release** — production multi-arch image, versioned releases on GHCR, upgrade instructions.

## Working rules

- Small, reviewable commits, one logical change each. Keep `CHANGELOG.md` current.
- When the original's behaviour is unclear or looks like a bug, note it in `FEATURE_PARITY.md` and ask;
  don't silently "fix" it.
- No new features beyond the original plus the QR feature without my explicit approval.
- No cloud dependency, no telemetry. Nothing leaves the LAN except what the original already does
  (e.g. checking for firmware updates).
- Destructive actions (reboot, restore, firmware update, factory reset) always need explicit confirmation in the UI.
- I test against my own devices. When you need me to try something, tell me exactly what to run and what to look for.
- At the end of each phase: summarise what was done, what was tested, what is open, and wait for my go.
