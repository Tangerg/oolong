//go:build windows

package term

import "time"

// Console resize records share the keyboard handle. Poll geometry instead of
// adding a competing record reader, preserving sole ownership of input.
const resizePollInterval = 100 * time.Millisecond

// Start sampling before the initial measurement so changes during startup
// remain observable by the watcher.
type resizeSource struct{ ticker *time.Ticker }

func subscribeResize() resizeSource {
	return resizeSource{ticker: time.NewTicker(resizePollInterval)}
}

func (t *Terminal) startResizeWatcher(source resizeSource) {
	go func() {
		defer close(t.resizeDone)
		defer source.ticker.Stop()
		pollResize(t.stop, source.ticker.C, func() { t.noteResize(t.Size, false) })
	}()
}
