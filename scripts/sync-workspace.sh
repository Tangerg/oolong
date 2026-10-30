#!/bin/sh

set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
case "${1-}" in
"") set -- ;;
--check) set -- -check ;;
*)
	echo "usage: scripts/sync-workspace.sh [--check]" >&2
	exit 2
	;;
esac
GOWORK=off go run "$root/ui/internal/cmd/workspace/main.go" -root "$root" "$@"
