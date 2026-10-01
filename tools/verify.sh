#!/bin/sh
# Everything the repository can check, in Docker, so the host needs only Docker:
# frontend typecheck + tests + build, gofmt, go vet, go test -race, the
# production image build, and the release notes of every CHANGELOG.md version.
# New tests are found automatically (Go *_test.go, web/test/*.test.ts).
#
#   sh tools/verify.sh
set -eu
cd "$(dirname "$0")/.."
sh tools/release-notes.sh --check
docker build --target test -t shellylanman:test .
docker build -t shellylanman:dev .
echo "verify: OK"
