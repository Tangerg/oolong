package ssh

import (
	"fmt"
	"sync"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

// The SSH request loop can block on its very first resize before a shell exists.
// Drain until channel teardown, even after an invalid update or a host's Close.
type windowFeed struct {
	mu      sync.RWMutex
	current input.Resize
	err     error

	// Closure publishes the first invalid size after any accepted update. The
	// normal end of window notifications does not terminate byte input.
	events chan input.Resize
	end    chan struct{}
	done   chan struct{}
}

func newWindowFeed(initial charmssh.Window, windows <-chan charmssh.Window) *windowFeed {
	f := &windowFeed{events: make(chan input.Resize, 1), end: make(chan struct{}), done: make(chan struct{})}
	f.apply(initial)
	go f.watch(windows)
	return f
}

func (f *windowFeed) watch(windows <-chan charmssh.Window) {
	defer close(f.done)
	for {
		select {
		case window, open := <-windows:
			if !open {
				return
			}
			f.apply(window)
		case <-f.end:
			// The request loop has ended. Settle any resize it enqueued before cleanup.
			for range len(windows) {
				f.apply(<-windows)
			}
			return
		}
	}
}

func (f *windowFeed) stop() {
	close(f.end)
	<-f.done
}

func (f *windowFeed) size() input.Resize {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.current
}

func (f *windowFeed) result() error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.err
}

func (f *windowFeed) apply(window charmssh.Window) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return
	}
	next := f.current
	if window.Width != 0 {
		next.Width = window.Width
	}
	if window.Height != 0 {
		next.Height = window.Height
	}
	if err := grid.ValidateTerminalSize(next.Width, next.Height); err != nil {
		f.err = fmt.Errorf("%w: %w", ErrWindowSize, err)
		close(f.events)
		return
	}
	if next == f.current {
		return
	}
	initial := f.current == (input.Resize{})
	f.current = next
	if initial {
		return
	}
	// The sender alone replaces a pending update, so this final send has room
	// even if the decoder takes the old value between the receive and send.
	select {
	case <-f.events:
	default:
	}
	f.events <- next
}
