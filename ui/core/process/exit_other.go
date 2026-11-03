//go:build !darwin && !linux && !dragonfly && !freebsd && !netbsd

package process

import (
	"errors"
	"os"
)

const supported = false

func observeExit(int) (int, os.Signal, error) { return 0, nil, errors.ErrUnsupported }
