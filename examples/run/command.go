package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// The command owns its Unix process group or Windows job, and merged output,
// until shutdown completes. Unix descendants that leave the group and Windows
// processes created through unrelated brokers fall outside that membership.
// Stdin stays disconnected because terminal input belongs to the inline program.
type commandProcess struct {
	cmd    *exec.Cmd
	output io.Writer
	// Both streams share one copier. Each platform joins it before run returns,
	// so its failure survives even when exec.Wait prefers an unsuccessful exit.
	outputErr error
}

func (c *commandProcess) Write(data []byte) (int, error) {
	if c.outputErr != nil {
		return 0, c.outputErr
	}
	n, err := c.output.Write(data)
	switch {
	case n < 0 || n > len(data):
		err = errors.Join(fmt.Errorf("run: output returned %d bytes for a %d-byte write", n, len(data)), err)
		n = 0
	case n < len(data) && err == nil:
		err = io.ErrShortWrite
	}
	c.outputErr = err
	return n, err
}

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
	if output == nil {
		output = io.Discard
	}
	command := &commandProcess{cmd: cmd, output: output}
	cmd.Stdout, cmd.Stderr = command, command
	err := command.run(ctx)
	if command.outputErr != nil && !errors.Is(err, command.outputErr) {
		err = errors.Join(command.outputErr, err)
	}
	if cause := context.Cause(ctx); cause != nil && !errors.Is(err, cause) {
		return errors.Join(cause, err)
	}
	return err
}
