#!/bin/sh
# Sync the working tree to a Docker host and run a command there, for
# development machines without Docker. Not needed if you have Docker locally.
#
#   REMOTE=user@dockerhost sh tools/remote.sh sh tools/verify.sh
#
# Uses ~/build/shellylanman on the remote host. Files are copied, not synced
# back; generated files you want to keep (go.sum, package-lock.json, fixtures)
# must be fetched explicitly.
set -eu
: "${REMOTE:?set REMOTE=user@host}"
DIR="${REMOTE_DIR:-build/shellylanman}"
SSH_OPTS="${SSH_OPTS:-}"
cd "$(dirname "$0")/.."
tar --exclude=./.git --exclude=./web/node_modules --exclude=./web/dist --exclude=./web/test/.out -cf - . |
  ssh $SSH_OPTS "$REMOTE" "rm -rf '$DIR' && mkdir -p '$DIR' && tar -xf - -C '$DIR'"
ssh $SSH_OPTS "$REMOTE" "cd '$DIR' && $*"
