//go:build unix

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// startResizeWatcher installs the process signal subscription before Open returns,
// so a size change immediately after opening cannot land in a subscription race.
// The goroutine owns the subscription from there until Close waits for it.
func (t *Terminal) startResizeWatcher(last dimensions) {
	changed := make(chan os.Signal, 1)
	signal.Notify(changed, syscall.SIGWINCH)
	go func() {
		defer close(t.resizeDone)
		defer signal.Stop(changed)
		for {
			select {
			case <-changed:
				if last.observe(t.Size()) {
					t.reportResize(last.point.X, last.point.Y)
				}
			case <-t.stop:
				return
			}
		}
	}()
}
