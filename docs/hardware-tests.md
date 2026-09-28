

## Phase 10 — release (2026-09-28)

| Check | Result |
|---|---|
| GitHub Actions "Test" on every push | green (last 8 runs) |
| arm64 build (cross-compiled `build` stage on dockerhostvm) | static ARM aarch64 binary, 10.4 MB; the final stage needs QEMU, which is not installed on dockerhostvm — it runs on GitHub at the first tag |
| Release check on the test container | setting off: no request; stable: checked, no release yet, no error; back to off |
| Retry of failed devices every 2 minutes | simulator test (device down when discovered, back later → on line) |
