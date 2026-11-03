//go:build darwin || linux

package process

import (
	"fmt"
	"os"
	"syscall"
)

const (
	childExited = 1
	childKilled = 2
	childDumped = 3
)

func childOutcome(code, status int32) (int, os.Signal, error) {
	switch code {
	case childExited:
		return int(status), nil, nil
	case childKilled, childDumped:
		return -1, syscall.Signal(status), nil
	default:
		return 0, nil, fmt.Errorf("process: unexpected child exit code %d", code)
	}
}
