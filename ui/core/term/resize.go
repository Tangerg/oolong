package term

import (
	"image"
	"time"
)

// dimensions owns resize deduplication for polling and signal-driven sources.
// Failed observations do not replace the last valid size; the first successful
// observation is new only when startup had no size.
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
