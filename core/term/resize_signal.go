//go:build unix

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// resizeSource is this platform's announcement that the terminal changed size,
// already subscribed to.
//
// Subscribing is separate from watching because of what sits between them: the
// first measurement. Until the subscription exists the signal goes to the default
// handler, which ignores it, so a change between measuring and subscribing is one
// nothing would ever report — and the session would draw at the wrong size until
// some later, unrelated change happened to correct it.
type resizeSource struct{ changed chan os.Signal }

func subscribeResize() resizeSource {
	changed := make(chan os.Signal, 1)
	signal.Notify(changed, syscall.SIGWINCH)
	return resizeSource{changed: changed}
}

// startResizeWatcher hands the subscription to the goroutine that owns it until
// Close waits for it.
func (t *Terminal) startResizeWatcher(source resizeSource) {
	go func() {
		defer close(t.resizeDone)
		defer signal.Stop(source.changed)
		for {
			select {
			case <-source.changed:
				t.noteResize(t.Size())
			case <-t.stop:
				return
			}
		}
	}()
}
