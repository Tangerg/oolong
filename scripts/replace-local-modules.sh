#!/bin/sh

# Point every declared public Oolong dependency at one local checkout. Module
# membership and paths come from go.work through modules.sh; callers only choose
# the checkout, so adding a module cannot leave a second replacement list stale.

set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: scripts/replace-local-modules.sh CHECKOUT" >&2
	exit 2
fi

root=$(CDPATH= cd -- "$1" && pwd)
current=$(sed -n 's/^module[[:space:]]*//p' go.mod)

set --
for directory in $("$root/scripts/modules.sh" --public); do
	dependency=$(sed -n 's/^module[[:space:]]*//p' "$root/$directory/go.mod")
	if [ "$dependency" = "$current" ]; then
		continue
	fi
	set -- "$@" "-replace=$dependency=$root/$directory"
done

GOWORK=off go mod edit "$@"
