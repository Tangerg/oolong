package main

import (
	"os"
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
	root := workspaceFixture(t)
	path := filepath.Join(root, "go.work")
	body, err := os.ReadFile(path) //nolint:gosec // G304: fixed go.work inside t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("\nreplace example.invalid/external v1.0.0 => ./ui\n")...)
	if err := os.WriteFile(path, body, 0o600); err != nil { //nolint:gosec // G703: path is the fixed go.work inside t.TempDir.
		t.Fatal(err)
	}
	if err := syncWorkspace(t.Context(), root, false); err == nil {
		t.Fatal("manual ownership was overwritten")
	}
}
