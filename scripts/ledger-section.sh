#!/bin/sh

# Each module release owns one immutable tag. Publishing a different module, or
# adding one to go.work, cannot reopen this release's API ledger section.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$root"
section=$(sed -n '/^## \[[^]]*\/v[0-9]/{s/^## \[\([^]]*\)\].*/\1/;p;q;}' CHANGELOG.md)
if [ -z "$section" ]; then
	printf '%s\n' Unreleased
	exit 0
fi
if git show-ref --verify --quiet "refs/tags/$section"; then
	printf '%s\n' Unreleased
else
	status=$?
	[ "$status" -eq 1 ] || exit "$status"
	printf '%s\n' "$section"
fi
