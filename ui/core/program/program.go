// Package program owns terminal lifetime, frame scheduling and the interface
// goroutine. Background work enters through [Dispatcher], so interface state needs
// no locks. The driver parks until input, work or writer progress arrives; timers
// are explicit. Inline programs publish finished content to terminal scrollback
// through [InlineRuntime.Publish].
package program

import "errors"

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
