# Test fixtures

Recorded responses of real Shelly devices, one directory per model:

```
testdata/gen1/<type>/     e.g. gen1/SHPLG-S   (type from /shelly)
testdata/gen2/<app>/      e.g. gen2/Plus1     (app from /shelly; also gen3/, gen4/)
testdata/blu/<model>/
```

One file per request, named by `internal/fixture.FileName`:
`/shelly` → `shelly.json`, `/settings/actions` → `settings_actions.json`,
`/rpc/Shelly.GetStatus` → `rpc_Shelly.GetStatus.json`.

## Recording

```sh
go run ./cmd/record -host <device-ip> -out testdata/gen2/<app>
```

`record` sends only GET requests. Everything is **scrubbed** before it is written:
MAC addresses become `AABBCC0000nn`, private IPv4 addresses become `192.0.2.n`
(consistently within one recording), and values of identifying keys (SSID,
names, user, server, password, latitude/longitude, time zone) are replaced.
`TestRepositoryFixturesAreScrubbed` fails the build if a real-looking MAC or a
private IP appears anywhere under `testdata/`. Still review a new fixture before
committing it: this repository is public.

## Present

| Directory | Device | Firmware | Recorded |
|---|---|---|---|
| `gen1/SHPLG-S` | Shelly Plug S (Gen1) | v1.14.0 | 2026-09-26 |
| `gen2/Plus1` | Shelly Plus 1 (Gen2) | 1.7.5 | 2026-09-26 |

Use them with the simulator: `go run ./cmd/shellysim testdata/gen1/SHPLG-S testdata/gen2/Plus1`.
