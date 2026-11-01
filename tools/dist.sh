#!/bin/sh
# Builds the release tarballs, in Docker (the Dockerfile's binary target):
#
#   sh tools/dist.sh 1.2.3 [commit]       (a leading "v" is allowed)
#
# dist/shellylanman_1.2.3_linux_amd64.tar.gz and …_linux_arm64.tar.gz, each with
# one directory holding the binary, the systemd unit, LICENSE and the third-party
# notices; and dist/SHA256SUMS. Go cross-compiles, so nothing is emulated.
set -eu
cd "$(dirname "$0")/.."
v="${1#v}"
[ -n "$v" ] || { echo "usage: $0 X.Y.Z [commit]" >&2; exit 2; }
commit="${2:-$(git rev-parse HEAD 2>/dev/null || true)}"
rm -rf dist
docker buildx build --target binary --platform linux/amd64,linux/arm64 \
	--build-arg VERSION="v$v" --build-arg COMMIT="$commit" \
	--output type=local,dest=dist/bin .
cd dist
for arch in amd64 arm64; do
	name="shellylanman_${v}_linux_$arch"
	mkdir "$name"
	cp "bin/linux_$arch/shellylanman" "$name/"
	chmod 0755 "$name/shellylanman"
	cp ../packaging/shellylanman.service ../LICENSE ../THIRD_PARTY_NOTICES.md "$name/"
	tar -czf "$name.tar.gz" --owner=0 --group=0 "$name"
	rm -r "$name"
done
rm -r bin
sha256sum -- *.tar.gz > SHA256SUMS
cat SHA256SUMS
