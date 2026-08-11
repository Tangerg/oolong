#!/usr/bin/env bash

# Require every incompatible exported change since the preceding immutable module tag
# to appear by its exact API name in CHANGELOG's Unreleased migration ledger. deadcode
# can say that an operation lacks repository evidence; it cannot decide whether a
# library should remove or reshape that operation. This independent gate makes every
# source break a visible API decision without confusing reachability with
# responsibility.

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

apidiff_bin=${APIDIFF:-apidiff}
section_name=${CHANGELOG_SECTION:-Unreleased}
command -v "$apidiff_bin" >/dev/null || {
	echo "apidiff is required; install the pinned version from CONTRIBUTING.md" >&2
	exit 2
}

scratch=$(mktemp -d "${TMPDIR:-/tmp}/oolong-api-changelog.XXXXXX")
cleanup() { rm -rf -- "$scratch"; }
trap cleanup EXIT HUP INT TERM

write_export() {
	local directory=$1 output=$2 module_path=$3 log="$2.log"
	if ! (cd "$directory" && "$apidiff_bin" -m -w "$output" "$module_path" >/dev/null 2>"$log"); then
		cat "$log" >&2
		return 1
	fi
}

incompatible_symbols() {
	sed -n -E '/^Incompatible changes:$/,/^Compatible changes:$/ {
		s/^- (\.\/)?([^:]+):.*$/\2/p
	}'
}

# Keep the parser's scope executable. apidiff reports additions below the compatible
# heading; they must not create migration work. Every entry above it must survive,
# including less common breaks such as adding a method to a public interface.
parser_probe=$(printf '%s\n' \
	'Incompatible changes:' \
	'- ./pkg.Removed: removed' \
	'- ./pkg.Changed: changed from func() to func(int)' \
	'- ./pkg.Interface.Method: added' \
	'Compatible changes:' \
	'- ./pkg.Added: added' |
	incompatible_symbols)
[[ "$parser_probe" == $'pkg.Removed\npkg.Changed\npkg.Interface.Method' ]] || {
	echo "internal error: apidiff incompatibility parser lost its contract" >&2
	exit 2
}

awk -v heading="## [$section_name]" '
	$0 == heading { inside = 1; next }
	inside && /^## / { exit }
	inside { print }
' CHANGELOG.md >"$scratch/changes.md"

failed=false
for module in $(scripts/modules.sh --public); do
	previous=$(git tag --list "$module/v*" --sort=-v:refname | head -n 1)
	# A module with no published API baseline cannot have a published break.
	[[ -n "$previous" ]] || continue

	version=${previous#*/}
	module_path=$(sed -n 's/^module[[:space:]]*//p' "$module/go.mod")
	GOWORK=off go mod download "$module_path@$version"
	old_dir=$(GOWORK=off go list -m -f '{{.Dir}}' "$module_path@$version")

	breaking_file="$scratch/$module-breaking"
	: >"$breaking_file"
	for goos in linux darwin windows; do
		old_export="$scratch/$module-$goos-old.api"
		new_export="$scratch/$module-$goos-new.api"
		GOOS="$goos" CGO_ENABLED=0 write_export "$old_dir" "$old_export" "$module_path"
		GOOS="$goos" CGO_ENABLED=0 write_export "$module" "$new_export" "$module_path"

		diff_log="$scratch/$module-$goos-diff.log"
		if ! changes=$("$apidiff_bin" -m "$old_export" "$new_export" 2>"$diff_log"); then
			cat "$diff_log" >&2
			exit 1
		fi
		printf '%s\n' "$changes" | incompatible_symbols >>"$breaking_file"
	done
	breaking=$(sort -u "$breaking_file")
	[[ -n "$breaking" ]] || continue

	section=$(awk -v heading="#### $module" '
		$0 == heading { found = 1; inside = 1; next }
		inside && /^#### / { exit }
		inside { print }
		END { if (!found) exit 1 }
	' "$scratch/changes.md") || {
		echo "CHANGELOG.md has incompatible API changes from $module/$version but no '#### $module' ledger in [$section_name]" >&2
		failed=true
		continue
	}

	while IFS= read -r symbol; do
		[[ -n "$symbol" ]] || continue
		needle="\`$symbol\`"
		if [[ "$section" != *"$needle"* ]]; then
			echo "CHANGELOG.md does not name incompatible API $module/$symbol (baseline $previous)" >&2
			failed=true
		fi
	done <<<"$breaking"
done

if $failed; then
	exit 1
fi
