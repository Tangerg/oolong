#!/usr/bin/env bash

set -euo pipefail

die() {
	printf 'tools: %s\n' "$*" >&2
	exit 1
}

# A module version owns every command built from that module. CI installation and
# release admission consume these same pins instead of maintaining parallel lists.
pins() {
	local module version commands suffix package
	while read -r module version commands; do
		for suffix in $commands; do
			package="$module"
			[[ "$suffix" == . ]] || package+="/$suffix"
			printf '%s\t%s\t%s\t%s\n' "${package##*/}" "$module" "$version" "$package"
		done
	done <<'EOF'
github.com/golangci/golangci-lint/v2 v2.14.0 cmd/golangci-lint
golang.org/x/tools v0.50.0 cmd/deadcode
golang.org/x/exp v0.0.0-20261007192929-f45ad48fbe92 cmd/apidiff cmd/gorelease
golang.org/x/vuln v1.8.0 cmd/govulncheck
mvdan.cc/gofumpt v0.11.1-0.20260820074422-a2bc6805583d .
mvdan.cc/sh/v3 v3.13.1 cmd/shfmt
EOF
}

selected_pins() {
	local pin_output requested name module version package found
	pin_output=$(pins) || die "cannot read the tool pins"
	if [[ $# == 0 ]]; then
		printf '%s\n' "$pin_output"
		return
	fi
	for requested in "$@"; do
		found=false
		while read -r name module version package; do
			[[ "$name" == "$requested" ]] || continue
			printf '%s\t%s\t%s\t%s\n' "$name" "$module" "$version" "$package"
			found=true
		done <<<"$pin_output"
		$found || die "unknown pinned tool: $requested"
	done
}

check_binary() {
	local name="$1" module="$2" version="$3" package="$4" binary metadata
	binary=$(type -p "$name") || die "$name is missing; run scripts/tools.sh install $name"
	[[ -n "$binary" ]] || die "$name must resolve to an executable file"
	# MSYS can omit .exe from a command path; native Go opens the literal filename.
	# File identity prevents substituting a different executable beside a script.
	if [[ "$binary" -ef "$binary.exe" ]]; then
		binary+=".exe"
	fi
	case "$OSTYPE" in
	msys* | cygwin*) binary=$(cygpath -am "$binary") || die "cannot resolve the native path for $name" ;;
	esac
	metadata=$(go version -m "$binary") || die "$name has no readable Go build identity: $binary"
	if ! awk -v package="$package" -v module="$module" -v version="$version" '
		$1 == "path" && $2 == package { matched_path = 1 }
		$1 == "mod" && $2 == module && $3 == version { matched_version = 1 }
		$1 == "=>" { replaced = 1 }
		END { exit !(matched_path && matched_version && !replaced) }
	' <<<"$metadata"; then
		die "$name must be $package@$version without replacements; run scripts/tools.sh install $name"
	fi
}

action="${1-}"
case "$action" in
list | install | check) shift ;;
*) die "usage: scripts/tools.sh {list|install|check} [TOOL...]" ;;
esac

selection=$(selected_pins "$@")
while read -r name module version package; do
	case "$action" in
	list) printf '%s\t%s\t%s\t%s\n' "$name" "$module" "$version" "$package" ;;
	install) go install "$package@$version" ;;
	check) check_binary "$name" "$module" "$version" "$package" ;;
	esac
done <<<"$selection"
