package program

import (
	"context"
	"errors"
	"image"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/internal/known"
)

// Environment is the stable set of terminal facts learned before a program runs.
// Its zero value reports that nothing was learned.
type Environment struct{ facts *environmentFacts }

type environmentFacts struct {
	ground   grid.Ground
	color    grid.Depth
	wheel    input.Wheel
	keyboard known.Value[input.KeyboardFeatures]
	locale   string
}

func environmentFor(host hostServices) Environment {
	return Environment{facts: &environmentFacts{
		ground: host.ground(), color: host.color(), wheel: host.wheel(),
		keyboard: host.keyboard(), locale: host.locale(),
	}}
}

// unknownEnvironment is what a host with no optional capabilities reports;
// hostServices owns those fallbacks, so the zero Environment does not restate them.
var unknownEnvironment = environmentFor(hostServices{}).facts

func (e Environment) learned() *environmentFacts {
	if e.facts == nil {
		return unknownEnvironment
	}
	return e.facts
}

func (e Environment) Ground() grid.Ground { return e.learned().ground }

func (e Environment) Color() grid.Depth { return e.learned().color }

func (e Environment) Wheel() input.Wheel { return e.learned().wheel }

func (e Environment) Keyboard() (input.KeyboardFeatures, bool) { return e.learned().keyboard.Get() }

// Locale reports the character locale of the user-facing terminal. An empty string
// means the host made no claim; it does not mean the process locale should be used.
func (e Environment) Locale() string { return e.learned().locale }

// Clipboard is the clipboard associated with the user-facing host. A host without
// one refuses with [errors.ErrUnsupported]; a zero Clipboard, like one whose
// program has ended, refuses with [ErrStopped].
type Clipboard struct{ runtime *Runtime }

// Copy queues text for the host's clipboard. A nil error means it was queued, not
// that the terminal accepted it: terminals may refuse clipboard access silently.
func (c Clipboard) Copy(text string) error {
	host, err := c.runtime.live()
	if err != nil {
		return err
	}
	return host.copy(text)
}

// Read reserves completion capacity before starting a host read. receive runs
// once on this UI owner with the result or failure. Caller cancellation or runtime
// shutdown revokes the callback; ctx communicates cancellation to the caller.
// A returned error means no work was accepted. A nil callback is invalid.
func (c Clipboard) Read(ctx context.Context, receive func(string, error)) error {
	p := c.runtime.owner()
	if p == nil {
		return ErrStopped
	}
	reader := p.host.clipboardReader
	if reader == nil {
		return errors.ErrUnsupported
	}
	if receive == nil {
		return errors.New("program: clipboard read requires a receiver")
	}
	scope, err := c.runtime.NewScope(ScopeConfig{Context: ctx}) //nolint:contextcheck // ScopeConfig carries the caller's context into the existing lifetime owner.
	if err != nil {
		return err
	}
	_, err = c.runtime.Start(scope, Work[string]{
		Run: reader.ReadClipboard,
		Apply: func(value string, readErr error) {
			defer scope.Close()
			receive(value, readErr)
		},
	})
	if err != nil {
		scope.Close()
	}
	return err
}

// Session controls the live terminal session around rendered frames. It groups
// operations that must remain ordered with the interface owner's state. A running
// host without a capability returns [errors.ErrUnsupported] for it; directory
// reporting remains optional host metadata and succeeds silently instead. A zero
// Session, like one whose program has ended, returns [ErrStopped].
//
// Like Clipboard and Images, it retains the runtime lifetime, so a copied capability
// cannot continue changing an externally owned host after that program ends.
type Session struct{ runtime *Runtime }

// ReportDirectory tells the host which directory relative links belong to.
func (s Session) ReportDirectory(path string) error {
	host, err := s.runtime.live()
	if err != nil {
		return err
	}
	return host.reportDirectory(path)
}

// Hand gives exclusive display ownership to run and repaints after it returns. If
// pending frames cannot drain, it returns [ErrFrameTimeout] without calling run.
// It returns [ErrStopped] once the program has ended and [errors.ErrUnsupported]
// for a host that is not a [HandoverHost], in both cases without calling run or
// disturbing the display. Once a frame
// or the transport has failed it returns [ErrDisplayFailed] on the same terms: the
// display cannot be left in a describable state, so it is not given away and the
// failure is not made to happen a second time.
// A nil callback is a no-op.
func (s Session) Hand(run func() error) error {
	if run == nil {
		return nil
	}
	p := s.runtime.owner()
	if p == nil {
		return ErrStopped
	}
	if !p.host.canHandOver() {
		return errors.ErrUnsupported
	}
	if p.frameFailed || p.outputFailed {
		return ErrDisplayFailed
	}
	return p.host.hand(func() error {
		// Preparation belongs inside the host's claim: otherwise a refused nested
		// handover can draw the interface over a child that still owns the display.
		defer p.present.RequestFull()
		if p.inline != nil && p.root != nil {
			if err := p.leaveBlock(); err != nil {
				return p.fail(err)
			}
		}
		if err := p.drainWriter(); err != nil {
			return err
		}
		if err := p.writer.Err(); err != nil {
			p.outputFailed = true
			return p.fail(err)
		}
		p.root.InvalidateGeometry()
		return nil
	}, run)
}

// Suspend uses [Session.Hand] so terminal ownership is released before job
// control can stop this process. Hand owns admission and restoration; [term.Suspend]
// preserves the host's signal policy and owns the platform's delivery boundary.
func (s Session) Suspend() error { return s.Hand(term.Suspend) }

func (s Session) SetTitle(title string) error {
	host, err := s.runtime.live()
	if err != nil {
		return err
	}
	return host.setTitle(title)
}

// SetProgress presents task progress outside the cell grid when supported. The
// zero value clears an earlier value. It is independent of progress drawn inside
// the interface, which remains ordinary component state.
// Unknown states return [term.ErrInvalidProgressState] before the host is called.
func (s Session) SetProgress(progress term.Progress) error {
	if err := progress.State.Validate(); err != nil {
		return err
	}
	host, err := s.runtime.live()
	if err != nil {
		return err
	}
	return host.setProgress(progress)
}

func (s Session) Bell() error {
	host, err := s.runtime.live()
	if err != nil {
		return err
	}
	return host.bell()
}

func (s Session) Notify(text string) error {
	host, err := s.runtime.live()
	if err != nil {
		return err
	}
	return host.notify(text)
}

// Images is the host's image transport. Its zero value, like one whose program has
// ended, reports no protocol and refuses transmission and release with [ErrStopped].
//
// Unlike Session, its methods may be called from any goroutine: an upload waits on
// the terminal's answer, so a worker transmits, waits on the returned channel and
// hands the resource to the UI owner itself, without a detour through the owner to
// start the upload.
type Images struct{ runtime *Runtime }

func (i Images) Protocol() graphics.Protocol { return i.runtime.services().graphics() }

func (i Images) CellSize() (image.Point, bool) { return i.runtime.services().cellSize() }

func (i Images) Transmit(ctx context.Context, png []byte) (<-chan graphics.UploadResult, error) {
	host, err := i.runtime.live()
	if err != nil {
		return nil, err
	}
	return host.transmit(ctx, png)
}

// Release ends the transmitted image's lifetime. Remove all its placements before
// calling this; the handle must not be placed again without retransmission.
func (i Images) Release(img graphics.Resource) error {
	host, err := i.runtime.live()
	if err != nil {
		return err
	}
	return host.releaseImage(img)
}
