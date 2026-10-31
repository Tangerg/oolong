package arch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLedgerSectionFollowsOneModuleTag(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // G306,G703: executable fixture paths are repository-authored under t.TempDir.
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // G204: fixed repository tools and fixture arguments.
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %s: %v", name, args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	for _, name := range []string{"ledger-section.sh", "module-tag.sh"} {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", name)) //nolint:gosec // G304: fixed repository script names.
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join("scripts", name), string(data))
	}
	module := func(name string) {
		write(name+"/go.mod", "module example.com/"+name+"\n\ngo 1.27.0\n")
		write(name+"/value.go", "package "+name+"\nconst Value=1\n")
	}
	commit := func() {
		run("git", "add", ".")
		run("git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	}
	expect := func(module, want string) {
		t.Helper()
		if got := run(shell, "scripts/ledger-section.sh", module); got != want {
			t.Fatalf("section=%q want=%q", got, want)
		}
	}
	run("git", "init")
	module("ui")
	write("go.work", "go 1.27.0\nuse (\n ./ui\n)\n")
	write("CHANGELOG.md", "## [Unreleased]\n## [ui/v0.1.0]\n")
	commit()
	expect("ui", "ui/v0.1.0")
	run("git", "tag", "ui/v0.1.0")
	expect("ui", "Unreleased")
	module("fresh")
	write("go.work", "go 1.27.0\nuse (\n ./ui\n ./fresh\n)\n")
	commit()
	expect("ui", "Unreleased")
	write("CHANGELOG.md", "## [Unreleased]\n## [fresh/v0.3.0]\n## [ui/v0.1.0]\n")
	commit()
	expect("fresh", "fresh/v0.3.0")
	run("git", "tag", "ui/v0.3.0")
	expect("fresh", "fresh/v0.3.0")
	write("CHANGELOG.md", "## [Unreleased]\n## [ui/v0.2.0]\n## [fresh/v0.3.0]\n## [ui/v0.1.0]\n")
	expect("ui", "ui/v0.2.0")
	expect("fresh", "fresh/v0.3.0")
	run("git", "tag", "fresh/v0.3.0")
	expect("fresh", "Unreleased")
	expect("ui", "ui/v0.2.0")
}
