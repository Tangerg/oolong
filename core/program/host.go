package program

import (
	"errors"
	"image"
	"time"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/term"
)

// EventSource is one ordered input stream and its terminal result.
//
// Events closes after the last event. Once it has closed, Err reports why the
// stream ended: nil means a clean end of input, while a non-nil error is the
// transport failure that ended it. Err must not report end-of-file as a failure.
//
// The two methods form one lifecycle. Keeping the result on the source avoids a
// race between an event channel closing and a separate error channel becoming
// readable, and follows the same iteration-then-error shape as a scanner.
type EventSource interface {
	Events() <-chan input.Event
	Err() error
}

// Host is where a program's input comes from and its frames go.
//
// A program opens the real terminal unless it is given one of these. Being able to
// supply it is what lets an interface be driven and inspected in a test, with no
// terminal in sight.
//
// Everything beyond transport is optional. Each independently useful operation is
// represented by a small consumer interface such as [GroundHost], [CopyHost] or
// [NotifyHost], so implementing one never silently depends on implementing its
// neighbours. [ImageHost] is the exception because its transport and geometry form
// one protocol. Absent capabilities receive harmless defaults.
type Host interface {
	// Input is the ordered input stream and the reason it eventually ends.
	Input() EventSource
	// Writer is where frames go. The interface is defined here, where it is used;
	// a host is not coupled to the terminal package's concrete writer.
	Writer() FrameWriter
	// Size is the terminal's size in cells. The result must satisfy [ValidateSize].
	Size() (w, h int, err error)
}

// FrameWriter is the part of a frame queue the program needs.
//
// It is defined by the consumer rather than exposing [term.Writer] through [Host].
// Implementations must preserve queue order, report successful completion as a
// watermark and be safe for concurrent use. Changes returns one stable,
// single-consumer channel for the writer's lifetime; a receive means Written or Err
// may have changed. Closing it means the writer has permanently stopped and Err must
// report the cause. Drain returns nil only after every frame accepted before the call
// has either been written or accounted for. [term.Writer] is the standard implementation.
type FrameWriter interface {
	// Queue takes ownership of frame. The caller does not read or change the slice
	// after the call; an asynchronous implementation may retain it without copying.
	// Every accepted call returns a non-zero sequence strictly greater than earlier
	// sequences from the same writer. Written reports a watermark in that sequence
	// space.
	Queue(frame []byte) uint64
	// Changes coalesces state changes; the receiver must re-read Written and Err.
	Changes() <-chan struct{}
	Written() uint64
	Err() error
	Drain(timeout time.Duration) error
}

// GroundHost supplies the colours discovered before a program starts.
type GroundHost interface{ Ground() grid.Ground }

// WheelHost supplies the host's wheel-event scale.
type WheelHost interface{ Wheel() input.Wheel }

// KeyboardHost supplies keyboard protocol features negotiated with the host.
type KeyboardHost interface {
	Keyboard() (input.KeyboardFeatures, bool)
}

// LocaleHost supplies the character locale of the user-facing terminal.
type LocaleHost interface{ Locale() string }

// DirectoryHost tells a terminal how to resolve relative paths in program output.
type DirectoryHost interface {
	ReportDirectory(path string) error
}

// CopyHost writes to the clipboard associated with the user-facing host.
type CopyHost interface{ Copy(text string) bool }

// PasteHost requests text from the clipboard associated with the user-facing host.
// Paste reports whether the request was accepted; answers arrive asynchronously
// through [EventSource.Events] as an [input.Paste].
type PasteHost interface{ Paste() bool }

// HandoverHost temporarily gives exclusive ownership of its display to run.
type HandoverHost interface {
	Hand(run func() error) error
}

// TitleHost names the user-facing host window.
type TitleHost interface{ SetTitle(title string) }

// ProgressHost presents task progress outside the interface's cell grid.
type ProgressHost interface{ SetProgress(progress term.Progress) }

// BellHost rings the user-facing host's audible or visible bell.
type BellHost interface{ Bell() }

// NotifyHost sends a notification through the user-facing host.
type NotifyHost interface{ Notify(text string) }

// ImageHost transmits images and reports the protocol geometry needed to place
// them. These methods form one capability: a handle from Transmit cannot be placed
// without the protocol and cell geometry that interpret it.
type ImageHost interface {
	Graphics() graphics.Protocol
	CellSize() (image.Point, bool)
	Transmit(png []byte) (graphics.Image, error)
}

// hostServices is the resolved set of optional host capabilities. It owns the
// fallback semantics, so the event loop invokes ordinary methods and never repeats
// type assertions or spreads nil checks through its state machine.
type hostServices struct {
	groundHost   GroundHost
	wheelHost    WheelHost
	keyboardHost KeyboardHost
	localeHost   LocaleHost
	directory    DirectoryHost
	copyHost     CopyHost
	pasteHost    PasteHost
	handover     HandoverHost
	titleHost    TitleHost
	progressHost ProgressHost
	bellHost     BellHost
	notifyHost   NotifyHost
	images       ImageHost
}

func hostServicesFor(host Host) hostServices {
	services := hostServices{}
	services.groundHost, _ = host.(GroundHost)
	services.wheelHost, _ = host.(WheelHost)
	services.keyboardHost, _ = host.(KeyboardHost)
	services.localeHost, _ = host.(LocaleHost)
	services.directory, _ = host.(DirectoryHost)
	services.copyHost, _ = host.(CopyHost)
	services.pasteHost, _ = host.(PasteHost)
	services.handover, _ = host.(HandoverHost)
	services.titleHost, _ = host.(TitleHost)
	services.progressHost, _ = host.(ProgressHost)
	services.bellHost, _ = host.(BellHost)
	services.notifyHost, _ = host.(NotifyHost)
	services.images, _ = host.(ImageHost)
	return services
}

func (s hostServices) ground() grid.Ground {
	if s.groundHost == nil {
		return grid.Ground{}
	}
	return s.groundHost.Ground()
}

func (s hostServices) wheel() input.Wheel {
	if s.wheelHost == nil {
		return input.Wheel{}
	}
	return s.wheelHost.Wheel()
}

func (s hostServices) keyboard() (input.KeyboardFeatures, bool) {
	if s.keyboardHost == nil {
		return 0, false
	}
	return s.keyboardHost.Keyboard()
}

func (s hostServices) locale() string {
	if s.localeHost == nil {
		return ""
	}
	return s.localeHost.Locale()
}

func (s hostServices) reportDirectory(path string) error {
	if s.directory == nil {
		return nil
	}
	return s.directory.ReportDirectory(path)
}

func (s hostServices) copy(text string) bool {
	return s.copyHost != nil && s.copyHost.Copy(text)
}

func (s hostServices) paste() bool {
	return s.pasteHost != nil && s.pasteHost.Paste()
}

func (s hostServices) hand(run func() error) error {
	if run == nil {
		return nil
	}
	if s.handover == nil {
		return run()
	}
	return s.handover.Hand(run)
}

func (s hostServices) canHandOver() bool { return s.handover != nil }

func (s hostServices) setTitle(title string) {
	if s.titleHost != nil {
		s.titleHost.SetTitle(title)
	}
}

func (s hostServices) setProgress(progress term.Progress) {
	if s.progressHost != nil {
		s.progressHost.SetProgress(progress)
	}
}

func (s hostServices) bell() {
	if s.bellHost != nil {
		s.bellHost.Bell()
	}
}

func (s hostServices) notify(text string) {
	if s.notifyHost != nil {
		s.notifyHost.Notify(text)
	}
}

func (s hostServices) graphics() graphics.Protocol {
	if s.images == nil {
		return graphics.None
	}
	return s.images.Graphics()
}

func (s hostServices) cellSize() (image.Point, bool) {
	if s.images == nil {
		return image.Point{}, false
	}
	return s.images.CellSize()
}

func (s hostServices) transmit(png []byte) (graphics.Image, error) {
	if s.images == nil {
		return graphics.Image{}, errors.ErrUnsupported
	}
	return s.images.Transmit(png)
}
