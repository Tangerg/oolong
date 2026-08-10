#!/bin/sh

# Print the repository's module directories, one per line. go.work is the single
# source for membership; --public removes modules that are tested but not tagged.

set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

case "${1-}" in
"") ;;
--public) ;;
*)
	echo "usage: scripts/modules.sh [--public]" >&2
	exit 2
	;;
esac

modules=$(
	sed -n '/^use (/,/^)/p' "$root/go.work" |
		sed -n 's|^[[:space:]]*\./||p'
)

if [ "${1-}" = "--public" ]; then
	printf '%s\n' "$modules" | while IFS= read -r module; do
		case "$module" in
		internal | examples) ;;
		*) printf '%s\n' "$module" ;;
		esac
	done
	 exit 0
fi

printf '%s\n' "$modules"
