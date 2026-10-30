//go:build unix

package term

import (
	"fmt"
	"syscall"
)

// relaunch replaces this process. It returns only on failure: on success there is
// no longer a caller to return to.
func relaunch(path string, argv, env []string) (int, error) {
	// G204: running a program named at runtime is the whole function. The name came
	// from the caller's own argv and was resolved by LookPath; there is no shell in
	// the path, so it is one argument and not a command line.
	if err := syscall.Exec(path, argv, env); err != nil { //nolint:gosec // running a program named at runtime is the function.
		return 0, fmt.Errorf("%w: exec %s: %w", ErrRelaunch, path, err)
	}
	// Unreachable. Exec either replaced this process or reported why it could not.
	return 0, nil
}
