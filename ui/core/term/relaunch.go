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

// Relaunch requires terminal restoration and deferred cleanup before entry:
// Unix exec replaces the process, so those obligations cannot run afterwards.
// Windows runs and waits for a child, returning its exit code so the caller can
// exit without competing for the console.
//
// Nil env inherits the process environment. Repeated names keep their last
// value on every platform so appending an override cannot retain the setting
// that caused the relaunch.
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
