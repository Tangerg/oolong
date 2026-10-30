package ssh

import (
	"errors"
	"fmt"
	"io"
	"sync"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/term"
)

// Host is the one transport boundary. In addition to the required program.Host
// methods it exposes colour, locale, wheel profile and clipboard determined by the
// client terminal's environment. An SSH PTY does not prove notification, image or
// probe capabilities merely by existing, so those remain absent.
// A Host must not be copied after Open.
type Host struct {
	source *eventSource
	writer *term.Writer
	modes  term.Modes
	clip   *clipboard.Channel
	locale string
	color  grid.Depth
	wheel  input.Wheel

	windowMu sync.RWMutex
	window   charmssh.Window

	closeOnce sync.Once
	closeErr  error
}

var (
	_ program.Host       = (*Host)(nil)
	_ program.ColorHost  = (*Host)(nil)
	_ program.LocaleHost = (*Host)(nil)
	_ program.WheelHost  = (*Host)(nil)
	_ program.CopyHost   = (*Host)(nil)
	_ program.PasteHost  = (*Host)(nil)
)

func newHost(
	cancelled <-chan struct{},
	channel io.ReadWriter,
	window charmssh.Window,
	windows <-chan charmssh.Window,
	modes term.Modes,
	clip *clipboard.Channel,
	locale string,
	color grid.Depth,
	wheel input.Wheel,
) *Host {
	h := &Host{
		writer: term.NewWriter(term.WriterConfig{Destination: channel}),
		modes:  modes,
		clip:   clip,
		locale: locale,
		color:  color,
		wheel:  wheel,
		window: window,
	}
	_, _ = h.writer.QueueControl([]byte(h.modes.Enter()))
	h.source = newEventSource(cancelled, channel, windows, h.resize, h.clip)
	return h
}

// Input is the ordered client input and PTY resize stream owned by this host.
func (h *Host) Input() program.EventSource { return h.source }

// Writer is the same bounded FIFO used for session modes and application frames.
func (h *Host) Writer() program.FrameWriter { return h.writer }

// Locale is the accepted session's locale, frozen when the host was opened.
func (h *Host) Locale() string { return h.locale }

// Color is the capability inferred from the accepted client's terminal type.
func (h *Host) Color() grid.Depth { return h.color }

// Wheel describes the client's negotiated wheel input convention.
func (h *Host) Wheel() input.Wheel { return h.wheel }

// Copy asks the client terminal to update the clipboard beside the user, not one
// attached to the remote server process.
func (h *Host) Copy(text string) bool {
	sequence, ok := h.clip.Copy(clipboard.System, text)
	if !ok {
		return false
	}
	_, err := h.writer.Queue([]byte(sequence))
	return err == nil
}

// Paste asks the client terminal for that same clipboard. A valid answer is
// translated by eventSource into the ordinary input.Paste event components already
// consume.
func (h *Host) Paste() bool {
	return h.clip.Request(clipboard.System, func(sequence string) bool {
		_, err := h.writer.Queue([]byte(sequence))
		return err == nil
	})
}

// Size reads the latest accepted PTY dimensions and is safe during resize delivery.
func (h *Host) Size() (width, height int, _ error) {
	h.windowMu.RLock()
	defer h.windowMu.RUnlock()
	return h.window.Width, h.window.Height, nil
}

// resize applies the RFC rule that a zero dimension leaves that dimension alone.
// A positive update is published only after Size observes it, so handling the
// resulting event and asking the host can never disagree about one window.
func (h *Host) resize(next charmssh.Window) (input.Resize, bool, error) {
	h.windowMu.Lock()
	defer h.windowMu.Unlock()

	if next.Width < 0 || next.Height < 0 {
		return input.Resize{}, false, fmt.Errorf("%w: %dx%d", ErrWindowSize,
			next.Width, next.Height)
	}
	window := h.window
	if next.Width > 0 {
		window.Width = next.Width
	}
	if next.Height > 0 {
		window.Height = next.Height
	}
	if err := program.ValidateSize(window.Width, window.Height); err != nil {
		return input.Resize{}, false, fmt.Errorf("%w: %w", ErrWindowSize, err)
	}
	if window.Width == h.window.Width && window.Height == h.window.Height {
		return input.Resize{}, false, nil
	}
	h.window = window
	return input.Resize{Width: window.Width, Height: window.Height}, true, nil
}

// Close settles output in ownership order: stop accepting input, queue the inverse
// modes after the final frame, then close the writer that owns both. The SSH channel
// remains the caller's so its handler can still send the chosen exit status.
//
// Stopping input stops the decoding of it. Reading the channel is the channel's own
// lifetime — see [eventSource.Close] — so nothing here is waiting for a client that
// may say nothing more.
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		h.source.Close()
		h.writer.CloseAdmission()
		if _, err := h.writer.QueueControl([]byte(h.modes.Leave())); err != nil {
			h.closeErr = err
		}
		if err := h.writer.Close(); err != nil {
			h.closeErr = errors.Join(h.closeErr, fmt.Errorf("ssh: close frame writer: %w", err))
		}
		if err := h.writer.Err(); err != nil {
			h.closeErr = errors.Join(h.closeErr, fmt.Errorf("ssh: write session: %w", err))
		}
	})
	return h.closeErr
}
