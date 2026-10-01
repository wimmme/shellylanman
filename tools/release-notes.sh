#!/bin/sh
# Prints the CHANGELOG.md section of one version (without its heading), for the
# GitHub release notes: sh tools/release-notes.sh 0.3.0  (a leading "v" is allowed).
# With --check: fails unless every released version in CHANGELOG.md has notes.
set -eu
changelog="$(dirname "$0")/../CHANGELOG.md"
if [ "${1:-}" = "--check" ]; then
	versions="$(sed -n 's/^## \[\([0-9][0-9.]*\)\].*/\1/p' "$changelog")"
	[ -n "$versions" ] || { echo "no released versions in CHANGELOG.md" >&2; exit 1; }
	for v in $versions; do
		sh "$0" "$v" >/dev/null
	done
	echo "release notes: OK ($(echo $versions | wc -w) versions)"
	exit 0
fi
v="${1#v}"
[ -n "$v" ] || { echo "usage: $0 X.Y.Z | --check" >&2; exit 2; }
notes="$(awk -v v="$v" '
	/^## \[/ { if (on) exit; on = index($0, "## [" v "]") == 1; next }
	/^\[[^]]*\]: / { if (on) exit }
	on { print }
' "$changelog" | sed -e '/./,$!d')"
[ -n "$notes" ] || { echo "no CHANGELOG.md section for $v" >&2; exit 1; }
printf '%s\n' "$notes"
