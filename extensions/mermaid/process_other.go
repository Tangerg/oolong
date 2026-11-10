//go:build !unix && !windows

package mermaid

import (
	"context"
	"errors"
)

func platformSupported() error          { return errors.ErrUnsupported }
func run(context.Context, launch) error { return errors.ErrUnsupported }
