package mermaid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNPMEntryPointUsesPackageMetadata(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "global", true: "local"}[local], func(t *testing.T) {
			directory := t.TempDir()
			bin := directory
			root := filepath.Join(directory, "node_modules", "@mermaid-js", "mermaid-cli")
			if local {
				bin = filepath.Join(directory, "node_modules", ".bin")
				if err := os.MkdirAll(bin, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(root, "src", "cli.js")
			if err := os.WriteFile(script, []byte("// fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			metadata := filepath.Join(root, "package.json")
			if err := os.WriteFile(metadata, []byte(`{"name":"@mermaid-js/mermaid-cli","bin":{"mmdc":"src/cli.js"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			shim := filepath.Join(bin, "mmdc.cmd")
			if err := os.WriteFile(shim, []byte("@echo off"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := npmCLI(shim)
			if err != nil || !sameEntry(t, got, script) {
				t.Fatalf("entry=%q error=%v", got, err)
			}
			if err := os.WriteFile(metadata, []byte(`{"name":"@mermaid-js/mermaid-cli","bin":{"mmdc":"../outside.js"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := npmCLI(filepath.Join(bin, "mmdc.cmd")); err == nil {
				t.Fatal("accepted entry outside installed package")
			}
		})
	}
}

func TestSelectedSymlinkOwnsBackendEvenBesideAnotherInstallation(t *testing.T) {
	dir := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var selected string
	for _, root := range []string{filepath.Join(dir, "node_modules", "@mermaid-js", "mermaid-cli"), filepath.Join(dir, "selected")} {
		write(filepath.Join(root, "package.json"), `{"name":"@mermaid-js/mermaid-cli","bin":{"mmdc":"dist/bin/diagram.js"}}`)
		selected = filepath.Join(root, "dist", "bin", "diagram.js")
		write(selected, "// entry locator")
	}
	shim := filepath.Join(dir, "mmdc")
	if err := os.Symlink(selected, shim); err != nil {
		t.Skipf("cannot create npm symlink: %v", err)
	}
	for _, executable := range []string{shim, selected} {
		got, err := npmCLI(executable)
		if err != nil || !sameEntry(t, got, selected) {
			t.Fatalf("selected %s, got %s: %v", executable, got, err)
		}
	}
}

func sameEntry(t *testing.T, first, second string) bool {
	t.Helper()
	a, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(second)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(a, b)
}
