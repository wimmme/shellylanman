# Contributing

Thanks for helping. ShellyLanMan is a web version of ShellyScanner; the goal is
**functional parity** with it, in a small, maintainable package.

## Before you start

- Read `ARCHITECTURE.md` (how it fits together), `FEATURE_PARITY.md` (what
  exists, what is missing) and `DECISIONS.md` §8 (decisions already taken).
- New features beyond ShellyScanner's (plus the firmware QR feature) need an
  issue and agreement first.
- If ShellyScanner does something unclear, find it in its source, trace it to
  the Shelly API call, and write down why before changing anything.

## Development

You only need Docker:

```sh
sh tools/verify.sh        # everything CI checks, then the production image
docker compose up -d --build
```

Without Docker on your machine, `REMOTE=user@dockerhost sh tools/remote.sh sh tools/verify.sh`
runs the same on another host.

Layout: Go in `cmd/` and `internal/`, TypeScript in `web/src`, fixtures in
`testdata/`. Tests live next to the code (`*_test.go`, `web/test/*.test.ts`) and
are discovered automatically.

## Rules

- Every change comes with tests. A row in `FEATURE_PARITY.md` is ticked only
  when it is implemented **and** tested.
- Small commits, one logical change each; update `CHANGELOG.md`.
- Code ported from ShellyScanner gets the "Portions derived from ShellyScanner"
  header and a line in `docs/PROVENANCE.md`.
- Fixtures: record with `cmd/record` (it scrubs MACs, IPs, SSIDs, names) and
  review them — this repository is public.
- A dependency needs a reason; its licence goes into `THIRD_PARTY_NOTICES.md`
  first.
- CI never touches real devices.

By contributing you agree that your contribution is licensed under
GPL-3.0-or-later.
