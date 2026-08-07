//go:build windows

package term

import "time"

// Windows exposes console resize records through the same input handle as keys.
// Reading those records separately would give the terminal two competing readers,
// while the VT byte reader deliberately has sole ownership of input. Sampling the
// console geometry here preserves that ownership and presents the same Resize
// stream as SIGWINCH platforms.
const resizePollInterval = 100 * time.Millisecond

func (t *Terminal) startResizeWatcher(last dimensions) {
	go func() {
		defer close(t.resizeDone)
		ticker := time.NewTicker(resizePollInterval)
		defer ticker.Stop()
		pollResize(t.stop, ticker.C, last, t.Size, t.reportResize)
	}()
}
