//go:build !unix && !windows

package term

// startResizeWatcher has no platform source outside the supported Unix and Windows
// families. It still participates in the terminal lifetime so Close has one
// shutdown protocol on every source set.
func (t *Terminal) startResizeWatcher(dimensions) {
	go func() {
		defer close(t.resizeDone)
		<-t.stop
	}()
}
