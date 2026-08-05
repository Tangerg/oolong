//go:build !unix

package term

import (
	"errors"
	"fmt"
)

// Suspend reports [errors.ErrUnsupported] here.
//
// Stopping and being continued is a job-control idea, and job control is a Unix
// idea. Somewhere without it, a program that means "get out of the way for a
// moment" has to exit and be started again, which is a decision about a program
// rather than something a library can do on its behalf.
func Suspend() error {
	return fmt.Errorf("term: suspend: %w", errors.ErrUnsupported)
}
