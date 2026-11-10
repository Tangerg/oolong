//go:build !unix && !windows

package term

// resizeSource has no platform mechanism outside the supported Unix and Windows
// families, so there is nothing to subscribe to before the first measurement.
type resizeSource struct{}

func subscribeResize() resizeSource { return resizeSource{} }

// startResizeWatcher still participates in the terminal lifetime so Close has one
// shutdown protocol on every source set.
func (t *Terminal) startResizeWatcher(resizeSource) {
	go func() {
		defer close(t.resizeDone)
		<-t.stop
	}()
}
