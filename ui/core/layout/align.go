package layout

import (
	"errors"
	"fmt"
)

type Align uint8

const (
	Start Align = iota
	Center
	End
)

var ErrInvalidAlign = errors.New("layout: invalid alignment")

func (a Align) Validate() error {
	if a > End {
		return fmt.Errorf("%w: %d", ErrInvalidAlign, a)
	}
	return nil
}

// Offset panics for invalid alignment even without available space, so collapsing
// geometry cannot hide malformed configuration.
func (a Align) Offset(space, extent int) int {
	if err := a.Validate(); err != nil {
		panic(err)
	}
	switch a {
	case Center:
		return Remaining(space, extent) / 2
	case End:
		return Remaining(space, extent)
	default:
		return 0
	}
}
