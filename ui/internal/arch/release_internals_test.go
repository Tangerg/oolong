package arch

import (
	"fmt"
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
	for _, name := range []string{"release.sh", "module-tag.sh"} {
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
	for _, name := range []string{"apidiff", "deadcode", "golangci-lint", "gofumpt", "govulncheck", "npx", "shfmt", "npm", "go"} {
		write("bin/"+name, "#!/bin/sh\nprintf '%s|%s|%s\\n' \"${0##*/}\" \"$PWD\" \"$*\" >> \"$TRACE\"\n")
	}
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
 fetch|status|tag|push) ;;
 *) exit 99 ;;
esac
`)
	command := exec.CommandContext(t.Context(), "bash", filepath.Join(root, "scripts", "release.sh"), "ui", "0.1.0", "--execute") //nolint:gosec // G204: the copied release script can reach only fixture tools.
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "TRACE="+filepath.Join(root, "events"))
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
			if failure != "tag" && commands[1] != "push --quiet origin refs/tags/extensions/markdown/v0.4.0" {
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
				source += "echo compatible\n"
			}
			executable := filepath.Join(dir, "gorelease")
			//nolint:gosec // G306: this isolated fake tool must be executable by the test.
			if err := os.WriteFile(executable, []byte(source), 0o700); err != nil {
				t.Fatal(err)
			}
			script := fmt.Sprintf("version=v0.4.0\ngorelease_bin=%q\ngit() { echo extensions/markdown/v0.3.0; }\n", executable) + function + "compatibility extensions/markdown 'extensions/markdown/v[01].*'\n"
			output, err := runReleaseFragment(t, dir, script)
			if (err != nil) != fail {
				t.Fatalf("failed=%t error=%v\n%s", fail, err, output)
			}
		})
	}
}
