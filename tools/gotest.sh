#!/bin/sh
# Run go vet + go test for some packages in the Go image (quick check during
# development; tools/verify.sh is the full gate).
#
#   sh tools/gotest.sh ./internal/parse/ ./internal/service/
set -eu
cd "$(dirname "$0")/.."
pkgs="${*:-./...}"
docker run --rm -v "$PWD":/src -w /src -v shellylanman-gomod:/go/pkg/mod -v shellylanman-gocache:/root/.cache/go-build \
  golang:1.27 sh -c "gofmt -l internal cmd; go vet $pkgs && go test -race -count=1 $pkgs; rc=\$?; chown -R $(id -u):$(id -g) /src; exit \$rc"
