//go:build unix

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// Subscribe before measuring: a resize between measurement and subscription
// would otherwise disappear until a later, unrelated signal.
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
				t.noteResize(t.Size, false)
			case <-t.stop:
				return
			}
		}
	}()
}
