//go:build !unix && !windows

package main

import (
	"context"
	"errors"
	"os/exec"
)

func editorCommand(context.Context, string, string) (*exec.Cmd, error) {
	return nil, errors.ErrUnsupported
}
