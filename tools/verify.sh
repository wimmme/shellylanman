#!/bin/sh
# Everything the repository can check, in Docker, so the host needs only Docker:
# frontend typecheck + tests + build, gofmt, go vet, go test -race, and the
# production image build. New tests are found automatically (Go *_test.go,
# web/test/*.test.ts).
#
#   sh tools/verify.sh
set -eu
cd "$(dirname "$0")/.."
docker build --target test -t shellylanman:test .
docker build -t shellylanman:dev .
echo "verify: OK"
