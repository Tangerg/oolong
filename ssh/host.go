package ssh

import (
	"errors"
	"fmt"
	"io"
	"sync"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

// host is the one transport boundary. In addition to the required program.Host
// methods it exposes the clipboard that OSC 52 reaches through the client terminal.
// An SSH PTY does not prove notification, image or probe capabilities merely by
// existing, so those remain absent.
type host struct {
	source *eventSource
	writer *term.Writer
	modes  term.Modes
	clip   *clipboard.Channel

	windowMu sync.RWMutex
	window   charmssh.Window

	closeOnce sync.Once
	closeErr  error
}

var (
	_ program.Host      = (*host)(nil)
	_ program.CopyHost  = (*host)(nil)
	_ program.PasteHost = (*host)(nil)
)

func newHost(
	cancelled <-chan struct{},
	channel io.ReadWriter,
	window charmssh.Window,
	windows <-chan charmssh.Window,
	modes term.Modes,
	clip *clipboard.Channel,
) *host {
	h := &host{
		writer: term.NewWriter(channel),
		modes:  modes,
		clip:   clip,
		window: window,
	}
	h.writer.Queue([]byte(h.modes.Enter()))
	h.source = newEventSource(cancelled, channel, windows, h.resize, h.clip)
	return h
}

func (h *host) Input() program.EventSource  { return h.source }
func (h *host) Writer() program.FrameWriter { return h.writer }

// Copy asks the client terminal to update the clipboard beside the user, not one
// attached to the remote server process.
func (h *host) Copy(text string) bool {
	sequence, ok := h.clip.Copy(clipboard.System, text)
	if ok {
		h.writer.Queue([]byte(sequence))
	}
	return ok
}

// Paste asks the client terminal for that same clipboard. A valid answer is
// translated by eventSource into the ordinary input.Paste event components already
// consume.
func (h *host) Paste() {
	if sequence, ok := h.clip.Request(clipboard.System); ok {
		h.writer.Queue([]byte(sequence))
	}
}

func (h *host) Size() (width, height int, _ error) {
	h.windowMu.RLock()
	defer h.windowMu.RUnlock()
	return h.window.Width, h.window.Height, nil
}

// resize applies the RFC rule that a zero dimension leaves that dimension alone.
// A positive update is published only after Size observes it, so handling the
// resulting event and asking the host can never disagree about one window.
func (h *host) resize(next charmssh.Window) (input.Resize, bool, error) {
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
func (h *host) Close() error {
	h.closeOnce.Do(func() {
		h.source.Close()
		h.writer.Queue([]byte(h.modes.Leave()))
		if err := h.writer.Close(); err != nil {
			h.closeErr = errors.Join(h.closeErr, fmt.Errorf("ssh: close frame writer: %w", err))
		}
		if err := h.writer.Err(); err != nil {
			h.closeErr = errors.Join(h.closeErr, fmt.Errorf("ssh: write session: %w", err))
		}
	})
	return h.closeErr
}
