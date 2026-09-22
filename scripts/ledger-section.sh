#!/bin/sh

# A published release owns its module membership; adding a module today cannot
# turn that completed release back into a candidate.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$root"
version=$(sed -n '/^## \[[0-9]/{s/^## \[\([^]]*\)\].*/\1/;p;q;}' CHANGELOG.md)
if [ -z "$version" ]; then
	printf '%s\n' Unreleased
	exit 0
fi
tags=$(git tag --list "*/v$version")
if [ -z "$tags" ]; then
	printf '%s\n' "$version"
	exit 0
fi
reference=$(printf '%s\n' "$tags" | head -n 1)
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
git archive --format=tar "$reference" >"$scratch/tree.tar"
mkdir "$scratch/tree"
tar -xf "$scratch/tree.tar" -C "$scratch/tree"
modules=$(sh "$scratch/tree/scripts/modules.sh" --public)
for module in $modules; do
	if git show-ref --verify --quiet "refs/tags/$module/v$version"; then
		continue
	else
		status=$?
		[ "$status" -eq 1 ] || exit "$status"
		printf '%s\n' "$version"
		exit 0
	fi
done
printf '%s\n' Unreleased
