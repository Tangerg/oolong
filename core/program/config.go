package program

import (
	"errors"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/term"
)

// DefaultFrameInterval is the shortest time between program redraws. A terminal
// cannot usefully show more, and a stream of updates would otherwise ask for a frame
// each.
const DefaultFrameInterval = 16 * time.Millisecond

// Config is what a program needs to run.
//
// Exactly one of Root and Inline says what to run, and which one it is decides
// where the interface is drawn: Root takes a screen of its own, Inline draws in the
// terminal's own screen and prints finished output into its scrollback.
type Config struct {
	// Root builds the component to run on a screen of its own. It is given the runtime
	// first, so the component can hold it from the moment it exists. Returning nil is
	// an error.
	Root func(*Runtime) Component

	// Inline builds the component to run as a block in the terminal's own screen,
	// with output that is finished printed above it. Its component is given an
	// [InlineRuntime], which is a [Runtime] that can also print.
	// Returning nil is an error.
	Inline func(*InlineRuntime) Component

	// Terminal says which of the terminal's optional behaviours to ask for. Ignored
	// when Host is set.
	//
	// AltScreen is the program's to decide rather than the caller's, because where
	// frames go is the rendering model and not an input capability: it follows from
	// which of Root and Inline was set. Asking for it alongside Inline is a
	// contradiction and is reported as one.
	Terminal term.Options

	// Color says how much colour the terminal can show. The zero value, [grid.Auto],
	// asks [term.DetectDepth] — which is the one thing in this library that reads
	// its environment rather than making a request and letting it be ignored,
	// because a truecolor sequence a terminal cannot read prints wrong rather than
	// degrading.
	//
	// Setting it is how a program that already knows — from its own configuration,
	// or because it is writing to something that is not a terminal at all — takes
	// that decision back.
	Color grid.Depth

	// Host overrides where input comes from and frames go. Nil opens the real terminal
	// and gives it back on the way out.
	Host Host

	// FrameInterval is the shortest time between redraws. Zero uses
	// [DefaultFrameInterval]; a negative duration is invalid.
	FrameInterval time.Duration
}

// Validate reports contradictions in c without opening a terminal or invoking a
// component builder. Transport adapters call it before acquiring their own session
// resources; Run calls it as well, so there is one definition of a runnable
// configuration.
func (c Config) Validate() error {
	if (c.Root == nil) == (c.Inline == nil) {
		return errors.New("program: exactly one of Root and Inline is required")
	}
	if c.Inline != nil && c.Terminal.AltScreen {
		return errors.New("program: an inline interface cannot take the alternate screen")
	}
	if c.FrameInterval < 0 {
		return errors.New("program: frame interval cannot be negative")
	}
	return nil
}
