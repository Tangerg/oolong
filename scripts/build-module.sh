#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: scripts/build-module.sh MODULE" >&2
	exit 2
fi

# Go's directory output selects only commands. The host's null device checks
# every package without letting a command overwrite a same-named source directory.
build_host=$(go env GOHOSTOS)
case "$build_host" in
windows) build_output=NUL ;;
*) build_output=/dev/null ;;
esac

go -C "$1" build -o "$build_output" ./...
