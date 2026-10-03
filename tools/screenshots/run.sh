#!/bin/sh
# README screenshots with simulated devices, on a Docker host. Run from the
# repository root (on a machine without Docker: through tools/remote.sh):
#
#   sh tools/screenshots/run.sh            # PNGs in ./screenshots-out
#
# Builds the current tree as slm-shots:latest, starts it on port 3199 (not the
# usual 3082) with an IP scan of 127.0.0.2–14, starts tools/screenshots/sims.sh
# in a golang container on the same network, then shoot.py in the Playwright
# image. Removes its containers at the end. Touches no real device: the scan
# covers loopback addresses only.
set -eu
cd "$(dirname "$0")/../.."
OUT="$PWD/screenshots-out"
PW=mcr.microsoft.com/playwright/python:v1.55.0-noble
GO=golang:1.27
mkdir -p "$OUT"
cleanup() { docker rm -f slm-shots slm-shots-sim >/dev/null 2>&1 || true; }
cleanup
trap cleanup EXIT

docker build -q -t slm-shots:latest . >/dev/null
docker run -d --name slm-shots -p 3199:3082 slm-shots:latest >/dev/null
sleep 3
curl -fsS -o /dev/null -X PUT -H 'Content-Type: application/json' \
  -d '{"firstRunDone":true,"scan":{"mode":"ip","ranges":[{"base":"127.0.0","first":2,"last":14}],"refreshSeconds":2,"configTics":5}}' \
  http://127.0.0.1:3199/api/v1/settings
docker run -d --name slm-shots-sim --network container:slm-shots -v "$PWD":/src -w /src $GO sh tools/screenshots/sims.sh >/dev/null
# Wait for the simulators (the first run compiles), then scan again.
until [ "$(docker logs slm-shots-sim 2>&1 | grep -c ' on http')" -ge 11 ]; do sleep 2; done
curl -fsS -o /dev/null -X POST http://127.0.0.1:3199/api/v1/scan
sleep 20
docker run --rm --network container:slm-shots -v "$PWD/tools/screenshots":/tools -v "$OUT":/shots $PW \
  sh -c 'pip install -q --break-system-packages playwright==1.55.0 >/dev/null 2>&1; python3 /tools/shoot.py'
ls "$OUT"
