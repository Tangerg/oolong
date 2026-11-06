//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

const (
	editorFixtureMarker      = "--editor-fixture"
	editorFixtureExecutable  = "OOLONG_EDITOR_TEST_EXECUTABLE"
	editorFixtureFailure     = "--editor-fixture-failure"
	editorFixtureFailureCode = 23
)

func TestWindowsEditorCommandsPreserveArguments(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(editorFixtureExecutable, executable)
	t.Setenv(editorFileEnvironment, "parent value")
	directory := t.TempDir()
	batch := filepath.Join(directory, "editor with spaces.cmd")
	script := `@echo off
"%` + editorFixtureExecutable + `%" -test.run=^TestWindowsEditorProcessFixture$ -- ` + editorFixtureMarker + ` %*
`
	if err := os.WriteFile(batch, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "draft %PATH% ! & (kept).txt")
	for _, editor := range []string{
		`"%` + editorFixtureExecutable + `%" -test.run=^TestWindowsEditorProcessFixture$ -- ` + editorFixtureMarker,
		`"` + batch + `"`,
	} {
		t.Run(editor, func(t *testing.T) {
			cmd, err := editorCommand(t.Context(), editor+` --wait "two words"`, filename)
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("editor: %v", err)
			}
			var arguments []string
			if err := json.Unmarshal(output, &arguments); err != nil {
				t.Fatalf("editor arguments %q: %v", output, err)
			}
			if want := []string{"--wait", "two words", filename}; !slices.Equal(arguments, want) {
				t.Fatalf("editor arguments = %q, want %q", arguments, want)
			}
			if got := os.Getenv(editorFileEnvironment); got != "parent value" {
				t.Fatalf("editor mutated its parent's environment: %q", got)
			}
		})
	}
}

func TestWindowsEditorPreservesTheChildExitError(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(editorFixtureExecutable, executable)
	editor := `"%` + editorFixtureExecutable + `%" -test.run=^TestWindowsEditorProcessFixture$ -- ` + editorFixtureMarker + " " + editorFixtureFailure
	cmd, err := editorCommand(t.Context(), editor, "unused.txt")
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Run()
	if failure, ok := errors.AsType[*exec.ExitError](err); !ok || failure.ExitCode() != editorFixtureFailureCode {
		t.Fatalf("editor exit error lost its identity or status: %v", err)
	}
}

func TestWindowsEditorProcessFixture(_ *testing.T) {
	index := slices.Index(os.Args, editorFixtureMarker)
	if index < 0 {
		return
	}
	arguments := os.Args[index+1:]
	if slices.Contains(arguments, editorFixtureFailure) {
		os.Exit(editorFixtureFailureCode)
	}
	if err := json.NewEncoder(os.Stdout).Encode(arguments); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
