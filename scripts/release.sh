#!/usr/bin/env bash
#
# release.sh — cut one coordinated release of every public module.
#
# This is the executable form of "Coordinated releases" in CONTRIBUTING.md. The
# reasoning lives there; what lives here is the part a person should not be asked to
# remember at eleven at night.
#
# The organizing fact is that a published tag is immutable. The Go module proxy and
# the checksum database keep the first thing they saw under a version forever, so a
# tag pushed in error cannot be moved, deleted, or corrected — only superseded by a
# further release that everyone must then upgrade to. Everything this script does
# before the first tag reaches the remote is undoable; nothing after it is. So the
# script is a dry run unless it is told otherwise, it verifies before it plans, and it
# prints the whole plan before it does any of it.
#
#   scripts/release.sh 0.2.0             # verify and print the plan
#   scripts/release.sh 0.2.0 --execute   # do it
#
# It never moves a tag, never force-pushes, and never writes a `replace` directive
# into a module it is about to tag.

set -euo pipefail

# Modules that carry a version tag, and the whole set including those that do not.
# From 0.1.0 the public ones are one release train: every one is tagged at every
# release with the same version, whether or not its own files changed.
PUBLIC_MODULES=(core components markdown highlight ptytest)
ALL_MODULES=(core components markdown highlight ptytest internal examples)

MODULE_PATH=github.com/Tangerg/oolong
GORELEASE=golang.org/x/exp/cmd/gorelease@v0.0.0-20260727155853-b88d891fe743

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

execute=false
version=""
for argument in "$@"; do
	case "$argument" in
	--execute) execute=true ;;
	-h | --help)
		sed -n '3,25p' "${BASH_SOURCE[0]}" | sed 's|^# \?||'
		exit 0
		;;
	-*)
		echo "unknown option: $argument" >&2
		exit 2
		;;
	*)
		if [[ -n "$version" ]]; then
			echo "give one version" >&2
			exit 2
		fi
		version="$argument"
		;;
	esac
done

if [[ -z "$version" ]]; then
	echo "usage: scripts/release.sh X.Y.Z [--execute]" >&2
	exit 2
fi
version="v${version#v}"

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
note() { printf '   %s\n' "$*"; }
die() {
	printf '\n\033[31mstopped:\033[0m %s\n' "$*" >&2
	exit 1
}

# ---------------------------------------------------------------------------
# Preflight. Every one of these has cost somebody a release.
# ---------------------------------------------------------------------------

step "Preflight"

[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
	die "$version is not a canonical version. Pre-release and metadata suffixes are not part of this train."

branch=$(git rev-parse --abbrev-ref HEAD)
[[ "$branch" == "main" ]] || die "on branch $branch. A release is cut from main."

[[ -z "$(git status --porcelain)" ]] ||
	die "the working tree has changes. gorelease refuses to read a dirty tree, and a tag must name a commit that exists."

git fetch --quiet origin --tags
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] ||
	die "main and origin/main differ. Push or pull first: a tag that names an unpushed commit is a tag nobody else can resolve."

# A tag is never moved. Finding one already there means this version was released, or
# started and abandoned; either way the answer is a new version, not a second attempt.
for module in "${PUBLIC_MODULES[@]}"; do
	tag="$module/$version"
	if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
		die "$tag already exists locally. Tags are immutable; choose the next version."
	fi
	if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
		die "$tag already exists on the remote. Tags are immutable; choose the next version."
	fi
done

grep -q "^## \[${version#v}\]" CHANGELOG.md ||
	die "CHANGELOG.md has no '## [${version#v}]' section. What changed is editorial and this script will not invent it."

# CONTRIBUTING: no release tag may contain a replace directive. A replacement resolves
# for whoever runs it and for nobody who depends on the tag.
for module in "${ALL_MODULES[@]}"; do
	if grep -qE '^\s*replace\s' "$module/go.mod"; then
		die "$module/go.mod contains a replace directive."
	fi
done

note "main is clean, current, and free of $version."

# ---------------------------------------------------------------------------
# Dependency order, derived rather than listed.
# ---------------------------------------------------------------------------

# oolong_deps prints the in-repo modules a module requires. Reading them out of go.mod
# rather than hard-coding an order means adding a module does not silently leave this
# script releasing in the wrong sequence.
oolong_deps() {
	grep "$MODULE_PATH/" "$1/go.mod" |
		grep -v '^module ' |
		sed "s|.*$MODULE_PATH/||; s|[[:space:]].*||" |
		sort -u
}

# Repeated passes rather than a clever sort: seven modules, and an order anyone can
# check by reading it.
order=()
remaining=("${ALL_MODULES[@]}")
while ((${#remaining[@]} > 0)); do
	progressed=false
	still=()
	for module in "${remaining[@]}"; do
		ready=true
		for dependency in $(oolong_deps "$module"); do
			if [[ " ${order[*]-} " != *" $dependency "* ]]; then
				ready=false
			fi
		done
		if $ready; then
			order+=("$module")
			progressed=true
		else
			still+=("$module")
		fi
	done
	remaining=("${still[@]-}")
	remaining=("${remaining[@]:-}")
	[[ -n "${remaining[0]:-}" ]] || break
	$progressed || die "the module dependency graph has a cycle among: ${remaining[*]}"
done

step "Release order"
note "${order[*]}"

# ---------------------------------------------------------------------------
# The gate. The same checks CI runs, because a release is not the place to find out.
# ---------------------------------------------------------------------------

step "Gate"

for module in "${ALL_MODULES[@]}"; do
	(cd "$module" && go build ./... && go vet ./...) || die "$module does not build or vet."
done
note "build and vet"

for module in "${ALL_MODULES[@]}"; do
	(cd "$module" && go test -race -count=1 ./... >/dev/null) || die "$module has failing tests."
done
note "tests with the race detector"

if command -v golangci-lint >/dev/null; then
	for module in "${ALL_MODULES[@]}"; do
		(cd "$module" && golangci-lint run ./... >/dev/null 2>&1) || die "$module has lint findings."
	done
	note "golangci-lint"
else
	note "golangci-lint not installed — skipped, and CI will run it"
fi

if command -v gofumpt >/dev/null; then
	[[ -z "$(gofumpt -l .)" ]] || die "gofumpt would reformat: $(gofumpt -l . | tr '\n' ' ')"
	note "gofumpt"
fi

for module in "${ALL_MODULES[@]}"; do
	[[ -z "$(cd "$module" && go fix -diff ./... 2>/dev/null)" ]] ||
		die "$module has modernizations go fix would apply."
done
note "go fix"

# The floor is only honoured if the sources that only build elsewhere still compile.
# This is where a platform-specific file quietly raises a module's declared minimum.
for goos in windows darwin linux; do
	for module in "${ALL_MODULES[@]}"; do
		(cd "$module" && GOOS="$goos" CGO_ENABLED=0 go test -run '^$' -exec /usr/bin/true ./... >/dev/null 2>&1) ||
			die "$module does not build for $goos, sources and tests included."
	done
done
note "windows, darwin and linux source sets"

# ---------------------------------------------------------------------------
# What each module is proposing, according to the tool CI uses.
# ---------------------------------------------------------------------------

step "Compatibility"

if ! command -v gorelease >/dev/null; then
	note "installing the pinned gorelease"
	go install "$GORELEASE"
fi
gorelease_bin=$(command -v gorelease || echo "$(go env GOPATH)/bin/gorelease")

for module in "${PUBLIC_MODULES[@]}"; do
	previous=$(git tag --list "$module/v*" --sort=-v:refname | head -1)
	base=none
	[[ -n "$previous" ]] && base="v${previous#*/v}"
	printf '   %-11s base=%-8s ' "$module" "$base"
	# Major zero reports every API change without pretending patch-number
	# compatibility was promised; from v1 the proposal must satisfy Go's rules.
	if [[ "$version" == v0.* ]]; then
		(cd "$module" && GOWORK=off "$gorelease_bin" -base=none -version="$version" >/dev/null) ||
			die "$module: $version is not a usable version for this module."
		suggestion=$(cd "$module" && GOWORK=off "$gorelease_bin" -base="$base" 2>&1 | grep -i 'Suggested version' || true)
		printf '%s\n' "${suggestion:-no baseline}"
	else
		(cd "$module" && GOWORK=off "$gorelease_bin" -base="$base" -version="$version" >/dev/null) ||
			die "$module: $version violates Go compatibility against $base."
		printf 'compatible with %s\n' "$base"
	fi
done

if [[ "$version" == v0.* ]]; then
	note "Pre-1.0: the suggestions above are advice. A version below one of them is a"
	note "deliberate choice, and CI will report the same thing rather than refuse it."
fi

# ---------------------------------------------------------------------------
# The plan.
# ---------------------------------------------------------------------------

# A module is bumped in the phase after every in-repo dependency it has was tagged,
# because a module's checksum needs a tag and a tag needs the commit that names it.
# That is the circularity the phases exist to break.
declare -a phase_of
phases=1
for module in "${order[@]}"; do
	phase=1
	for dependency in $(oolong_deps "$module"); do
		for index in "${!order[@]}"; do
			if [[ "${order[index]}" == "$dependency" ]]; then
				(( phase_of[index] + 1 > phase )) && phase=$((phase_of[index] + 1))
			fi
		done
	done
	phase_of+=("$phase")
	((phase > phases)) && phases=$phase
done

step "Plan for $version"
for phase in $(seq 1 "$phases"); do
	bump=()
	tag=()
	for index in "${!order[@]}"; do
		[[ "${phase_of[index]}" == "$phase" ]] || continue
		module="${order[index]}"
		[[ -n "$(oolong_deps "$module")" ]] && bump+=("$module")
		[[ " ${PUBLIC_MODULES[*]} " == *" $module "* ]] && tag+=("$module/$version")
	done
	printf '   phase %d\n' "$phase"
	[[ ${#bump[@]} -gt 0 ]] && printf '     bump   %s\n' "${bump[*]}"
	[[ ${#tag[@]} -gt 0 ]] && printf '     tag    %s\n' "${tag[*]}"
done

if ! $execute; then
	printf '\n\033[1mDry run.\033[0m Nothing was written. Re-run with --execute to release.\n'
	exit 0
fi

# ---------------------------------------------------------------------------
# Execution. From the first tag push onward this cannot be undone.
# ---------------------------------------------------------------------------

if [[ -t 0 ]]; then
	printf '\n\033[1mA pushed tag cannot be moved, deleted, or corrected.\033[0m\n'
	read -r -p "Release $version? Type the version to confirm: " confirmation
	[[ "$confirmation" == "${version#v}" || "$confirmation" == "$version" ]] ||
		die "not confirmed."
fi

for phase in $(seq 1 "$phases"); do
	step "Phase $phase"

	changed=false
	for index in "${!order[@]}"; do
		[[ "${phase_of[index]}" == "$phase" ]] || continue
		module="${order[index]}"
		for dependency in $(oolong_deps "$module"); do
			[[ " ${PUBLIC_MODULES[*]} " == *" $dependency "* ]] || continue
			note "$module -> $dependency@$version"
			(cd "$module" &&
				GOFLAGS=-mod=mod GOWORK=off GOPROXY=direct GOSUMDB=off \
					go get "$MODULE_PATH/$dependency@$version" >/dev/null 2>&1 &&
				GOWORK=off GOPROXY=direct GOSUMDB=off go mod tidy >/dev/null 2>&1) ||
				die "$module could not resolve $dependency@$version."
			changed=true
		done
	done

	if $changed; then
		# The bumped graph has to hold on its own, not only inside the workspace.
		for index in "${!order[@]}"; do
			[[ "${phase_of[index]}" == "$phase" ]] || continue
			module="${order[index]}"
			(cd "$module" && go mod tidy -diff >/dev/null 2>&1) || die "$module is not tidy after the bump."
			(cd "$module" && go test -count=1 ./... >/dev/null) || die "$module fails after the bump."
		done
		git add -A
		git commit --quiet -m "build: point phase $phase at $version

Every module this phase depends on was tagged in an earlier one, which is what lets
these ask for $version here: a module's checksum needs a tag, and a tag needs the
commit it names."
		git push --quiet origin main
		note "committed and pushed $(git rev-parse --short HEAD)"
	fi

	head=$(git rev-parse HEAD)
	pushed=()
	for index in "${!order[@]}"; do
		[[ "${phase_of[index]}" == "$phase" ]] || continue
		module="${order[index]}"
		[[ " ${PUBLIC_MODULES[*]} " == *" $module "* ]] || continue
		git tag -a "$module/$version" -m "Part of the ${version#v} release train. See CHANGELOG.md." "$head"
		pushed+=("$module/$version")
	done
	if [[ ${#pushed[@]} -gt 0 ]]; then
		git push --quiet origin "${pushed[@]}"
		note "tagged ${pushed[*]}"
	fi
done

# ---------------------------------------------------------------------------
# Verify from the proxy — and not a moment too early.
# ---------------------------------------------------------------------------

step "Publication"

# proxy.golang.org caches a negative lookup. Asking for a version before its tag is
# visible teaches it that the version does not exist, and that answer outlives the
# push. So this waits before the first question rather than after the first failure.
note "waiting for the tags to become visible before asking the proxy"
sleep 30

for attempt in 1 2 3 4 5 6; do
	missing=()
	for module in "${PUBLIC_MODULES[@]}"; do
		code=$(curl -s -o /dev/null -w '%{http_code}' \
			"https://proxy.golang.org/$MODULE_PATH/$module/@v/$version.info" || echo 000)
		[[ "$code" == "200" ]] || missing+=("$module")
	done
	if [[ ${#missing[@]} -eq 0 ]]; then
		note "all ${#PUBLIC_MODULES[@]} modules resolve at $version"
		printf '\n\033[1mReleased %s.\033[0m\n' "$version"
		exit 0
	fi
	note "still propagating: ${missing[*]} (attempt $attempt)"
	sleep 30
done

printf '\n\033[1mReleased %s.\033[0m\n' "$version"
note "The tags are pushed and correct. The proxy had not caught up with: ${missing[*]}"
note "Propagation is eventual; re-check with:"
note "  GOPROXY=https://proxy.golang.org go list -m $MODULE_PATH/<module>@$version"
