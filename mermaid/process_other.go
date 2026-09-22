//go:build !unix && !windows

package mermaid

import (
	"context"
	"errors"
	"os/exec"
)

func platformSupported() error             { return errors.ErrUnsupported }
func run(context.Context, *exec.Cmd) error { return errors.ErrUnsupported }
