//go:build windows

package term

import "time"

// Windows exposes console resize records through the same input handle as keys.
// Reading those records separately would give the terminal two competing readers,
// while the VT byte reader deliberately has sole ownership of input. Sampling the
// console geometry here preserves that ownership and presents the same Resize
// stream as SIGWINCH platforms.
const resizePollInterval = 100 * time.Millisecond

// resizeSource is the clock this platform watches its console with. It is started
// before the first measurement for the same reason a signal subscription is: what
// happens between measuring and starting to watch has to be something the watcher
// can still find out about.
type resizeSource struct{ ticker *time.Ticker }

func subscribeResize() resizeSource {
	return resizeSource{ticker: time.NewTicker(resizePollInterval)}
}

func (t *Terminal) startResizeWatcher(source resizeSource) {
	go func() {
		defer close(t.resizeDone)
		defer source.ticker.Stop()
		pollResize(t.stop, source.ticker.C, t.Size, t.noteResize)
	}()
}
