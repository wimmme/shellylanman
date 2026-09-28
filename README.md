<p align="center">
  <img src="docs/images/logo-banner.png" alt="ShellyLanMan — Local · Monitor · Manage" width="420">
</p>

<p align="center">
  <a href="https://github.com/wimmme/shellylanman/releases"><img src="https://img.shields.io/github/v/release/wimmme/shellylanman?style=flat-square&color=0ea5e9" alt="Release"></a>
  <a href="https://github.com/wimmme/shellylanman/pkgs/container/shellylanman"><img src="https://img.shields.io/badge/docker-ghcr.io%2Fwimmme%2Fshellylanman-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker image"></a>
  <img src="https://img.shields.io/badge/platforms-amd64%20%7C%20arm64-6b7280?style=flat-square" alt="Platforms">
  <img src="https://img.shields.io/badge/Shelly-Gen1%20%7C%20Gen2%20%7C%20Gen3%20%7C%20Gen4%20%7C%20BLU-1e40af?style=flat-square" alt="Shelly generations">
  <img src="https://img.shields.io/badge/languages-8-0f766e?style=flat-square" alt="Languages">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wimmme/shellylanman?style=flat-square&color=161a3a" alt="Licence"></a>
</p>

# ShellyLanMan

**Discover, monitor and manage Shelly devices on your local network — in the browser.**

ShellyLanMan is a network-based tool for monitoring and managing Shelly IoT devices.
It automatically discovers devices on the local network and shows key information
such as Wi-Fi strength, cloud connectivity, uptime, temperature and meter readings.
Devices can be controlled directly, and configuration backup and restore are
available from the toolbar. Firmware management is built in, with a QR code for a
quick download of the firmware, so you can update a device through the Shelly's
own access point.

It is heavily based on [ShellyScanner](https://github.com/usnasoft/shellyscanner)
by usnasoft — the same features and terminology, as a native web UI in a single
lightweight Docker image. No desktop, no VNC, no Java, no cloud.

[Why](#why-shellylanman) · [Quick start](#quick-start) · [Features](#features) ·
[Pages](#pages) · [Screenshots](#screenshots) · [AI assistants (MCP)](#ai-assistants-mcp) · [Security](#security) ·
[Configuration](#configuration) · [Development](#development) · [Support](#support) · [Credits](#credits)

## Why ShellyLanMan

- **One place for every Shelly on the LAN** — Gen1 to Gen4, Pro, BLU through their
  gateways, range-extender clients and protected devices.
- **Runs where your other services run** — one Docker image for `amd64` and `arm64`
  (Raspberry Pi 4/5), opened from any browser, also on a phone.
- **Local first** — nothing leaves your LAN except what ShellyScanner also does (the
  devices' own firmware checks) and an opt-in release check. No account, no telemetry.
- **Firmware without the cloud** — a QR code gives your phone the firmware file from
  ShellyLanMan, to update a device through its own access point.
- **In your language** — English, Nederlands, Deutsch, Français, Español, Italiano,
  Български, 中文.

## Quick start

Linux with Docker Engine. `docker-compose.yml`:

```yaml
services:
  shellylanman:
    image: ghcr.io/wimmme/shellylanman:latest
    container_name: shellylanman
    network_mode: host
    volumes:
      - data:/data
    restart: unless-stopped

volumes:
  data:
```

```sh
docker compose up -d
```

Open `http://<your-host>:3082`. There is nothing to configure beforehand: a
first-run dialog appears in the browser.

Or with `docker run`:

```sh
docker run -d --name shellylanman --network host -v shellylanman-data:/data \
  --restart unless-stopped ghcr.io/wimmme/shellylanman:latest
```

Images are published for `linux/amd64` and `linux/arm64` (Raspberry Pi 4/5 with
a 64-bit OS). Tags: `latest`, `X.Y` and `X.Y.Z`.

### Update

```sh
docker compose pull && docker compose up -d
```

Use `docker compose up -d`, not `docker restart` — a restart keeps the old image.
To stay on one release line, use a tag such as `ghcr.io/wimmme/shellylanman:0.2`
instead of `latest`. The changes of each release are in [`CHANGELOG.md`](CHANGELOG.md).

Before a major version, save the data volume (it holds settings, archive, backups):

```sh
docker run --rm -v shellylanman_data:/data -v "$PWD":/backup alpine   tar czf /backup/shellylanman-data.tgz -C /data .
```

(`docker volume ls` shows the volume's name: compose prefixes it with the project
directory, e.g. `shellylanman_data`; the `docker run` example uses `shellylanman-data`). To go back, start the previous tag with the saved
volume content.

### Why host networking

ShellyScanner finds devices with mDNS, which uses multicast on your LAN.
Docker's default bridge network does not pass that multicast into the
container, so discovery by mDNS only works with `network_mode: host` (Linux).
In bridge mode (`-p 3082:3082`) everything else works and devices can be found
with an IP-range scan. Details: [`ARCHITECTURE.md` §2.5](ARCHITECTURE.md).

### Configuration

Everything is set in the browser. A few environment variables exist for things
needed before the UI is up:

| Variable | Default | Meaning |
|---|---|---|
| `SHELLYLANMAN_LISTEN` | — | Listen address. Normally the port is set in Settings → General (default 3082); when this variable is set it wins and the setting is locked |
| `SHELLYLANMAN_DATA` | `/data` | Data directory |
| `SHELLYLANMAN_ORIGINS` | — | Extra allowed browser origins (host names), comma separated, e.g. your reverse proxy's name |
| `TZ` | `UTC` | Time zone for logs |

### What is stored in `/data`

| File | Content |
|---|---|
| `secret.key` | Random key created on first start; encrypts secrets in `settings.json` |
| `settings.json` | Application settings; device credentials encrypted |
| `archive.json` | Device archive: known devices, last address, notes and keywords |
| `deferred.json` | Deferred tasks for off-line devices (passwords encrypted) |
| `backups/<device>/*.sbk` | Device backups (newest N per device, setting) |
| `firmware/` | Verified firmware files of the local download (cache) |

Chart readings are kept in memory only (24 hours). Details:
[`ARCHITECTURE.md` §2.6](ARCHITECTURE.md).

### Behind a reverse proxy

Terminate TLS at the proxy, forward WebSocket upgrades for `/ws`, and set
`SHELLYLANMAN_ORIGINS` to the public host name. UI authentication is off by
default — add authentication at the proxy if the UI is reachable beyond your
own LAN. See [`SECURITY.md`](SECURITY.md).

## Features

Everything ShellyScanner does, in the browser:

- **Discovery** by mDNS (all interfaces or one), IP-range scan or offline, Gen1 to
  Gen4, Pro, BLU devices through their gateways, range-extender clients, protected
  devices, an archive of known devices with notes and keywords.
- **Devices table** with all columns, filters, views, device information, live logs,
  controls (relays, rollers, lights, thermostats, …), reboot, CSV export and print.
- **Configuration** of one or many devices (Wi-Fi, login, MQTT, NTP, cloud, …), the
  configuration checklist and deferred tasks for devices that are off line.
- **Backup and restore** (`.sbk`, compatible with ShellyScanner), kept on the server.
- **Firmware** check and update with live progress, plus one new feature: a **QR code
  for a local firmware download**, to update a device through its own access point.
- **Scripts** (with a code editor) and KVS, **schedulers** (Gen2+, Wall Display
  thermostat, BLU TRV) and **charts** with 24 hours of history.
- **Appearance**: dark and light themes, colour palettes, fonts and sizes, per browser.

What leaves your LAN: the devices' own firmware checks (as with ShellyScanner); Shelly's
firmware index when you open the Firmware page; the ShellyLanMan release check only
if you switch it on (off by default). No telemetry.

## Pages

| Page | What it is for |
|---|---|
| **Devices** | The live table of every device: status, type, name, IP, RSSI, cloud, MQTT, uptime, temperature, measurements and controls. The toolbar works on the selected devices: info, logs, web UI, reload, reboot, checklist, settings, charts, scheduler, scripts, notes, backup, restore. |
| **Checklist** | One row per device with the settings worth checking — eco mode, LED, logs, Bluetooth, access point, roaming, Wi-Fi, range extender, scripts, automatic firmware update — and right-click actions to fix them. |
| **Charts** | Power, energy, voltage, temperature, RSSI and more for the selected devices, with 24 hours of history, zoom, pause and CSV export. |
| **Firmware** | Current, stable and beta firmware per device, update with live progress, and the QR code for the local download. |
| **Deferred** | Actions for off-line devices, run when the device comes back. |
| **Settings** | Scan mode and IP ranges, archive, device credentials, backups, web server port, script editor, appearance and language. |
| **About** | What is running (version, runtime, uptime), release notes, dependencies with their licences, credits and help. |

## Screenshots

| | |
|---|---|
| ![Devices](docs/images/screenshot-devices.png) | ![Checklist](docs/images/screenshot-checklist.png) |
| **Devices** — the live table with controls | **Checklist** — settings worth checking |
| ![Firmware](docs/images/screenshot-firmware.png) | ![About](docs/images/screenshot-about.png) |
| **Firmware** — check, update and the Shelly index | **About** — version, system information and support |

The screenshots show simulated devices (`cmd/shellysim`).

## AI assistants (MCP)

ShellyLanMan has a built-in [Model Context Protocol](https://modelcontextprotocol.io)
server, so an AI assistant — Claude Code, Claude Desktop, or a local model — can ask
about your devices and, if you allow it, switch them. It is **local only**: the
assistant talks to ShellyLanMan, ShellyLanMan talks to the devices on your LAN. No
Shelly cloud account, nothing leaves your network except what your AI client itself
sends to its model.

Switch it on in **Settings → MCP**. That makes a token (shown once) and gives you the
command to paste, for example:

```sh
claude mcp add --transport http shellylanman http://<your-host>:3082/mcp \
  --header "Authorization: Bearer <token>"
```

| Tools | |
|---|---|
| Read (always) | `shelly_list_devices`, `shelly_get_device`, `shelly_get_readings` (24 h of history), `shelly_firmware_check`, `shelly_checklist`, `shelly_list_backups`, `shelly_rpc_read` (Gen2+ `Get*`/`List*` methods only) |
| Control (when set to *read and control*) | `shelly_switch`, `shelly_light`, `shelly_cover`, `shelly_thermostat`, `shelly_backup` |
| Destructive (control, and `confirm: true`) | `shelly_reboot`, `shelly_firmware_update` |

Off by default, read-only unless you choose otherwise, a bearer token on every
request, browser requests from other sites refused, and every control action written
to the log. Devices are named by name, host name, IP or MAC.

## Security

- **Keep it on your LAN.** ShellyLanMan can change the configuration of your devices;
  do not expose it to the internet. For remote access use a reverse proxy with TLS
  **and** authentication.
- **UI authentication is off by default** — anyone who can reach the port can use it.
  A warning is logged at start and shown once per browser.
- **Secrets at rest** — device and Wi-Fi passwords are encrypted in `/data` with
  AES-256-GCM; they are never sent back to the browser.
- **Browser protection** — same-origin checks on every state-changing request and the
  WebSocket, a strict Content Security Policy, no third-party scripts or fonts.
- **Destructive actions** (reboot, restore, firmware update, port change) always ask
  for confirmation, and the API requires `confirm: true`.
- **MCP** is off by default, needs a token, and is read-only unless you allow control.
- Report vulnerabilities privately: see [`SECURITY.md`](SECURITY.md).

## Development

Only Docker is needed:

```sh
sh tools/verify.sh     # typecheck, tests, gofmt, vet, go test -race, image build
```

Try it without hardware: `go run ./cmd/shellysim testdata/gen1/SHPLG-S testdata/gen2/Plus1`
starts simulated devices on ports 8081 and 8082. See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Support

ShellyLanMan is free and open source, and always will be. If it saves you time, a
coffee (or a few tokens for coding) is welcome — and never expected.

<a href="https://buymeacoffee.com/wimmme"><img src="https://img.shields.io/badge/Buy%20me%20a%20coffee-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black" alt="Buy me a coffee"></a>
<a href="https://www.paypal.com/donate/?business=LPS62D2BRTD2Y&no_recurring=0&item_name=You+help+me+buying+coffee+and+tokens+for+coding+%3A-%29&currency_code=EUR"><img src="https://img.shields.io/badge/Donate-PayPal-0070BA?style=for-the-badge&logo=paypal&logoColor=white" alt="Donate with PayPal"></a>

Bugs and ideas: [open an issue](https://github.com/wimmme/shellylanman/issues/new).

## Credits

- **[ShellyScanner](https://github.com/usnasoft/shellyscanner)** by Antonio
  Flaccomio (usnasoft) — the functional and code reference for everything
  ShellyLanMan does: every feature, the terminology and the Shelly know-how. More at
  https://www.usna.it/shellyscanner/. Beyond the credits, he deserves a coffee too.
- **[MikroDash](https://github.com/SecOps-7/MikroDash)** by SecOps-7 — the idea, the
  inspiration and the basis of the look and feel: design tokens, palettes and
  appearance settings (MIT). Beyond the credits, SecOps-7 deserves a coffee too.
- CodeMirror 6 (script editor), Chart.js (charts), go-qrcode (QR codes) — MIT;
  fonts under the SIL Open Font License. Full list in
  [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Disclaimer

ShellyLanMan is an independent project. It is heavily based on ShellyScanner but
not affiliated with it, and not affiliated with or endorsed by usnasoft or Shelly
Group. Shelly is a trademark
of its owner.

## Licence

[GPL-3.0-or-later](LICENSE).
