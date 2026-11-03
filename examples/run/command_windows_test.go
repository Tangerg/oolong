//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandResolvesWindowsExecutableExtensions(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(executable))
	withoutExtension := strings.TrimSuffix(executable, filepath.Ext(executable))
	for _, path := range []string{withoutExtension, "." + string(os.PathSeparator) + filepath.Base(withoutExtension)} {
		t.Run(path, func(t *testing.T) {
			var output bytes.Buffer
			if err := runCommand(t.Context(), []string{path, "-test.run=^TestCommandProcessFixture$", "--", commandFixture, "success", "extension resolved"}, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "argument=extension resolved") {
				t.Fatalf("command did not execute: %q", output.String())
			}
		})
	}
}
