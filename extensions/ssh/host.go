package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/term"
)

// Host resolves optional capabilities from the accepted client environment because
// an SSH PTY proves only cell I/O.
// A Host must not be copied after opening.
type Host struct {
	source *eventSource
	writer *term.Writer
	modes  term.Modes
	clip   *clipboard.Channel
	locale string
	color  grid.Depth
	wheel  input.Wheel

	windows *windowFeed

	closeOnce sync.Once
	closeErr  error
}

var (
	_ program.Host            = (*Host)(nil)
	_ program.ColorHost       = (*Host)(nil)
	_ program.LocaleHost      = (*Host)(nil)
	_ program.WheelHost       = (*Host)(nil)
	_ program.CopyHost        = (*Host)(nil)
	_ program.ClipboardReader = (*Host)(nil)
)

func newHost(
	cancelled <-chan struct{},
	channel io.ReadWriter,
	windows *windowFeed,
	modes term.Modes,
	clip *clipboard.Channel,
	locale string,
	color grid.Depth,
	wheel input.Wheel,
) (*Host, error) {
	writer := term.NewWriter(term.WriterConfig{Destination: channel})
	// A host whose modes were never entered must not later send their Leave.
	if _, err := writer.QueueControl([]byte(modes.Enter())); err != nil {
		return nil, errors.Join(fmt.Errorf("ssh: enter terminal modes: %w", err), writer.Close())
	}
	h := &Host{
		writer:  writer,
		modes:   modes,
		clip:    clip,
		locale:  locale,
		color:   color,
		wheel:   wheel,
		windows: windows,
	}
	h.source = newEventSource(cancelled, channel, windows, h.clip)
	return h, nil
}

func (h *Host) Input() program.EventSource { return h.source }

// Writer shares one FIFO with terminal modes so teardown cannot overtake accepted
// application output.
func (h *Host) Writer() program.FrameWriter { return h.writer.Frames() }

// Locale stays frozen for the host's lifetime to preserve the accepted session facts.
func (h *Host) Locale() string { return h.locale }

func (h *Host) Color() grid.Depth { return h.color }

func (h *Host) Wheel() input.Wheel { return h.wheel }

// Copy targets the user's client terminal because the server
// process does not own the user's local clipboard.
func (h *Host) Copy(text string) bool {
	sequence, ok := h.clip.Copy(clipboard.System, text)
	if !ok {
		return false
	}
	_, err := h.writer.Queue([]byte(sequence))
	return err == nil
}

// ReadClipboard addresses the remote user's terminal. A cancelled read retains
// its unanswered wire slot; a later query cannot adopt that reply.
func (h *Host) ReadClipboard(ctx context.Context) (string, error) {
	return h.clip.Read(ctx, clipboard.System, func(sequence string) error {
		_, err := h.writer.Queue([]byte(sequence))
		return err
	})
}

// Size uses window-feed synchronization to permit reads during resize delivery.
func (h *Host) Size() (width, height int, _ error) {
	size := h.windows.size()
	return size.Width, size.Height, nil
}

// Close stops input before queuing inverse modes behind the final frame.
// The caller retains the SSH channel so it can send its chosen exit status.
// An in-flight channel read may last until that caller closes the channel;
// teardown must not wait for another client byte.
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		h.source.Close()
		h.writer.CloseAdmission()
		h.clip.Interrupt()
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
