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
if [ ! -f go.mod ]; then
	echo "replace-local-modules.sh: run this command from a module root" >&2
	exit 2
fi
current=$(GOWORK=off go list -m -f '{{.Path}}')

if ! public_modules=$("$root/scripts/modules.sh" --public); then
	echo "replace-local-modules.sh: cannot derive public modules from $root" >&2
	exit 2
fi

set --
for directory in $public_modules; do
	replacement=$(GOWORK=off go -C "$root/$directory" list -m -f '{{.Path}}={{.Dir}}')
	if [ "${replacement%%=*}" = "$current" ]; then
		continue
	fi
	set -- "$@" "-replace=$replacement"
done

GOWORK=off go mod edit "$@"
