#!/bin/sh

# Each module's most recent release heading owns its prepared migration entries
# until that exact tag exists. A peer's release never selects this ledger.
set -eu
[ "$#" -eq 1 ] || {
	echo "usage: scripts/ledger-section.sh MODULE" >&2
	exit 2
}
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$root"
pattern=$(scripts/module-tag.sh --pattern "$1")
sections=$(sed -n 's/^## \[\([^]]*\)\].*/\1/p' CHANGELOG.md)
while IFS= read -r section; do
	case "$section" in
	$pattern) ;;
	*) continue ;;
	esac
	canonical=$(scripts/module-tag.sh "$1" "${section##*/}")
	[ "$section" = "$canonical" ] || {
		echo "invalid module release section: $section" >&2
		exit 2
	}
	if git show-ref --verify --quiet "refs/tags/$section"; then
		break
	else
		status=$?
		[ "$status" -eq 1 ] || exit "$status"
		printf '%s\n' "$section"
		exit 0
	fi
done <<EOF
$sections
EOF
printf '%s\n' Unreleased
