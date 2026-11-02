//go:build windows

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Windows cannot replace this process. Waiting for the child keeps the parent
// shell from reclaiming the console while the replacement interface runs.
func relaunch(path string, argv, env []string) (int, error) {
	// noctx: there is no context to honour. This stands in for replacing the
	// process, which on every other platform leaves nothing behind that could be
	// cancelled — a deadline here would end the program the user is now using.
	//
	// G204: running a program named at runtime is the whole function, and the name
	// came from the caller's own argv by way of LookPath.
	cmd := exec.Command(path, argv[1:]...) //nolint:noctx,gosec // no context to honour, and the name is the function.
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		// The program ran and decided how it ended, which is not this function
		// failing.
		return exit.ExitCode(), nil
	}
	return 0, fmt.Errorf("%w: run %s: %w", ErrRelaunch, path, err)
}
