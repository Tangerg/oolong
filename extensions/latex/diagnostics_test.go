package latex_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
)

const diagnosticProbeEnv = "OOLONG_LATEX_DIAGNOSTIC_PROBE"

func TestRenderingKeepsDiagnosticsInTheResult(t *testing.T) {
	if os.Getenv(diagnosticProbeEnv) == "1" {
		if err := diagnosticProbe(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A child observes both process streams without replacing global writers shared
	// by concurrent tests or formula requests.
	cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestRenderingKeepsDiagnosticsInTheResult$") //nolint:gosec // The executable is this test binary, with a fixed helper test.
	cmd.Env = append(os.Environ(), diagnosticProbeEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("formula probe: %v\nstdout: %s\nstderr: %s", err, &stdout, &stderr)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("rendering wrote process output\nstdout: %s\nstderr: %s", &stdout, &stderr)
	}
}

func diagnosticProbe() error {
	cases := []struct {
		source string
		valid  bool
	}{
		{`08`, true},
		{`1e`, true},
		{`0x`, true},
		{`1.2.3`, true},
		{`"unterminated`, true},
		{`x_08`, true},
		{`\textbf{08 1e}`, true},
		{`\frac{08}{1e}`, true},
		{`\frac{1}`, false},
		{`\unknown{x}`, false},
	}
	results := make(chan error, len(cases))
	var workers sync.WaitGroup
	for _, tc := range cases {
		workers.Go(func() {
			for range 4 {
				formula := latex.Render(tc.source, latex.Look{})
				if (formula.Err() == nil) != tc.valid {
					results <- errors.Join(fmt.Errorf("Render(%q): expected valid=%t", tc.source, tc.valid), formula.Err())
					return
				}
			}
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		return err
	}
	return nil
}

func TestMathematicalTokensDoNotUseGoLiteralSyntax(t *testing.T) {
	for _, source := range []string{`08`, `1e`, `0x`, `1.2.3`, `"unterminated`} {
		formula := latex.Render(source, latex.Look{})
		if err := formula.Err(); err != nil {
			t.Fatalf("Render(%q): %v", source, err)
		}
		if lines := formula.Lines(); len(lines) != 1 || lines[0].String() != source {
			t.Fatalf("Render(%q) = %v", source, lines)
		}
	}
}
