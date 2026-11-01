package term

import (
	"image"
	"time"
)

// dimensions is the newest successful platform observation offered to the input
// stream.
//
// A failed observation is not a new size, and the first successful observation
// is a change only when Open could not report an opening size. Keeping that rule
// here makes a polling platform obey the same event contract as a signalling one
// without teaching the pump how either platform discovered the change.
type dimensions struct {
	point image.Point
	known bool
}

func knownDimensions(width, height int) dimensions {
	return dimensions{point: image.Pt(width, height), known: true}
}

func (d *dimensions) observe(width, height int) bool {
	next := image.Pt(width, height)
	if d.known && d.point == next {
		return false
	}
	d.point, d.known = next, true
	return true
}

// Polling and signal-based platforms offer the same measurement to the
// terminal, which serializes it against handover.
func pollResize(stop <-chan struct{}, ticks <-chan time.Time, observe func()) {
	for {
		select {
		case _, ok := <-ticks:
			if !ok {
				return
			}
			observe()
		case <-stop:
			return
		}
	}
}
