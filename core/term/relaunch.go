package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ErrRelaunch is reported when a relaunch could not be started at all, as opposed
// to starting and then failing on its own terms.
var ErrRelaunch = errors.New("term: relaunch")

// Relaunch starts argv again in place of this process, keeping the terminal.
//
// It is what a session does to change something only startup can decide — which
// screen it draws on, above all: the alternate screen and the terminal's own
// scrollback are different rendering models, and the second needs the scroll region
// from the beginning.
//
// The caller must give the terminal back first. [Terminal.Close] and every deferred
// call that a normal exit would have run has to have run, because this cannot know
// which terminal was taken over, and relaunching from raw mode hands the next run a
// mode it did not set and will not restore. Rebuilding argv is the caller's too.
//
// On success it does not return on Unix, where exec replaces the process image. On
// Windows it returns the exit code of the process it ran, so the same three lines
// work everywhere and the exit is unreachable on Unix:
//
//	code, err := term.Relaunch(argv, nil)
//	if err != nil {
//		return err
//	}
//	os.Exit(code)
//
// A nil env inherits this process's environment. A name given more than once keeps
// its last value on every platform: appending to os.Environ leaves the old value in
// front of the new one, exec would hand the kernel both and let the first win, and a
// relaunch that kept the state it was leaving would relaunch forever.
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
		start := 0
		if strings.HasPrefix(entry, "=") {
			start = 1
		}
		suffix, _, found := strings.Cut(entry[start:], "=")
		name := entry[:start] + suffix
		if runtime.GOOS == "windows" {
			name = strings.ToUpper(name)
		}
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
