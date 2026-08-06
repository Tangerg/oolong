package program

import (
	"errors"
	"image"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// GroundHost supplies the colours discovered before a program starts.
type GroundHost interface{ Ground() grid.Ground }

// WheelHost supplies the host's wheel-event scale.
type WheelHost interface{ Wheel() input.Wheel }

// KeyboardHost supplies keyboard protocol features negotiated with the host.
type KeyboardHost interface {
	Keyboard() (input.KeyboardFlags, bool)
}

// DirectoryHost tells a terminal how to resolve relative paths in program output.
type DirectoryHost interface {
	ReportDirectory(path string) error
}

// CopyHost writes to the clipboard associated with the user-facing host.
type CopyHost interface{ Copy(text string) bool }

// PasteHost requests text from the clipboard associated with the user-facing
// host. Answers arrive asynchronously through [Host.Events] as an [input.Paste].
type PasteHost interface{ Paste() }

// HandoverHost temporarily gives exclusive ownership of its display to run.
type HandoverHost interface {
	Hand(run func() error) error
}

// TitleHost names the user-facing host window.
type TitleHost interface{ SetTitle(title string) }

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
	directory    DirectoryHost
	copyHost     CopyHost
	pasteHost    PasteHost
	handover     HandoverHost
	titleHost    TitleHost
	bellHost     BellHost
	notifyHost   NotifyHost
	images       ImageHost
}

func hostServicesFor(host Host) hostServices {
	services := hostServices{}
	services.groundHost, _ = host.(GroundHost)
	services.wheelHost, _ = host.(WheelHost)
	services.keyboardHost, _ = host.(KeyboardHost)
	services.directory, _ = host.(DirectoryHost)
	services.copyHost, _ = host.(CopyHost)
	services.pasteHost, _ = host.(PasteHost)
	services.handover, _ = host.(HandoverHost)
	services.titleHost, _ = host.(TitleHost)
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

func (s hostServices) keyboard() (input.KeyboardFlags, bool) {
	if s.keyboardHost == nil {
		return input.KeyboardFlags{}, false
	}
	return s.keyboardHost.Keyboard()
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

func (s hostServices) paste() {
	if s.pasteHost != nil {
		s.pasteHost.Paste()
	}
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
