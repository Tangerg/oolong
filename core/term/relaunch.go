package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrRelaunch is reported when a relaunch could not be started at all, as opposed
// to starting and then failing on its own terms.
var ErrRelaunch = errors.New("term: relaunch")

// Relaunch starts argv again in place of this process, keeping the terminal.
//
// It is what a session does to change something only startup can decide — which
// screen it draws on, above all. A program cannot move an interface from the
// alternate screen into the terminal's own scrollback while it is running: the two
// are different rendering models, and the second needs the terminal's own scroll
// region from the beginning. Starting again is the answer, and starting again
// without the user losing their window, its size, or the output already in it means
// keeping the terminal.
//
// The terminal is kept because the file descriptors are. On Unix it is the same
// process as well: exec replaces the image rather than starting something beside
// it, so there is no second process, no shell waiting on one, and nothing to lose
// a signal on the way through.
//
// # What the caller has to do first
//
// Give the terminal back. [Terminal.Close], and every deferred call that would have
// run on the way out of a normal exit, has to have run. This does not do it: this
// package cannot know which terminal a caller took over, and a process that
// relaunched while still in raw mode hands the next one a mode it did not set,
// did not expect, and will not restore.
//
// # What it returns
//
// On success it does not return on Unix, and on Windows it returns the exit code of
// the process it ran, because a Windows process cannot be replaced. So a caller
// writes the same three lines everywhere and the exit is unreachable on Unix:
//
//	code, err := term.Relaunch(argv, nil)
//	if err != nil {
//		return err
//	}
//	os.Exit(code)
//
// Rebuilding argv is the caller's. Which flags to keep, which to drop, and how a
// session says which mode it is resuming in are decisions about a program's command
// line, and a library that made them would be a framework for one program.
//
// A nil env inherits this process's environment. Passing one is how a program tells
// the next run something it can only be told at startup — which is the usual reason
// to relaunch at all.
//
// A name given more than once keeps its last value, whichever platform this is. That
// is not a nicety. The obvious way to set a variable for the next run is to append
// to os.Environ, and appending leaves the old value in front of the new one: exec
// hands the pair to the kernel untouched and the first one wins, while the Windows
// path deduplicates and the second one wins. The same call would behave differently
// on two platforms, and the Unix one would send a program back into the state it was
// trying to leave — which, for a relaunch, means doing it again forever.
func Relaunch(argv, env []string) (code int, err error) {
	if len(argv) == 0 || argv[0] == "" {
		return 0, fmt.Errorf("%w: no program to run", ErrRelaunch)
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrRelaunch, err)
	}
	if env == nil {
		env = os.Environ()
	}
	return relaunch(path, argv, lastWins(env))
}

// lastWins drops all but the final value of each name, keeping the order the names
// first appeared in.
func lastWins(env []string) []string {
	at := make(map[string]int, len(env))
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			// Not a setting. Passed through rather than dropped: it is not this
			// function's place to decide what a caller's environment may contain.
			out = append(out, entry)
			continue
		}
		if i, seen := at[name]; seen {
			out[i] = entry
			continue
		}
		at[name] = len(out)
		out = append(out, entry)
	}
	return out
}
