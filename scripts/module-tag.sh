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

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)

directory_tag() {
	local directory="$1" path pattern tags tag candidate
	case "/$directory/" in
	*'/../'* | *'/./'* | '//'* | *'//'* | *'\\'*) die "invalid module directory: $directory" ;;
	esac
	[[ -f "$root/$directory/go.mod" ]] || die "$directory has no go.mod"
	path=$(GOWORK=off go -C "$root/$directory" list -m -f '{{.Path}}')
	if [[ "$2" != --latest ]]; then
		module_tag "$directory" "$path" "$2"
		return
	fi
	pattern=$(module_tag "$directory" "$path" --pattern)
	tags=$(git -C "$root" tag --list "$pattern" --sort=-v:refname)
	while IFS= read -r tag; do
		[[ -n "$tag" ]] || continue
		# Git globs also match prereleases and nested tags. Only the forward
		# mapping can identify a version published by this module.
		if candidate=$(module_tag "$directory" "$path" "${tag##*/}" 2>/dev/null) && [[ "$candidate" == "$tag" ]]; then
			printf '%s\n' "$tag"
			return
		fi
	done <<<"$tags"
	return 0
}

# A major-version directory and its parent may share a tag prefix. The public
# module whose forward mapping produces the exact tag owns its release.
resolve_tag() {
	local tag="$1" directories directory pattern candidate selected=
	directories=$("$root/scripts/modules.sh" --public)
	while IFS= read -r directory; do
		pattern=$(directory_tag "$directory" --pattern)
		[[ "$tag" == $pattern ]] || continue
		candidate=$(directory_tag "$directory" "${tag##*/}")
		[[ "$candidate" == "$tag" ]] || continue
		[[ -z "$selected" ]] || die "ambiguous public module tag: $tag"
		selected="$directory"
	done <<<"$directories"
	[[ -n "$selected" ]] || die "no public module owns tag: $tag"
	printf '%s\n' "$selected"
}

[[ $# == 2 ]] || die "usage: scripts/module-tag.sh MODULE vX.Y.Z | --pattern MODULE | --latest MODULE | --resolve TAG"
case "$1" in
--resolve) resolve_tag "$2" ;;
--pattern | --latest) directory_tag "$2" "$1" ;;
*) directory_tag "$1" "$2" ;;
esac
