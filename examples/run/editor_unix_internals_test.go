//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeEditorScript(t *testing.T, filename, script string) {
	t.Helper()
	//nolint:gosec // G306: the fixture needs owner-only execution permission for the test shell.
	if err := os.WriteFile(filename, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestEditorCommandPreservesShellArguments(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	writeEditorScript(t, "editor with spaces", "#!/bin/sh\nprintf '%s\\000' \"$@\"\n")
	t.Setenv("OOLONG_EDITOR_TEST_EXECUTABLE", "./editor with spaces")
	t.Setenv("OOLONG_EDITOR_TEST_LABEL", "expanded value")
	filename := filepath.Join(directory, "draft ' \" $HOME $(printf expanded) `printf expanded` ! & ; (kept)\\\n.txt")
	for _, test := range []struct {
		command string
		args    []string
	}{
		{`'./editor with spaces' --wait --label 'two words'`, []string{"--wait", "--label", "two words"}},
		{`"./editor with spaces" 'embedded "quotes"' ''`, []string{`embedded "quotes"`, ""}},
		{`"$OOLONG_EDITOR_TEST_EXECUTABLE" "$OOLONG_EDITOR_TEST_LABEL"`, []string{"expanded value"}},
	} {
		t.Run(test.command, func(t *testing.T) {
			cmd, err := editorCommand(t.Context(), test.command, filename)
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("editor: %v", err)
			}
			want := append(slices.Clone(test.args), filename)
			got := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
			if !slices.Equal(got, want) {
				t.Fatalf("editor arguments = %q, want %q", got, want)
			}
		})
	}
}

func TestEditUsesTheDefaultEditorAndRemovesItsTemporaryFile(t *testing.T) {
	const captureEnvironment = "OOLONG_EDITOR_TEST_CAPTURE"
	directory, executables := t.TempDir(), t.TempDir()
	capture := filepath.Join(t.TempDir(), "filename")
	script := `#!/bin/sh
[ "$#" -eq 1 ] && [ -f "$1" ] || exit 1
printf '%s' "$1" > "$` + captureEnvironment + `"
`
	writeEditorScript(t, filepath.Join(executables, "vi"), script)
	t.Setenv("EDITOR", "")
	t.Setenv("TMPDIR", directory)
	t.Setenv("PATH", executables+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(captureEnvironment, capture)
	if err := edit(); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G304: the fake editor reports through this test's private capture file.
	filename, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(string(filename)) != directory || filepath.Ext(string(filename)) != ".txt" {
		t.Fatalf("default editor received an unexpected file: %q", filename)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary editor file survived completion: %v, %v", entries, err)
	}
}

func TestEditPreservesTheChildExitError(t *testing.T) {
	const exitStatus = 23
	directory := t.TempDir()
	t.Chdir(directory)
	writeEditorScript(t, "failed editor", fmt.Sprintf("#!/bin/sh\nexit %d\n", exitStatus))
	t.Setenv("EDITOR", `'./failed editor' --wait 'one argument'`)
	t.Setenv("TMPDIR", t.TempDir())
	err := edit()
	if failure, ok := errors.AsType[*exec.ExitError](err); !ok || failure.ExitCode() != exitStatus {
		t.Fatalf("editor exit error lost its identity or status: %v", err)
	}
	entries, err := os.ReadDir(os.Getenv("TMPDIR"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed editor retained its temporary file: %v, %v", entries, err)
	}
}

func TestEditorRejectsInvalidShellSyntax(t *testing.T) {
	cmd, err := editorCommand(t.Context(), "'unclosed editor", "unused.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = cmd.Output()
	if failure, ok := errors.AsType[*exec.ExitError](err); !ok || failure.ExitCode() == 0 {
		t.Fatalf("invalid shell syntax did not retain its failure: %v", err)
	}
}
