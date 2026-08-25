package apiledger

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestIncompatibleSymbolsStopsAtCompatibleChanges(t *testing.T) {
	report := strings.Join([]string{
		"Incompatible changes:",
		"- ./pkg.Removed: removed",
		"- ./pkg.Changed: changed from func() to func(int)",
		"- ./pkg.Interface.Method: added",
		"Compatible changes:",
		"- ./pkg.Added: added",
	}, "\n")
	want := []string{"pkg.Removed", "pkg.Changed", "pkg.Interface.Method"}
	if got := incompatibleSymbols(report); !slices.Equal(got, want) {
		t.Fatalf("incompatibleSymbols = %q, want %q", got, want)
	}
}

func TestReleaseSectionMatchesTheWholeVersion(t *testing.T) {
	document := strings.Join([]string{
		"## [Unreleased]",
		"next",
		"## [0.10.0] — 2026-01-02",
		"ten",
		"## [0.1.0] — 2026-01-01",
		"one",
	}, "\n")
	got, err := releaseSection(document, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "one" {
		t.Fatalf("section = %q, want only 0.1.0", got)
	}
	if _, err := releaseSection(document, "missing"); err == nil {
		t.Fatal("missing section was accepted")
	}
}

func TestModuleLedgerEndsAtTheNextPeerOrParentHeading(t *testing.T) {
	section := strings.Join([]string{
		"### Breaking API migration",
		"#### core",
		"- `grid.Cell.Width` changed.",
		"#### components",
		"- `headless.Editor` changed.",
		"### Fixed",
		"nothing",
	}, "\n")
	got, ok := moduleLedger(section, "core")
	if !ok || !strings.Contains(got, "`grid.Cell.Width`") || strings.Contains(got, "headless.Editor") {
		t.Fatalf("core ledger = %q, found %v", got, ok)
	}
	if _, ok := moduleLedger(section, "markdown"); ok {
		t.Fatal("missing module ledger was found")
	}
}

func TestCheckerRequiresEveryPlatformBreakInTheExactModuleLedger(t *testing.T) {
	root := t.TempDir()
	writeChangelog(t, root, strings.Join([]string{
		"## [Unreleased]",
		"### Breaking API migration",
		"#### core",
		"- `grid.Cell.Width` now reports a complete display atom.",
		"- `grid.OldName` is extra migration context and is not a second baseline.",
		"#### latex",
		"- `latex.GlyphsFor` now accepts its complete rendering policy.",
		"## [0.10.0] — 2026-01-01",
	}, "\n"))
	run := &fixtureRunner{}
	var log bytes.Buffer
	if err := newChecker(Config{Root: root, Log: &log}, run).check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := log.String(); got != "fresh: no released baseline; skipped\n" {
		t.Fatalf("progress log = %q", got)
	}
	if got := run.exportPlatforms(); !slices.Equal(got, []string{
		"darwin", "darwin", "darwin", "darwin", "darwin", "darwin",
		"linux", "linux", "linux", "linux", "linux", "linux",
		"windows", "windows", "windows", "windows", "windows", "windows",
	}) {
		t.Fatalf("export platforms = %q, want old and new exports for three modules on every platform", got)
	}

	writeChangelog(t, root, "## [Unreleased]\n### Breaking API migration\n#### core\n")
	err := newChecker(Config{Root: root}, &fixtureRunner{}).check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "core/grid.Cell.Width (baseline core/v0.10.0)") {
		t.Fatalf("missing exact entry error = %v", err)
	}
}

func TestModuleReleaseRequiresAnExactOrModuleQualifiedSymbol(t *testing.T) {
	release := moduleRelease{name: "latex", path: "github.com/Tangerg/oolong/latex", tag: "latex/v0.10.0"}
	for _, ledger := range []string{
		"#### latex\n- `GlyphsFor` changed.\n",
		"#### latex\n- `latex.GlyphsFor` changed.\n",
	} {
		if got := release.migrationProblems(ledger, "Unreleased", []string{"GlyphsFor"}); len(got) != 0 {
			t.Fatalf("ledger %q produced problems %q", ledger, got)
		}
	}
	wrong := "#### latex\n- `other.GlyphsFor` changed.\n"
	if got := release.migrationProblems(wrong, "Unreleased", []string{"GlyphsFor"}); len(got) != 1 {
		t.Fatalf("wrong qualifier produced problems %q, want one", got)
	}
}

func TestCheckerReportsTheCommandAndItsOutput(t *testing.T) {
	root := t.TempDir()
	writeChangelog(t, root, "## [Unreleased]\n")
	want := errors.New("exit status 1")
	err := newChecker(Config{Root: root}, failingRunner{err: want}).check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "scripts/modules.sh --public: exit status 1: broken workspace") {
		t.Fatalf("command error = %v", err)
	}
}

func TestCheckerRejectsAnInvalidPublicInventory(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		want   string
	}{
		{name: "empty", want: "returned no modules"},
		{name: "duplicate", output: strings.Repeat("core\n", 2), want: "returned duplicate modules"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeChangelog(t, root, "## [Unreleased]\n")
			err := newChecker(Config{Root: root}, moduleInventoryRunner(test.output)).check(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid inventory error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestOSRunnerSeparatesDataFromDiagnostics(t *testing.T) {
	run := osRunner{}
	out, err := run.run(t.Context(), command{
		name: "sh",
		args: []string{"-c", "printf 'core\\n'; printf 'diagnostic\\n' >&2"},
	})
	if err != nil || out != "core\n" {
		t.Fatalf("successful command = %q, %v; want stdout only", out, err)
	}

	out, err = run.run(t.Context(), command{
		name: "sh",
		args: []string{"-c", "printf 'broken workspace\\n' >&2; exit 7"},
	})
	if err == nil || out != "broken workspace\n" {
		t.Fatalf("failed command = %q, %v; want its diagnostic", out, err)
	}
}

func writeChangelog(t *testing.T, root, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fixtureRunner struct {
	commands []command
}

func (r *fixtureRunner) run(_ context.Context, cmd command) (string, error) {
	r.commands = append(r.commands, cmd)
	switch {
	case cmd.name == "sh" && len(cmd.args) == 2 && filepath.Base(cmd.args[0]) == "modules.sh" && cmd.args[1] == "--public":
		return "components\ncore\nfresh\nlatex\n", nil
	case cmd.name == "git":
		module := strings.TrimSuffix(cmd.args[2], "/v*")
		if module == "fresh" {
			return "", nil
		}
		return module + "/v0.10.0\n" + module + "/v0.9.0\n", nil
	case cmd.name == "go" && slices.Equal(cmd.args, []string{"list", "-m", "-f", "{{.Path}}"}):
		return "github.com/Tangerg/oolong/" + filepath.Base(cmd.dir) + "\n", nil
	case cmd.name == "go" && len(cmd.args) >= 2 && cmd.args[0] == "mod" && cmd.args[1] == "download":
		return "", nil
	case cmd.name == "go" && len(cmd.args) >= 2 && cmd.args[0] == "list" && strings.Contains(cmd.args[len(cmd.args)-1], "@v0.10.0"):
		return "/released/" + strings.TrimSuffix(filepath.Base(cmd.args[len(cmd.args)-1]), "@v0.10.0") + "\n", nil
	case cmd.name == "apidiff" && len(cmd.args) >= 2 && cmd.args[1] == "-w":
		return "", nil
	case cmd.name == "apidiff" && len(cmd.args) == 3 && strings.Contains(filepath.ToSlash(cmd.args[1]), "/core/"):
		return "Incompatible changes:\n- ./grid.Cell.Width: changed\nCompatible changes:\n", nil
	case cmd.name == "apidiff" && len(cmd.args) == 3 && strings.Contains(filepath.ToSlash(cmd.args[1]), "/latex/"):
		return "Incompatible changes:\n- ./GlyphsFor: changed\nCompatible changes:\n", nil
	case cmd.name == "apidiff" && len(cmd.args) == 3:
		return "Incompatible changes:\nCompatible changes:\n", nil
	default:
		return "", errors.New("unexpected command: " + cmd.description())
	}
}

func (r *fixtureRunner) exportPlatforms() []string {
	var found []string
	for _, command := range r.commands {
		if command.name == "apidiff" && len(command.args) >= 2 && command.args[1] == "-w" {
			found = append(found, command.env["GOOS"])
		}
	}
	slices.Sort(found)
	return found
}

type failingRunner struct{ err error }

func (r failingRunner) run(context.Context, command) (string, error) {
	return "broken workspace", r.err
}

type moduleInventoryRunner string

func (output moduleInventoryRunner) run(_ context.Context, cmd command) (string, error) {
	if cmd.name == "sh" && len(cmd.args) == 2 && filepath.Base(cmd.args[0]) == "modules.sh" && cmd.args[1] == "--public" {
		return string(output), nil
	}
	return "", errors.New("unexpected command: " + cmd.description())
}
