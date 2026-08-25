package term_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/term"
)

// stageEnv is how the helper below knows which run of itself it is.
const stageEnv = "OOLONG_RELAUNCH_STAGE"

// TestRelaunchHelper is not a test. It is the program the real test runs, in a
// process of its own, because a relaunch replaces the process that asks for one and
// there would be no test left to report the result.
//
// It reports which run it is and what process it is, then relaunches itself once.
func TestRelaunchHelper(t *testing.T) {
	stage := os.Getenv(stageEnv)
	if stage == "" {
		t.Skip("not the helper: run by TestRelaunch")
	}
	fmt.Printf("stage=%s pid=%d\n", stage, os.Getpid())
	if stage != "1" {
		return
	}

	env := append(os.Environ(), stageEnv+"=2")
	code, err := term.Relaunch(os.Args, env)
	if err != nil {
		fmt.Printf("relaunch failed: %v\n", err)
		os.Exit(1)
	}
	// Reached on Windows, where a process cannot be replaced and the exit code of
	// the one that ran comes back instead. Unreachable everywhere else.
	os.Exit(code)
}

// TestRelaunchStartsTheProgramAgain is the whole claim, checked from outside: the
// program runs twice, and on Unix it is the same process both times.
//
// The pid is what distinguishes this from starting a child. A relaunch that spawned
// would keep the terminal too, but it would leave a parent waiting, a shell watching
// the wrong process, and signals going to something that is no longer drawing.
func TestRelaunchStartsTheProgramAgain(t *testing.T) {
	// Bounded by the test's own context, so a relaunch that looped would fail the
	// test rather than hang the run — which is how the environment bug behind
	// lastWins was found.
	cmd := exec.CommandContext(t.Context(), os.Args[0], //nolint:gosec // G204: this test binary.
		"-test.run=TestRelaunchHelper")
	cmd.Env = append(os.Environ(), stageEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		var diagnostic []byte
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			diagnostic = exit.Stderr
		}
		t.Fatalf("the helper failed: %v\nstdout:\n%s\nstderr:\n%s", err, out, diagnostic)
	}

	var stages, pids []string
	for line := range strings.SplitSeq(string(out), "\n") {
		rest, ok := strings.CutPrefix(line, "stage=")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) != 2 {
			t.Fatalf("unreadable line %q in:\n%s", line, out)
		}
		stages = append(stages, fields[0])
		pids = append(pids, strings.TrimPrefix(fields[1], "pid="))
	}
	if len(pids) != 2 {
		t.Fatalf("the program ran %d times, want 2:\n%s", len(pids), out)
	}
	// The second run saw the value the first one set, which is the environment
	// question: the test appends to os.Environ, so the name is in there twice and
	// only the last value ending the loop proves which one won.
	if stages[0] != "1" || stages[1] != "2" {
		t.Fatalf("the runs saw stages %v, want [1 2]:\n%s", stages, out)
	}
	if runtime.GOOS == "windows" {
		return // a process cannot be replaced there, only run beside
	}
	if pids[0] != pids[1] {
		t.Errorf("%s then %s: the process was not replaced, it was spawned", pids[0], pids[1])
	}
}

func TestRelaunchRefusesWhatItCannotStart(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"nothing at all", nil},
		{"an empty name", []string{""}},
		{"a program that is not there", []string{"oolong-no-such-program-anywhere"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := term.Relaunch(tc.argv, nil); err == nil {
				t.Fatal("no error")
			} else if !strings.Contains(err.Error(), "relaunch") {
				t.Errorf("error %q does not say what failed", err)
			}
		})
	}
}
