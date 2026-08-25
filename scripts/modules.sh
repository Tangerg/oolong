#!/bin/sh

# Print the repository's module directories, one per line. go.work is the single
# source for membership. --public selects modules that contain an externally
# importable production package; command-only, test-only and internal-only modules
# are tested but not tagged.

set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)

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
if [ -z "$modules" ]; then
	echo "modules.sh: go.work declares no modules" >&2
	exit 2
fi
seen_modules=
for module in $modules; do
	case " $seen_modules " in
	*" $module "*)
		echo "modules.sh: go.work declares $module more than once" >&2
		exit 2
		;;
	esac
	if [ ! -f "$root/$module/go.mod" ]; then
		echo "modules.sh: $module has no go.mod" >&2
		exit 2
	fi
	seen_modules="${seen_modules}${seen_modules:+ }$module"
done

public_module() {
	module=$1
	public=false
	for goos in linux darwin windows; do
		if ! packages=$(
			GOWORK="$root/go.work" GOOS=$goos GOARCH=amd64 CGO_ENABLED=0 \
				go -C "$root/$module" list \
				-f '{{if and (ne .Name "main") (or .GoFiles .CgoFiles)}}{{.ImportPath}}{{end}}' ./...
		); then
			echo "modules.sh: cannot inspect $module for $goos" >&2
			return 2
		fi
		for package in $packages; do
			case "$package" in
			*/internal | */internal/*) ;;
			*) public=true ;;
			esac
		done
	done
	if $public; then
		return 0
	fi
	return 1
}

if [ "${1-}" = "--public" ]; then
	public_modules=
	for module in $seen_modules; do
		if public_module "$module"; then
			public_modules="${public_modules}${public_modules:+ }$module"
		else
			status=$?
			if [ "$status" -ne 1 ]; then
				exit "$status"
			fi
		fi
	done
	if [ -z "$public_modules" ]; then
		echo "modules.sh: no public modules were derived from go.work" >&2
		exit 2
	fi
	for module in $public_modules; do
		printf '%s\n' "$module"
	done
	exit 0
fi

for module in $seen_modules; do
	printf '%s\n' "$module"
done
