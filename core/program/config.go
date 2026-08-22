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

	// Terminal says which optional terminal features to request. Run applies them
	// when opening the local terminal; a transport adapter may interpret them before
	// supplying Host. A Host passed directly to Run already owns its setup and
	// receives none of these settings.
	//
	// Screen ownership is deliberately absent. Root owns the alternate screen and
	// Inline owns the terminal's ordinary screen, so the rendering model has exactly
	// one entry point and cannot be contradicted by terminal configuration.
	Terminal term.Features

	// Color says how much colour the terminal can show. The zero value, [grid.Auto],
	// asks the host's optional [ColorHost]. A local terminal derives that answer with
	// [term.DetectDepth]; a host without the capability safely uses no colour.
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

// TerminalConfig derives the complete terminal-session configuration selected by c.
//
// It is the adapter boundary between a program's rendering model and a concrete
// terminal transport. Root owns the alternate screen, Inline owns the ordinary
// screen, and Terminal contributes only optional features. Keeping that projection
// here gives local, SSH, and future transports one answer instead of making each
// reconstruct the ownership rule.
//
// Call [Config.Validate] before acquiring transport resources; this projection does
// not make an otherwise invalid root selection runnable.
func (c Config) TerminalConfig() term.Config {
	return term.Config{Features: c.Terminal, AltScreen: c.Root != nil}
}

// Validate reports contradictions in c without opening a terminal or invoking a
// component builder. Transport adapters call it before acquiring their own session
// resources; Run calls it as well, so there is one definition of a runnable
// configuration.
func (c Config) Validate() error {
	if (c.Root == nil) == (c.Inline == nil) {
		return errors.New("program: exactly one of Root and Inline is required")
	}
	if c.FrameInterval < 0 {
		return errors.New("program: frame interval cannot be negative")
	}
	return nil
}
