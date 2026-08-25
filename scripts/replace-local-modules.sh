#!/bin/sh

# Point every declared public Oolong dependency at one local checkout. Module
# membership and paths come from go.work through modules.sh; callers only choose
# the checkout, so adding a module cannot leave a second replacement list stale.

set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: scripts/replace-local-modules.sh CHECKOUT" >&2
	exit 2
fi

root=$(CDPATH= cd -- "$1" && pwd -P)
current=$(sed -n 's/^module[[:space:]]*//p' go.mod)
if [ -z "$current" ]; then
	echo "replace-local-modules.sh: the current go.mod has no module path" >&2
	exit 2
fi

if ! public_modules=$("$root/scripts/modules.sh" --public); then
	echo "replace-local-modules.sh: cannot derive public modules from $root" >&2
	exit 2
fi

set --
for directory in $public_modules; do
	dependency=$(sed -n 's/^module[[:space:]]*//p' "$root/$directory/go.mod")
	if [ -z "$dependency" ]; then
		echo "replace-local-modules.sh: $directory/go.mod has no module path" >&2
		exit 2
	fi
	if [ "$dependency" = "$current" ]; then
		continue
	fi
	set -- "$@" "-replace=$dependency=$root/$directory"
done

GOWORK=off go mod edit "$@"
