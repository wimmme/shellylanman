# ShellyLanMan

**Discover, monitor and manage Shelly devices on your local network — in the browser.**

ShellyLanMan is a web application based on
[ShellyScanner](https://github.com/usnasoft/shellyscanner) by usnasoft: the same
features and terminology, as a native web UI in a single lightweight Docker
image. No desktop, no VNC, no Java, no cloud.

> **Status: early development.** The skeleton (server, themed UI, data store,
> Docker image, tests) is in place; device discovery and management follow
> phase by phase. See [`FEATURE_PARITY.md`](FEATURE_PARITY.md) for progress.

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

Until the first release is published, build it yourself: `docker compose up -d --build`
in a clone of this repository.

### Update

```sh
docker compose pull && docker compose up -d
```

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
| `SHELLYLANMAN_LISTEN` | `:3082` | Listen address (with host networking: pick a free host port) |
| `SHELLYLANMAN_DATA` | `/data` | Data directory |
| `SHELLYLANMAN_ORIGINS` | — | Extra allowed browser origins (host names), comma separated, e.g. your reverse proxy's name |
| `TZ` | `UTC` | Time zone for logs |

### What is stored in `/data`

| File | Content |
|---|---|
| `secret.key` | Random key created on first start; encrypts secrets in `settings.json` |
| `settings.json` | Application settings; device credentials encrypted |
| `archive.json` | Device archive: known devices, last address, notes and keywords |

Later phases add backups, deferred actions and a firmware cache — the full list is in
[`ARCHITECTURE.md` §2.6](ARCHITECTURE.md).

### Behind a reverse proxy

Terminate TLS at the proxy, forward WebSocket upgrades for `/ws`, and set
`SHELLYLANMAN_ORIGINS` to the public host name. UI authentication is off by
default — add authentication at the proxy if the UI is reachable beyond your
own LAN. See [`SECURITY.md`](SECURITY.md).

## Development

Only Docker is needed:

```sh
sh tools/verify.sh     # typecheck, tests, gofmt, vet, go test -race, image build
```

Try it without hardware: `go run ./cmd/shellysim testdata/gen1/SHPLG-S testdata/gen2/Plus1`
starts simulated devices on ports 8081 and 8082. See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Credits

- **[ShellyScanner](https://github.com/usnasoft/shellyscanner)** by Antonio
  Flaccomio (usnasoft) — the functional and code reference for everything
  ShellyLanMan does. More at https://www.usna.it/shellyscanner/.
- **[MikroDash](https://github.com/SecOps-7/MikroDash)** — the look and feel:
  design tokens, palettes and appearance settings (MIT).
- Fonts under the SIL Open Font License. Full list in
  [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Disclaimer

ShellyLanMan is an independent project. It is not ShellyScanner, and it is not
affiliated with or endorsed by usnasoft or Shelly Group. Shelly is a trademark
of its owner.

## Licence

[GPL-3.0-or-later](LICENSE).
