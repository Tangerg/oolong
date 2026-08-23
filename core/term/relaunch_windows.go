//go:build windows

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// consoleHandover is how long the new process is held back.
//
// A Windows console has one queue of input records and no way to hand it over. The
// reader this process started outlives the request to stop — a blocking read cannot
// be cancelled portably — so for a moment there are two readers, and a record taken
// by the one on its way out is a keystroke the new process never sees. Waiting is
// the only lever there is.
const consoleHandover = 150 * time.Millisecond

// relaunch runs the program again beside this one, because a Windows process cannot
// be replaced.
//
// It waits for the child rather than returning immediately, so the shell that
// started this process does not begin competing for the console while the new
// interface is drawing in it. The exit code comes back for the caller to exit with.
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

	time.Sleep(consoleHandover)
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
