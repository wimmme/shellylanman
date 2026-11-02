#!/bin/sh
# Builds the release tarballs (tools/dist.sh) and checks them: the expected files,
# the checksums, and the binary for this machine starts and answers /healthz.
#
#   sh tools/dist-check.sh
set -eu
cd "$(dirname "$0")/.."
v=0.0.0-check
sh tools/dist.sh "$v" >/dev/null
cd dist
sha256sum -c SHA256SUMS
for arch in amd64 arm64; do
	files="$(tar -tzf "shellylanman_${v}_linux_$arch.tar.gz" | LC_ALL=C sort | tr '\n' ' ')"
	want="shellylanman_${v}_linux_$arch/ shellylanman_${v}_linux_$arch/LICENSE shellylanman_${v}_linux_$arch/THIRD_PARTY_NOTICES.md shellylanman_${v}_linux_$arch/shellylanman shellylanman_${v}_linux_$arch/shellylanman.service "
	[ "$files" = "$want" ] || { echo "dist-check: $arch tarball holds: $files" >&2; exit 1; }
done
case "$(uname -m)" in
x86_64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) echo "dist-check: OK (no binary for $(uname -m) to start)"; exit 0 ;;
esac
tar -xzf "shellylanman_${v}_linux_$arch.tar.gz"
data="$(mktemp -d)"
port=3199
SHELLYLANMAN_DATA="$data" SHELLYLANMAN_PORT=$port "./shellylanman_${v}_linux_$arch/shellylanman" >"$data/out.log" 2>&1 &
pid=$!
# Wait for the server to stop before removing its data: it still writes there while it shuts down.
trap 'kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true; rm -rf "$data"' EXIT
i=0
until [ -s "$data/listen.port" ] && SHELLYLANMAN_DATA="$data" "./shellylanman_${v}_linux_$arch/shellylanman" -healthcheck >/dev/null 2>&1; do
	i=$((i + 1))
	[ $i -lt 20 ] || { echo "dist-check: the binary did not come up:" >&2; cat "$data/out.log" >&2; exit 1; }
	sleep 0.5
done
grep -q "version=v$v" "$data/out.log" || { echo "dist-check: version not in the log" >&2; cat "$data/out.log" >&2; exit 1; }
echo "dist-check: OK"
