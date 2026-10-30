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
			got, err := npmCLI(filepath.Join(bin, "mmdc.cmd"))
			if err != nil || got != script {
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
