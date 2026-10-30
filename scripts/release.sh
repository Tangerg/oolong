#!/usr/bin/env bash
# Release exactly one distribution module. Dependency requirements are reviewed
# and committed before this script runs; no module is bumped as a side effect.
# A dry run checks the same graph and compatibility as an executed release.
# Usage: scripts/release.sh MODULE X.Y.Z [--execute]

set -euo pipefail

MODULE_PATH=github.com/Tangerg/oolong
export GONOPROXY="$MODULE_PATH/*" GONOSUMDB="$MODULE_PATH/*"

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$root"

step() { printf '\n== %s\n' "$*"; }
note() { printf '   %s\n' "$*"; }
die() {
	printf 'release: %s\n' "$*" >&2
	exit 1
}

compatibility() {
	local module="$1" pattern="$2" previous base report status
	if ! previous=$(git tag --list "$pattern" --sort=-v:refname | sed -n '1p'); then
		die "released tags cannot be inspected for $module."
	fi
	base=none
	[[ -n "$previous" ]] && base="${previous##*/}"
	printf '   %-11s base=%-8s ' "$module" "$base"
	# Major zero reports every API change without pretending patch-number
	# compatibility was promised; from v1 the proposal must satisfy Go's rules.
	if [[ "$version" == v0.* ]]; then
		(cd "$module" && GOWORK=off "$gorelease_bin" -base=none -version="$version" >/dev/null) ||
			die "$module: $version is not a usable version for this module."
		if report=$(cd "$module" && GOWORK=off "$gorelease_bin" -base="$base" 2>&1); then
			printf '%s\n' "$report"
		else
			status=$?
			printf '%s\n' "$report" >&2
			die "$module: compatibility comparison failed (status $status)."
		fi
	else
		(cd "$module" && GOWORK=off "$gorelease_bin" -base="$base" -version="$version" >/dev/null) ||
			die "$module: $version violates Go compatibility against $base."
		printf 'compatible with %s\n' "$base"
	fi
}

# An immutable tag is the only write this command owns. Failure never causes a
# tag deletion, movement, force push, dependency edit or unrelated branch push.
publish() {
	git tag -a "$tag" -m "$module $version. See CHANGELOG.md." "$head"
	git push --quiet origin "refs/tags/$tag"
	local remote
	if ! remote=$(git ls-remote --tags origin "refs/tags/$tag^{}" | cut -f1); then
		die "cannot verify $tag on origin"
	fi
	[[ "$remote" == "$head" ]] || die "$tag does not point to $head on origin; inspect before any further release"
	note "published $tag at $head"
}

if [[ "${1-}" == --help || "${1-}" == -h ]]; then
	echo "usage: scripts/release.sh MODULE X.Y.Z [--execute]"
	exit 0
fi
[[ $# -ge 2 && $# -le 3 ]] || die "usage: scripts/release.sh MODULE X.Y.Z [--execute]"
module="$1"
version="v${2#v}"
execute=false
if [[ $# -eq 3 ]]; then
	[[ "$3" == --execute ]] || die "unknown option: $3"
	execute=true
fi

if ! all_module_output=$(scripts/modules.sh); then die "cannot derive workspace modules"; fi
ALL_MODULES=()
while IFS= read -r directory; do ALL_MODULES+=("$directory"); done <<<"$all_module_output"
if ! public_module_output=$(scripts/modules.sh --public); then die "cannot derive public modules"; fi
public=false
while IFS= read -r directory; do
	[[ "$directory" != "$module" ]] || public=true
done <<<"$public_module_output"
$public || die "$module is not a public workspace module"
tag=$(scripts/module-tag.sh "$module" "$version")

step "Preflight $tag"
branch=$(git rev-parse --abbrev-ref HEAD)
[[ "$branch" == main ]] || die "release from main, not $branch"
worktree_status=$(git status --porcelain)
[[ -z "$worktree_status" ]] || die "commit reviewed changes before releasing"
git fetch --quiet origin --tags
head=$(git rev-parse HEAD)
upstream=$(git rev-parse origin/main)
[[ "$head" == "$upstream" ]] || die "main and origin/main differ"
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
	die "$tag already exists; tags are immutable"
else
	status=$?
	[[ "$status" == 1 ]] || die "cannot inspect local tags"
fi
if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
	die "$tag already exists on origin"
else
	status=$?
	[[ "$status" == 2 ]] || die "cannot inspect remote tags"
fi
grep -Fq "## [$tag]" CHANGELOG.md || die "CHANGELOG.md needs a [$tag] section"
for directory in "${ALL_MODULES[@]}"; do
	if grep -qE '^[[:space:]]*replace[[:space:]]' "$directory/go.mod"; then
		die "$directory/go.mod contains a local replacement"
	else
		status=$?
		[[ "$status" == 1 ]] || die "cannot inspect $directory/go.mod"
	fi
done
scripts/sync-workspace.sh --check

step "Gate"

for tool in apidiff deadcode golangci-lint gofumpt govulncheck npx shfmt; do
	command -v "$tool" >/dev/null || die "$tool is required for a release. Install the version in CONTRIBUTING.md."
done

for directory in "${ALL_MODULES[@]}"; do
	(cd "$directory" && go build ./... && go vet ./...) || die "$directory does not build or vet."
done
note "build and vet"

for directory in "${ALL_MODULES[@]}"; do
	(cd "$directory" && go test -race -count=1 ./... >/dev/null) || die "$directory has failing tests."
done
note "tests with the race detector"

for directory in "${ALL_MODULES[@]}"; do
	(cd "$directory" && golangci-lint run ./... >/dev/null 2>&1) || die "$directory has lint findings."
done
note "golangci-lint"

scripts/check-reachability.sh || die "callable code lacks executable coverage or private code is unreachable."
note "callable code reachability"

go -C ui run ./internal/cmd/apiledger -root .. -section "$tag" ||
	die "an incompatible exported API change lacks an exact $version migration entry."
note "breaking API migration ledger"

if ! unformatted=$(gofumpt -l .); then
	die "gofumpt could not inspect the repository."
fi
[[ -z "$unformatted" ]] || die "gofumpt would reformat: ${unformatted//$'\n'/ }"
note "gofumpt"

shfmt -d scripts || die "shfmt would reformat repository scripts."
note "shfmt"

for directory in "${ALL_MODULES[@]}"; do
	(cd "$directory" && govulncheck ./... >/dev/null) || die "$directory has a reachable vulnerability."
done
note "govulncheck"

npm ci >/dev/null || die "documentation dependencies do not install from package-lock.json."
npm run docs:check >/dev/null || die "documentation audit, lint, or build failed."
note "documentation"

for directory in "${ALL_MODULES[@]}"; do
	if ! fixes=$(cd "$directory" && go fix -diff ./...); then
		die "$directory could not be inspected by go fix."
	fi
	[[ -z "$fixes" ]] ||
		die "$directory has modernizations go fix would apply."
done
note "go fix"

# The floor is only honoured if the sources that only build elsewhere still compile.
# This is where a platform-specific file quietly raises a module's declared minimum.
for goos in windows darwin linux; do
	for directory in "${ALL_MODULES[@]}"; do
		(cd "$directory" && GOOS="$goos" CGO_ENABLED=0 go test -run '^$' -exec /usr/bin/true ./... >/dev/null 2>&1) ||
			die "$directory does not build for $goos, sources and tests included."
	done
done
note "windows, darwin and linux source sets"

# WebAssembly is a library source-portability contract, not a claim that Oolong
# owns a browser or WASI terminal adapter. Vet includes target-specific tests and
# build proves every package can be consumed by an adapter supplied downstream.
while read -r goos goarch; do
	for directory in "${ALL_MODULES[@]}"; do
		(cd "$directory" && GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go vet ./... &&
			GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build ./...) ||
			die "$directory does not build for $goos/$goarch."
	done
done <<'EOF'
wasip1 wasm
js wasm
EOF
note "WASI and JavaScript WebAssembly source sets"

step "Published dependency graph"
(cd "$module" && GOWORK=off go mod tidy -diff && GOWORK=off go test -count=1 ./...) ||
	die "$module is not independently consumable with its declared dependency versions"
command -v gorelease >/dev/null || die "install the pinned gorelease version from CONTRIBUTING.md"
gorelease_bin=$(command -v gorelease)
tag_pattern=$(scripts/module-tag.sh --pattern "$module")
compatibility "$module" "$tag_pattern"
step "Plan"
note "tag $tag at $head; no dependency or other module version changes"
if ! $execute; then
	note "Dry run complete. Re-run with --execute to publish this one tag."
	exit 0
fi
publish
