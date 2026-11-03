package main

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

// The command owns its descendants and merged output until shutdown completes.
// Stdin stays disconnected because terminal input belongs to the inline program.
type commandProcess struct{ cmd *exec.Cmd }

func runCommand(ctx context.Context, arguments []string, output io.Writer) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if len(arguments) == 0 {
		return errors.New("run: command is empty")
	}
	//nolint:noctx,gosec // The platform owner installs group/job membership at creation and owns cancellation; arguments are the user's command line.
	cmd := exec.Command(arguments[0], arguments[1:]...)
	if cmd.Err != nil {
		return cmd.Err
	}
	cmd.Stdout, cmd.Stderr = output, output
	return (&commandProcess{cmd: cmd}).run(ctx)
}
