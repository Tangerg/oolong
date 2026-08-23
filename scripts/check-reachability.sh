#!/usr/bin/env bash

# Every callable path needs executable evidence. For private functions, a deadcode
# finding is ordinary dead implementation. For an exported operation in a library,
# it means the public contract lacks a caller-side test or example — never that the
# operation is unnecessary. Public API is retained or removed only after reviewing
# its responsibility and abstraction independently of repository call counts.

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

command -v deadcode >/dev/null || {
	echo "deadcode is required; install the pinned version from CONTRIBUTING.md" >&2
	exit 2
}

# noCopy.Lock and Unlock are consumed as a method set by go vet's copylocks
# analyzer. A runtime call would be fake reachability and would contradict the
# marker's contract, so remove exactly those two static-only methods from deadcode's
# runtime graph. internal/arch independently derives every public no-copy promise and
# verifies that both marker methods still exist.
actionable_findings() {
	sed -E '/: unreachable func: noCopy\.(Lock|Unlock)$/d'
}

# Keep the exception's boundary executable: changing the expression must not make an
# ordinary unreachable method disappear with the marker pair.
probe=$(
	printf '%s\n' \
		'owner.go:1: unreachable func: noCopy.Lock' \
		'owner.go:2: unreachable func: noCopy.Unlock' \
		'owner.go:3: unreachable func: owner.release' |
		actionable_findings
)
if [[ "$probe" != 'owner.go:3: unreachable func: owner.release' ]]; then
	echo "internal error: reachability exception lost its noCopy boundary" >&2
	exit 2
fi

failed=false
for goos in linux darwin windows; do
	for module in $(scripts/modules.sh); do
		findings=$(cd "$module" && GOOS="$goos" CGO_ENABLED=0 deadcode -test ./... | actionable_findings)
		if [[ -n "$findings" ]]; then
			printf '%s (%s):\n%s\n' "$module" "$goos" "$findings" >&2
			failed=true
		fi
	done
done

if $failed; then
	exit 1
fi
