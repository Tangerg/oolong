// Package program owns terminal lifetime, frame scheduling and the interface
// goroutine. Background work enters through [Dispatcher], so interface state needs
// no locks. The driver parks until input, work or writer progress arrives; timers
// are explicit. Inline programs publish finished content to terminal scrollback
// through [InlineRuntime.Publish].
package program

import (
	"errors"
	"fmt"
)

// ErrStopped means the program lost its interface owner before pending work could
// be applied. Unapplied callbacks and ingress data are released on shutdown.
var ErrStopped = errors.New("program: stopped")

// ErrFrameTimeout means a frame writer did not account for its pending frames
// before display ownership had to change. The program refuses the transition: a
// late frame would otherwise be written into the next owner's output.
var ErrFrameTimeout = errors.New("program: frame writer did not drain")

// ErrInvalidFrameSequence means a host accepted a non-empty frame without assigning
// it a usable position in its progress watermark. Continuing would allow a later
// frame to overtake output the presenter still owns, so publication stops instead.
var ErrInvalidFrameSequence = errors.New("program: invalid frame writer sequence")

// ErrDisplayFailed means a frame or the transport carrying it has already failed, so
// the display can no longer be left in a state anything else could take over. It is
// what [Session.Hand] refuses with: a child given a display nobody can describe is
// worse than a handover that did not happen.
var ErrDisplayFailed = errors.New("program: display failed")

// ErrInvalidSize means a host reported geometry that cannot safely back a program
// surface. Hosts are transport boundaries and their dimensions may come from an
// untrusted peer, so invalid input is an error rather than a grid allocation or
// panic.
var ErrInvalidSize = errors.New("program: invalid host size")

// MaxCells is the largest host-controlled program surface. A screen owns both a
// front and back cell store, so a bound belongs at the host edge before either is
// allocated. The limit admits terminals far larger than ordinary displays while
// keeping one resize from becoming an open-ended memory request.
const MaxCells = 1 << 18

// ValidateSize reports whether width and height describe a safe non-empty program
// surface. Host adapters can call it before acquiring transport resources; Run
// applies it to both the opening size and every later resize.
func ValidateSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: %dx%d", ErrInvalidSize, width, height)
	}
	if width > MaxCells/height {
		return fmt.Errorf("%w: %dx%d exceeds %d cells", ErrInvalidSize,
			width, height, MaxCells)
	}
	return nil
}
