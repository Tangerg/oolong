package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func workspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.work":                    "go 1.27.0\n\nuse (\n ./ui\n ./extensions/markdown\n ./examples\n)\n",
		"ui/go.mod":                  "module example.invalid/oolong/ui\n\ngo 1.27.0\n",
		"extensions/markdown/go.mod": "module example.invalid/oolong/extensions/markdown\n\ngo 1.27.0\n\nrequire example.invalid/oolong/ui v0.2.0\n",
		"examples/go.mod":            "module example.invalid/oolong/examples\n\ngo 1.27.0\n\nrequire (\n example.invalid/oolong/ui v0.3.0\n example.invalid/oolong/extensions/markdown v0.4.0\n)\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReplacementsAreDerivedFromEveryRequiredLocalVersion(t *testing.T) {
	root := workspaceFixture(t)
	if err := syncWorkspace(t.Context(), root, false); err != nil {
		t.Fatal(err)
	}
	body, readErr := os.ReadFile(filepath.Join(root, "go.work")) //nolint:gosec // G304: root is the isolated workspace fixture.
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, line := range []string{
		"example.invalid/oolong/ui v0.2.0 => ./ui",
		"example.invalid/oolong/ui v0.3.0 => ./ui",
		"example.invalid/oolong/extensions/markdown v0.4.0 => ./extensions/markdown",
	} {
		if strings.Count(string(body), line) != 1 {
			t.Fatalf("derived manifest missing %q:\n%s", line, body)
		}
	}
	if err := syncWorkspace(t.Context(), root, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "extensions", "markdown", "go.mod")
	updated := "module example.invalid/oolong/extensions/markdown\n\ngo 1.27.0\n\nrequire example.invalid/oolong/ui v0.2.1\n"
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syncWorkspace(t.Context(), root, true); err == nil {
		t.Fatal("stale generated replacements passed --check")
	}
	unchanged, err := os.ReadFile(filepath.Join(root, "go.work")) //nolint:gosec // G304: fixed file inside the isolated fixture.
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(body) {
		t.Fatal("--check modified the workspace")
	}
}

func TestManualReplacementsAreNotSilentlyOverwritten(t *testing.T) {
	for _, manual := range []string{
		"replace example.invalid/external v1.0.0 => ./ui",
		"replace(\nexample.invalid/external v1.0.0 => ./ui\n)",
	} {
		root := workspaceFixture(t)
		path := filepath.Join(root, "go.work")
		body, err := os.ReadFile(path) //nolint:gosec // G304: root is the isolated workspace fixture.
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, []byte("\n"+manual+"\n")...)
		if err := os.WriteFile(path, body, 0o600); err != nil { //nolint:gosec // G703: path is the fixed go.work inside t.TempDir.
			t.Fatal(err)
		}
		if err := syncWorkspace(t.Context(), root, false); err == nil {
			t.Fatalf("manual workspace replacement accepted: %s", manual)
		}
	}
}

func TestSyncPreservesMembershipAfterTheGeneratedDirective(t *testing.T) {
	for _, ending := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(ending.name, func(t *testing.T) {
			checkPreservedMembership(t, ending.newline)
		})
	}
}

func checkPreservedMembership(t *testing.T, newline string) {
	t.Helper()
	root := workspaceFixture(t)
	if err := syncWorkspace(t.Context(), root, false); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "go.work")
	body, err := os.ReadFile(work) //nolint:gosec // Isolated fixture manifest.
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), "\n", newline))
	//nolint:gosec // G703: go.work belongs to the isolated fixture.
	if err = os.WriteFile(work, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = syncWorkspace(t.Context(), root, true); err != nil {
		t.Fatalf("canonical generated block refused before mutation: %v", err)
	}
	newMember := filepath.Join(root, "fresh")
	if err = os.Mkdir(newMember, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(newMember, "go.mod"), []byte("module example.invalid/oolong/fresh\n\ngo 1.27.0\nrequire example.invalid/oolong/ui v0.5.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll("\n// A newly added module owns this directive.\nuse ./fresh\n", "\n", newline)
	//nolint:gosec // G703: fixed go.work inside the isolated workspace.
	if err = os.WriteFile(work, append(body, suffix...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = syncWorkspace(t.Context(), root, false); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(work) //nolint:gosec // Isolated fixture manifest.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(body), suffix) || !strings.Contains(string(body), "example.invalid/oolong/ui v0.5.0 => ./ui") {
		t.Fatalf("sync lost member or replacement:\n%s", body)
	}
	if newline == "\r\n" && strings.Contains(strings.ReplaceAll(string(body), "\r\n", ""), "\n") {
		t.Fatal("generated block introduced bare LF into the CRLF workspace")
	}
	if err = syncWorkspace(t.Context(), root, true); err != nil {
		t.Fatalf("sync did not reach a fixed point: %v", err)
	}
}

func TestGoDiagnosticsDoNotBecomeWorkspaceJSON(t *testing.T) {
	root, err := exec.CommandContext(t.Context(), "go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPATH", strings.TrimSpace(string(root)))
	if _, err := memberDirectories(t.Context(), workspaceFixture(t)); err != nil {
		t.Fatal(err)
	}
}
