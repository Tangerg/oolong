#!/usr/bin/env bash

# The module manifest and physical directory jointly determine Go's immutable
# tag namespace. Release and API comparison consume this one derivation.
set -euo pipefail

die() {
	printf 'module-tag: %s\n' "$*" >&2
	exit 1
}

module_tag() {
	local directory="$1" path="$2" proposed="$3" major=1 prefix="$1" stem="$2" requested
	if [[ "$path" =~ /v([0-9]+)$ ]]; then
		major="${BASH_REMATCH[1]}"
		[[ "$major" =~ ^([2-9]|[1-9][0-9]+)$ ]] || die "invalid semantic import suffix: $path"
		stem="${path%/v"$major"}"
		if [[ "$prefix" == */v"$major" ]]; then prefix="${prefix%/v"$major"}"; fi
	fi
	[[ -n "$prefix" && "$stem" == */"$prefix" ]] || die "module path and physical directory disagree: $directory, $path"
	if [[ "$proposed" == --pattern ]]; then
		if [[ "$major" == 1 ]]; then printf '%s/v[01].*\n' "$prefix"; else printf '%s/v%s.*\n' "$prefix" "$major"; fi
		return
	fi
	[[ "$proposed" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die "non-canonical version: $proposed"
	requested="${BASH_REMATCH[1]}"
	if [[ "$major" == 1 ]]; then
		[[ "$requested" == 0 || "$requested" == 1 ]] || die "$path needs the /v$requested semantic import suffix"
	else
		[[ "$requested" == "$major" ]] || die "$path cannot publish $proposed"
	fi
	printf '%s/%s\n' "$prefix" "$proposed"
}

[[ $# == 2 ]] || die "usage: scripts/module-tag.sh MODULE vX.Y.Z | --pattern MODULE"
if [[ "$1" == --pattern ]]; then
	directory="$2"
	proposed=--pattern
else
	directory="$1"
	proposed="$2"
fi
case "/$directory/" in
*'/../'* | *'/./'* | '//'* | *'//'* | *'\\'*) die "invalid module directory: $directory" ;;
esac
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
path=$(sed -n 's/^module[[:space:]]*//p' "$root/$directory/go.mod")
[[ -n "$path" ]] || die "$directory/go.mod has no module path"
module_tag "$directory" "$path" "$proposed"
