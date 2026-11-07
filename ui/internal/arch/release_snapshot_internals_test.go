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

func TestReleaseChecksTheTaggedCommit(t *testing.T) {
	type mutation uint8
	const (
		callerSource mutation = iota
		candidateSource
		candidateCommit
	)
	for _, test := range []struct {
		name           string
		valid          bool
		edit           mutation
		wantDiagnostic string
	}{
		{"caller repair cannot bless a failing commit", false, callerSource, "ui has failing tests"},
		{"caller regression cannot replace a passing commit", true, callerSource, ""},
		{"gate mutation cannot replace the candidate", true, candidateSource, "release checks changed the committed candidate"},
		{"gate commit cannot bless a failing commit", false, candidateCommit, "release checks changed the candidate commit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReleaseSnapshot(t, test.valid)
			head := fixture.git(t, "rev-parse", "HEAD")
			editPath := filepath.Join(fixture.root, "ui", "value.go")
			edited := releaseValueSource(!test.valid)
			if test.edit != callerSource {
				editPath = "ui/value.go"
			}
			if test.edit == candidateSource {
				edited = releaseValueSource(test.valid) + "\n"
			}
			command := exec.CommandContext(t.Context(), "bash", "scripts/release.sh", "ui", "0.1.0", "--execute")
			command.Dir = fixture.root
			command.Env = append(os.Environ(),
				"PATH="+fixture.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GOWORK=off", "FIXTURE_GO="+fixture.goBin,
				"FIXTURE_EDIT_PATH="+editPath, "FIXTURE_EDIT_SOURCE="+edited,
				"FIXTURE_COMMIT_EDIT="+strconv.FormatBool(test.edit == candidateCommit),
				"FIXTURE_EDIT_MARKER="+filepath.Join(fixture.bin, "edited"))
			output, err := command.CombinedOutput()
			if test.wantDiagnostic == "" {
				if err != nil {
					t.Fatalf("release: %v\n%s", err, output)
				}
				published := fixture.git(t, "ls-remote", "--tags", "origin", "refs/tags/ui/v0.1.0^{}")
				if !strings.HasPrefix(published, head+"\t") {
					t.Fatalf("published %q, want committed candidate %s", published, head)
				}
			} else {
				if err == nil || !strings.Contains(string(output), test.wantDiagnostic) {
					t.Fatalf("release = %v, want %q\n%s", err, test.wantDiagnostic, output)
				}
				if tags := fixture.git(t, "ls-remote", "--tags", "origin"); tags != "" {
					t.Fatalf("rejected candidate published tags: %s", tags)
				}
				if tags := fixture.git(t, "tag", "--list"); tags != "" {
					t.Fatalf("rejected candidate created tags: %s", tags)
				}
			}
			wantSource := edited
			if test.edit != callerSource {
				wantSource = releaseValueSource(test.valid)
			}
			source, err := os.ReadFile(filepath.Join(fixture.root, "ui", "value.go"))
			if err != nil || string(source) != wantSource {
				t.Fatalf("caller source = %q, %v; want %q", source, err, wantSource)
			}
			if trees := fixture.git(t, "worktree", "list", "--porcelain"); strings.Count(trees, "worktree ") != 1 {
				t.Fatalf("release retained its temporary checkout: %s", trees)
			}
		})
	}
}

type releaseSnapshot struct {
	root  string
	bin   string
	goBin string
}

func newReleaseSnapshot(t *testing.T, valid bool) releaseSnapshot {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	fixture := releaseSnapshot{root: t.TempDir(), bin: t.TempDir(), goBin: goBin}
	for _, name := range []string{"release.sh", "module-tag.sh", "build-module.sh", "tools.sh"} {
		body, err := readRepositoryFile(filepath.Join(repoRoot(t), "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		writeReleaseFixture(t, filepath.Join(fixture.root, "scripts", name), string(body))
	}
	writeReleaseFixture(t, filepath.Join(fixture.root, "scripts", "modules.sh"), "#!/bin/sh\nprintf 'ui\\n'\n")
	for _, name := range []string{"sync-workspace.sh", "check-reachability.sh"} {
		writeReleaseFixture(t, filepath.Join(fixture.root, "scripts", name), "#!/bin/sh\nexit 0\n")
	}
	writeReleaseFixture(t, filepath.Join(fixture.root, "ui", "go.mod"), "module example.invalid/oolong/ui\n\ngo 1.27.0\n")
	writeReleaseFixture(t, filepath.Join(fixture.root, "ui", "value.go"), releaseValueSource(valid))
	writeReleaseFixture(t, filepath.Join(fixture.root, "ui", "value_test.go"), `package candidate

import "testing"

func TestValid(t *testing.T) {
	if !Valid {
		t.Fatal("committed candidate is invalid")
	}
}
`)
	writeReleaseFixture(t, filepath.Join(fixture.root, "CHANGELOG.md"), "## [ui/v0.1.0]\n")
	for _, name := range []string{"apidiff", "deadcode", "golangci-lint", "gofumpt", "govulncheck", "npx", "shfmt", "npm", "gorelease"} {
		writeReleaseFixture(t, filepath.Join(fixture.bin, name), "#!/bin/sh\nexit 0\n")
	}
	writeToolBuildInfo(t, fixture.bin)
	// Build, vet, tests and Git remain real. Unrelated gates cannot reach a service,
	// and the one injected edit occurs only after the release captures its commit.
	writeReleaseFixture(t, filepath.Join(fixture.bin, "go"), `#!/bin/sh
set -eu
if [ "${1-} ${2-}" = 'version -m' ]; then cat "$3.buildinfo"; exit; fi
if [ -n "${GOOS-}" ] || [ "${1-}" = fix ]; then exit 0; fi
if [ "${1-}" = -C ] && [ "${3-}" = run ]; then exit 0; fi
if [ "${1-}" = -C ] && [ "${3-}" = build ] && [ ! -e "$FIXTURE_EDIT_MARKER" ]; then
 printf '%s' "$FIXTURE_EDIT_SOURCE" > "$FIXTURE_EDIT_PATH"
 if [ "$FIXTURE_COMMIT_EDIT" = true ]; then
  git add "$FIXTURE_EDIT_PATH"
  git commit -qm 'gate changed the candidate'
 fi
 : > "$FIXTURE_EDIT_MARKER"
fi
exec "$FIXTURE_GO" "$@"
`)
	fixture.git(t, "init", "-q", "-b", "main")
	fixture.git(t, "config", "user.name", "Release Fixture")
	fixture.git(t, "config", "user.email", "release@example.invalid")
	fixture.git(t, "add", ".")
	fixture.git(t, "commit", "-qm", "reviewed candidate")
	remote := t.TempDir()
	fixture.git(t, "init", "--bare", "-q", remote)
	fixture.git(t, "remote", "add", "origin", remote)
	fixture.git(t, "push", "-q", "-u", "origin", "main")
	return fixture
}

func releaseValueSource(valid bool) string {
	return fmt.Sprintf("package candidate\n\nconst Valid = %t\n", valid)
}

func writeReleaseFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G306,G703: executable fixture files are confined to this test's temporary directories.
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func (f releaseSnapshot) git(t *testing.T, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // G204: repository-authored arguments can reach only this fixture and its local bare remote.
	command.Dir = f.root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
