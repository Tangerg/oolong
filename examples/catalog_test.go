package examples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogCoversEveryCommand(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	catalog := string(readme)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	commands := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		mainPath := filepath.Join(name, "main.go")
		if _, err := os.Stat(mainPath); err != nil {
			continue
		}
		commands++
		if _, err := os.Stat(filepath.Join(name, "main_test.go")); err != nil {
			t.Errorf("%s is a command without main_test.go", name)
		}
		link := "[`" + name + "`](" + name + ")"
		if n := strings.Count(catalog, link); n != 1 {
			t.Errorf("README contains %d entries for %s, want one", n, name)
		}
		// mainPath is derived from a directory entry below the example module.
		//nolint:gosec // G304: the test deliberately audits repository-owned files.
		main, err := os.ReadFile(mainPath)
		if err != nil {
			t.Errorf("read %s: %v", mainPath, err)
			continue
		}
		if !strings.HasPrefix(string(main), "// Command "+name+" ") {
			t.Errorf("%s must begin with a Command %s package comment", mainPath, name)
		}
	}
	if commands == 0 {
		t.Fatal("no example commands found, so the catalog test proves nothing")
	}
}
