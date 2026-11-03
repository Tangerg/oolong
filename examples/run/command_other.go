//go:build !unix && !windows

package main

import (
	"context"
	"errors"
)

func (*commandProcess) run(context.Context) error { return errors.ErrUnsupported }
