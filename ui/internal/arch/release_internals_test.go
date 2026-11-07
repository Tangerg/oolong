package arch

import (
	"archive/zip"
	"bytes"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func scriptFunction(t *testing.T, file, name string) string {
	t.Helper()
	source, err := readRepositoryFile(filepath.Join(repoRoot(t), "scripts", file))
	if err != nil {
		t.Fatal(err)
	}
	_, tail, found := strings.Cut(string(source), "\n"+name+"() {\n")
	if !found {
		t.Fatalf("release script has no %s boundary", name)
	}
	body, _, found := strings.Cut(tail, "\n}\n")
	if !found {
		t.Fatalf("unterminated %s", name)
	}
	return name + "() {\n" + body + "\n}\n"
}

func runReleaseFragment(t *testing.T, dir, script string) (string, error) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	command := exec.CommandContext(t.Context(), bash, "-c", "set -euo pipefail\ndie() { echo \"$*\" >&2; exit 1; }\n"+script) //nolint:gosec // G204: repository-authored shell fragments run only in an isolated fixture.
	command.Dir = dir
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestIndependentReleaseTagsMatchSemanticImports(t *testing.T) {
	function := scriptFunction(t, "module-tag.sh", "module_tag")
	for _, test := range []struct{ directory, path, version, want string }{
		{"ui", "ui", "v0.1.0", "ui/v0.1.0"},
		{"extensions/markdown", "extensions/markdown", "v1.2.3", "extensions/markdown/v1.2.3"},
		{"ui", "ui/v2", "v2.0.0", "ui/v2.0.0"},
		{"ui/v2", "ui/v2", "v2.1.0", "ui/v2.1.0"},
		{"ui", "ui", "v2.0.0", ""},
		{"ui", "ui/v2", "v1.0.0", ""},
		{"unrelated", "ui", "v0.1.0", ""},
		{"ui", "ui", "v01.0.0", ""},
		{"ui", "ui", "--pattern", "ui/v[01].*"},
		{"ui/v2", "ui/v2", "--pattern", "ui/v2.*"},
		{"ui", "ui/v2", "--pattern", "ui/v2.*"},
		{"ui", "ui/v02", "--pattern", ""},
	} {
		t.Run(test.directory+"/"+test.path+"/"+test.version, func(t *testing.T) {
			script := function + fmt.Sprintf("module_tag %q %q %q\n", test.directory, "example.invalid/oolong/"+test.path, test.version)
			output, err := runReleaseFragment(t, t.TempDir(), script)
			if test.want == "" {
				if err == nil {
					t.Fatal("invalid release tag accepted")
				}
				return
			}
			if err != nil || strings.TrimSpace(output) != test.want {
				t.Fatalf("tag = %q, %v; want %q", output, err, test.want)
			}
		})
	}
}

func TestTagContractSelectsTheDirectoryWhoseManifestOwnsTheTag(t *testing.T) {
	root := t.TempDir()
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0o700); err != nil {
		t.Fatal(err)
	}
	tool, err := readRepositoryFile(filepath.Join(repoRoot(t), "scripts", "module-tag.sh"))
	if err != nil {
		t.Fatal(err)
	}
	const inventory = "#!/bin/sh\nset -eu\ntest \"$1\" = --public\nprintf '%s\\n' ui ui/v2 extensions/view\n"
	const inspector = "#!/bin/sh\nset -eu\nmodule=$(GOWORK=off go list -m -f '{{.Path}}')\nprintf '%s|%s\\n' \"$module\" \"$*\"\n"
	for name, script := range map[string]string{"module-tag.sh": string(tool), "modules.sh": inventory, "gorelease": inspector} {
		//nolint:gosec // G306: these three executable scripts run only in the isolated fixture.
		if err = os.WriteFile(filepath.Join(scripts, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, directory := range []string{"ui", "ui/v2", "extensions/view"} {
		physical := filepath.Join(root, filepath.FromSlash(directory))
		if err = os.MkdirAll(physical, 0o700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(physical, "go.mod"), []byte("module \"example.invalid/oolong/"+directory+"\" // a valid Go manifest\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workflow, err := readRepositoryFile(filepath.Join(repoRoot(t), ".github", "workflows", "release-contract.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_, body, found := strings.Cut(string(workflow), "        run: |\n")
	if !found {
		t.Fatal("release validation workflow has no shell step")
	}
	proxy := t.TempDir()
	const prefix = "example.invalid/oolong/"
	for _, tc := range []struct{ module, version, code string }{
		{"ui", "v0.2.0", "package ui"},
		{"ui/v2", "v2.1.0", "package ui"},
		{"extensions/view", "v1.2.3", "package view"},
		{"ui", "v0.3.0", "package ui\nvar Broken = undefined"},
	} {
		publishFixtureModule(t, proxy, prefix+tc.module, tc.version, tc.code)
	}
	cache := t.TempDir()
	proxyURL := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(proxy), "/")}
	for _, tc := range []struct{ tag, module, version, want string }{
		{tag: "ui/v0.2.0", want: "Built " + prefix + "ui@v0.2.0"},
		{tag: "ui/v2.1.0", want: "Built " + prefix + "ui/v2@v2.1.0"},
		{tag: "extensions/view/v1.2.3", want: "Built " + prefix + "extensions/view@v1.2.3"},
		{module: "ui/v2", version: "v2.0.1", want: prefix + "ui/v2|-base=none -version=v2.0.1"},
		{tag: "ui/v0.3.0"}, // The module exists but the bytes served to a consumer cannot build.
		{tag: "ui/v0.4.0"}, // Only the checkout exists.
		{tag: "ui/v3.0.0"},
		{tag: "ui/v2.01.0"},
		{tag: "examples/v0.1.0"},
	} {
		event := "workflow_dispatch"
		if tc.tag != "" {
			event = "push"
		}
		cmd := exec.CommandContext(t.Context(), "bash", "-c", body) //nolint:gosec // The fixture only builds modules from its isolated file proxy.
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+scripts+string(os.PathListSeparator)+os.Getenv("PATH"),
			"GITHUB_EVENT_NAME="+event, "GITHUB_REF_TYPE=tag", "GITHUB_REF_NAME="+tc.tag, "INPUT_MODULE="+tc.module, "INPUT_VERSION="+tc.version,
			"GOWORK=off", "GOFLAGS=-modcacherw", "GOPROXY="+proxyURL.String(), "GONOPROXY=none", "GOSUMDB=off", "GOMODCACHE="+cache, "RUNNER_TEMP="+t.TempDir())
		out, runErr := cmd.CombinedOutput()
		if tc.want == "" {
			if runErr == nil {
				t.Errorf("accepted invalid release %q: %s", tc.tag, out)
			}
		} else if runErr != nil || !strings.Contains(string(out), tc.want) {
			t.Errorf("contract for tag %q, module %q: %q, %v; want %s", tc.tag, tc.module, out, runErr, tc.want)
		}
	}
}

func publishFixtureModule(t *testing.T, proxy, module, version, source string) {
	t.Helper()
	dir := filepath.Join(proxy, filepath.FromSlash(module), "@v")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mod := "module " + module + "\n\ngo 1.27.0\n"
	info := fmt.Sprintf(`{"Version":%q,"Time":"2020-01-01T00:00:00Z"}`, version)
	for ext, data := range map[string]string{".mod": mod, ".info": info} {
		if err := os.WriteFile(filepath.Join(dir, version+ext), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, data := range map[string]string{"go.mod": mod, "api.go": source} {
		file, err := writer.Create(module + "@" + version + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, version+".zip"), archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseGatesKeepTheSelectedModule(t *testing.T) {
	testReleaseGatesKeepTheSelectedModule(t, t.TempDir())
}

func TestReleaseGatesKeepTheSelectedModuleThroughSymlink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "repository")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("cannot create repository symlink: %v", err)
	}
	testReleaseGatesKeepTheSelectedModule(t, link)
}

func testReleaseGatesKeepTheSelectedModule(t *testing.T, root string) {
	t.Helper()
	const selectedModule = "example.invalid/oolong/ui"
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // G306,G703: executable test tools and scripts live only in the isolated fixture.
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"release.sh", "module-tag.sh", "build-module.sh", "tools.sh"} {
		body, err := readRepositoryFile(filepath.Join(repoRoot(t), "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		write("scripts/"+name, string(body))
	}
	write("scripts/modules.sh", "#!/bin/sh\nif [ \"${1-}\" = --public ]; then echo ui; else printf 'ui\\nexamples\\n'; fi\n")
	for _, name := range []string{"sync-workspace.sh", "check-reachability.sh"} {
		write("scripts/"+name, "#!/bin/sh\nexit 0\n")
	}
	write("ui/go.mod", "module "+selectedModule+"\ngo 1.27.0\n")
	write("examples/go.mod", "module example.invalid/oolong/examples\ngo 1.27.0\n")
	write("CHANGELOG.md", "## [ui/v0.1.0]\n")
	for _, name := range []string{"apidiff", "deadcode", "golangci-lint", "gofumpt", "govulncheck", "npx", "shfmt", "npm"} {
		write("bin/"+name, "#!/bin/sh\nprintf '%s|%s|%s\\n' \"${0##*/}\" \"$PWD\" \"$*\" >> \"$TRACE\"\n")
	}
	writeToolBuildInfo(t, filepath.Join(root, "bin"))
	write("bin/go", `#!/bin/sh
printf 'go|%s|%s\n' "$PWD" "$*" >> "$TRACE"
if [ "$1 $2" = 'version -m' ]; then
 cat "$3.buildinfo"
elif [ "$1" = -C ] && [ "$3" = mod ]; then
 exec "$FIXTURE_GO" "$@"
elif [ "$1" = -C ] && [ "$3" = list ]; then
 awk '$1 == "module" {print $2}' "$2/go.mod"
fi
`)
	// Module identity survives MSYS path spelling and symlinked temporary roots.
	write("bin/gorelease", `#!/bin/sh
set -eu
module=$(awk '$1 == "module" { print $2; exit }' go.mod)
printf 'gorelease|%s|%s\n' "$module" "$*" >> "$TRACE"
`)
	write("bin/git", `#!/bin/sh
printf 'git|%s|%s\n' "$PWD" "$*" >> "$TRACE"
case "$1" in
 rev-parse)
  case "$2" in
   --abbrev-ref) echo main ;;
   -q) exit 1 ;;
   *) echo reviewed-commit ;;
  esac ;;
 ls-remote)
  if [ "$2" = --exit-code ]; then exit 2; fi
  printf 'reviewed-commit\trefs/tags/ui/v0.1.0^{}\n' ;;
 worktree)
  case "$2" in
   add) cp -R "$PWD/." "$5/"; printf 'fixture\n' > "$5/.git" ;;
   remove) rm -rf "$4" ;;
   *) exit 99 ;;
  esac ;;
 -C|fetch|status|tag|push) ;;
 *) exit 99 ;;
esac
`)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "bash", filepath.Join(root, "scripts", "release.sh"), "ui", "0.1.0", "--execute") //nolint:gosec // G204: the copied release script can reach only fixture tools.
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "TRACE="+filepath.Join(root, "events"), "FIXTURE_GO="+goBin)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture release: %v\n%s", err, output)
	}
	trace, err := os.ReadFile(filepath.Join(root, "events")) //nolint:gosec // G304: fixed trace inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	want := "gorelease|" + selectedModule + "|"
	if !strings.Contains(string(trace), want) {
		t.Fatalf("compatibility did not inspect selected ui module:\n%s", trace)
	}
	if !strings.Contains(string(trace), "tag -a ui/v0.1.0 -m ui v0.1.0.") || strings.Contains(string(trace), "tag -a examples/") {
		t.Fatalf("release module changed during gates:\n%s", trace)
	}
	write("examples/go.mod", "module example.invalid/oolong/examples\n\ngo 1.27.0\n\nreplace(\n example.invalid/pkg => ../ui\n)\n")
	check := exec.CommandContext(t.Context(), "bash", filepath.Join(root, "scripts", "release.sh"), "ui", "0.1.0") //nolint:gosec // Release preflight runs against fixture tools.
	check.Dir, check.Env = command.Dir, command.Env
	if out, err := check.CombinedOutput(); err == nil || !strings.Contains(string(out), "examples/go.mod contains a local replacement") {
		t.Fatalf("release accepted or misdiagnosed a grouped replacement: %v\n%s", err, out)
	}
}

func TestModuleBuildChecksLibrariesAndCommandsWithoutOutputCollisions(t *testing.T) {
	for _, test := range []struct {
		name    string
		sources map[string]string
		valid   bool
	}{
		{"library", map[string]string{"library.go": "package library\n"}, true},
		{"command", map[string]string{"main.go": "package main\nfunc main() {}\n", "command/source.txt": "preserve"}, true},
		{"mixed", map[string]string{"cmd/main.go": "package main\nfunc main() {}\n", "library/library.go": "package library\nvar Value = missing\n"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			test.sources["go.mod"] = "module example.invalid/command\n\ngo 1.27.0\n"
			for name, source := range test.sources {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts", "build-module.sh"), dir) //nolint:gosec // The repository build operation compiles only an isolated fixture.
			command.Env = append(os.Environ(), "GOWORK=off")
			output, err := command.CombinedOutput()
			if (err == nil) != test.valid {
				t.Fatalf("build: %v\n%s", err, output)
			}
			if test.name == "command" {
				data, err := os.ReadFile(filepath.Join(dir, "command", "source.txt")) //nolint:gosec // G304: fixed source marker inside this test's temporary module.
				if err != nil || string(data) != "preserve" {
					t.Fatalf("command output changed source: %q, %v", data, err)
				}
			}
		})
	}
}

func TestReleasePublishesExactlyOneImmutableTag(t *testing.T) {
	function := scriptFunction(t, "release.sh", "publish")
	for _, failure := range []string{"none", "tag", "push", "verify", "wrong-commit"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			script := fmt.Sprintf(`
module=extensions/markdown
version=v0.4.0
tag=extensions/markdown/v0.4.0
head=reviewed-commit
failure=%q
note() { :; }
git() {
 printf '%%s\n' "$*" >> events
 case "$1" in
  tag) [[ "$failure" != tag ]] ;;
  push) [[ "$failure" != push ]] ;;
  ls-remote)
   [[ "$failure" != verify ]] || return 42
   if [[ "$failure" == wrong-commit ]]; then echo wrong-commit; else printf 'reviewed-commit\trefs/tags/%%s^{}\n' "$tag"; fi
   ;;
  *) return 99 ;;
 esac
}
%s
publish
`, failure, function)
			output, err := runReleaseFragment(t, dir, script)
			if (err == nil) != (failure == "none") {
				t.Fatalf("failure=%s result=%v\n%s", failure, err, output)
			}
			trace, readErr := os.ReadFile(filepath.Join(dir, "events")) //nolint:gosec // G304: fixed trace file inside t.TempDir.
			if readErr != nil {
				t.Fatal(readErr)
			}
			commands := strings.Split(strings.TrimSpace(string(trace)), "\n")
			if !strings.HasPrefix(commands[0], "tag -a extensions/markdown/v0.4.0 ") || !strings.HasSuffix(commands[0], " reviewed-commit") {
				t.Fatalf("tag = %q", commands[0])
			}
			if failure != "tag" && commands[1] != "push --quiet --no-follow-tags origin refs/tags/extensions/markdown/v0.4.0" {
				t.Fatalf("push = %q", commands)
			}
			for _, line := range commands {
				if strings.Contains(line, "--force") || strings.HasPrefix(line, "add ") || strings.HasPrefix(line, "commit ") {
					t.Fatalf("unexpected mutation: %s", line)
				}
			}
		})
	}
}

func TestReleaseIgnoresTheUsersFollowTagsPreference(t *testing.T) {
	publish := scriptFunction(t, "release.sh", "publish")
	output, err := runReleaseFragment(t, t.TempDir(), `
git init --bare -q remote.git
git init -q work
cd work
git config user.name Fixture
git config user.email fixture@example.invalid
git config commit.gpgsign false
git config tag.gpgsign false
git config push.followTags true
git remote add origin ../remote.git
git commit -q --allow-empty -m reviewed
git tag -a unrelated/v1.0.0 -m unrelated
module=ui
version=v1.0.0
tag="$module/$version"
head=$(git rev-parse HEAD)
note() { :; }
`+publish+`
publish
[[ "$(git ls-remote --refs --tags origin | cut -f2)" == "refs/tags/$tag" ]]
`)
	if err != nil {
		t.Fatalf("release must publish only the selected module tag: %v\n%s", err, output)
	}
}

func TestReleaseComparisonPropagatesToolFailure(t *testing.T) {
	function := scriptFunction(t, "release.sh", "compatibility")
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "extensions", "markdown"), 0o700); err != nil {
				t.Fatal(err)
			}
			source := "#!/bin/sh\n"
			if fail {
				source += "echo comparison-failed >&2\nexit 42\n"
			} else {
				source += "test \"$*\" = '-base=v0.3.0 -version=v0.4.0' || exit 2\necho compatible\n"
			}
			executable := filepath.Join(dir, "gorelease")
			//nolint:gosec // G306: this isolated fake tool must be executable by the test.
			if err := os.WriteFile(executable, []byte(source), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "scripts"), 0o700); err != nil {
				t.Fatal(err)
			}
			//nolint:gosec // The isolated fixture supplies the release baseline to the command under test.
			if err := os.WriteFile(filepath.Join(dir, "scripts", "module-tag.sh"), []byte("#!/bin/sh\ntest \"$*\" = '--latest extensions/markdown' || exit 2\necho extensions/markdown/v0.3.0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			script := fmt.Sprintf("version=v0.4.0\ngorelease_bin=%q\n", executable) + function + "compatibility extensions/markdown\n"
			output, err := runReleaseFragment(t, dir, script)
			if (err != nil) != fail {
				t.Fatalf("failed=%t error=%v\n%s", fail, err, output)
			}
		})
	}
}

func TestStandaloneReplacementsUseParsedModuleIdentities(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // G306,G703: only fixture scripts in t.TempDir are executed.
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write("scripts/modules.sh", "#!/bin/sh\nprintf '%s\\n' ui extension\n")
	for _, member := range []string{"ui", "extension"} {
		write(member+"/go.mod", "module \"example.invalid/"+member+"\" // independent module\n\ngo 1.27.0\n")
	}
	dir := filepath.Join(root, "extension")
	cmd := exec.CommandContext(t.Context(), "sh", filepath.Join(repoRoot(t), "scripts", "replace-local-modules.sh"), root) //nolint:gosec // G204: repository script and fixture checkout.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("apply standalone replacements: %s: %v", out, err)
	}
	inspect := exec.CommandContext(t.Context(), "go", "mod", "edit", "-json")
	inspect.Dir = dir
	inspect.Env = append(os.Environ(), "GOWORK=off")
	out, err := inspect.Output()
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Replace []struct{ Old, New struct{ Path string } }
	}
	if err = json.Unmarshal(out, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Replace) != 1 || manifest.Replace[0].Old.Path != "example.invalid/ui" {
		t.Fatalf("replacement identities (including self): %+v", manifest.Replace)
	}
	// Git Bash and Go may spell the same Windows fixture directory differently.
	old, err := os.Stat(filepath.Join(root, "ui"))
	if err != nil {
		t.Fatal(err)
	}
	local, err := os.Stat(manifest.Replace[0].New.Path)
	if err != nil || !os.SameFile(old, local) {
		t.Fatalf("replacement target %s is not the fixture's ui module: %v", manifest.Replace[0].New.Path, err)
	}
}
