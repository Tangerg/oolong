package program

import (
	"errors"
	"image"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// Environment is the stable set of terminal facts learned before a program runs.
// Its zero value reports that nothing was learned.
type Environment struct{ host hostServices }

// services centralizes the zero Runtime contract for capability objects. Runtime
// operations that need the live owner still check it explicitly; a missing owner
// and a host with no optional services are equivalent only here.
func (r *Runtime) services() hostServices {
	if r == nil || r.p == nil {
		return hostServices{}
	}
	return r.p.host
}

// Environment returns the host facts available to this runtime.
func (r *Runtime) Environment() Environment {
	return Environment{host: r.services()}
}

// Ground reports the host's foreground and background colours when known.
func (e Environment) Ground() grid.Ground { return e.host.ground() }

// Wheel reports how host wheel events should be scaled.
func (e Environment) Wheel() input.Wheel { return e.host.wheel() }

// Keyboard reports negotiated keyboard protocol features.
func (e Environment) Keyboard() (input.KeyboardFlags, bool) { return e.host.keyboard() }

// Clipboard is the clipboard associated with the user-facing host. Its zero value
// refuses writes and ignores reads.
type Clipboard struct{ host hostServices }

// Clipboard returns the runtime's clipboard capability.
func (r *Runtime) Clipboard() Clipboard {
	return Clipboard{host: r.services()}
}

// Copy puts text on the host clipboard when supported.
func (c Clipboard) Copy(text string) bool { return c.host.copy(text) }

// Paste requests clipboard contents. A successful answer arrives as [input.Paste].
func (c Clipboard) Paste() { c.host.paste() }

// Session controls the live terminal session around rendered frames. It groups
// operations that must remain ordered with the interface owner's state. Its zero
// value performs harmless notification no-ops and can hand control to a callback,
// but cannot suspend a process.
//
// It holds the runtime rather than the resolved services the other capabilities
// hold, because two of its methods need the owner itself and not what the host can
// answer: see [Session.Hand].
type Session struct{ runtime *Runtime }

// Session returns the terminal-session capability owned by this runtime.
func (r *Runtime) Session() Session { return Session{runtime: r} }

// host is the services of a runtime that may not be there. [Runtime.services] takes
// a nil receiver, so a zero Session answers as a host with no optional services —
// which is the right answer for every method that only asks the host something.
//
// [Session.Hand] is not one of those and does not come through here.
func (s Session) host() hostServices {
	return s.runtime.services()
}

// ReportDirectory tells the host which directory relative links belong to.
func (s Session) ReportDirectory(path string) error { return s.host().reportDirectory(path) }

// Hand gives exclusive display ownership to run and repaints after it returns. If
// pending frames cannot drain, it returns [ErrFrameTimeout] without calling run.
func (s Session) Hand(run func() error) error {
	if run == nil {
		return nil
	}
	if s.runtime == nil || s.runtime.p == nil {
		return run()
	}
	p := s.runtime.p
	// Repaint is part of settling the handover, including when the child panics and
	// the host restores terminal ownership from a defer.
	defer p.present.RequestFull()
	if p.inline != nil && p.root != nil {
		if err := p.leaveBlock(); err != nil {
			p.frameFailed = true
			p.failure = err
			return err
		}
	}
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	if err := p.writer.Err(); err != nil {
		p.outputFailed = true
		p.failure = err
		return err
	}
	return p.host.hand(run)
}

// Suspend restores the terminal and stops the process until it is continued.
func (s Session) Suspend() error {
	if !s.host().canHandOver() {
		return errors.ErrUnsupported
	}
	return s.Hand(term.Suspend)
}

// SetTitle names the host window when supported.
func (s Session) SetTitle(title string) { s.host().setTitle(title) }

// Bell asks the host for the user's attention.
func (s Session) Bell() { s.host().bell() }

// Notify asks the host to display a desktop notification.
func (s Session) Notify(text string) { s.host().notify(text) }

// Images is the host's image transport. Its zero value reports no protocol and
// refuses transmission.
type Images struct{ host hostServices }

// Images returns the runtime's image capability.
func (r *Runtime) Images() Images {
	return Images{host: r.services()}
}

// Protocol reports the host's richest image protocol.
func (i Images) Protocol() graphics.Protocol { return i.host.graphics() }

// CellSize reports one terminal cell's pixel size when known.
func (i Images) CellSize() (image.Point, bool) { return i.host.cellSize() }

// Transmit sends one PNG and returns the image handle used by frames.
func (i Images) Transmit(png []byte) (graphics.Image, error) { return i.host.transmit(png) }
