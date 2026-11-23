package term

import (
	"image"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"

	xterm "golang.org/x/term"
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

// Size is the terminal's size in cells.
func (t *Terminal) Size() (w, h int, err error) {
	return xterm.GetSize(t.outFD)
}

// Measure under the same lock as publication: a delayed watcher must not
// overwrite a newer handover measurement. Handover forces a repaint even
// when the child returned the same size, because the display may have changed.
func (t *Terminal) noteResize(measure func() (int, int, error), force bool) {
	t.resizeMu.Lock()
	defer t.resizeMu.Unlock()
	width, height, err := measure()
	if err == nil && (t.size.observe(width, height) || force) {
		t.publishResize(width, height)
	}
}

// publishResize offers the newest measured size to the input pump. Dimensions are
// replaceable state: when the pump has not consumed an older observation, replace it
// instead of dropping the newer truth or blocking the platform watcher.
//
// The caller holds resizeMu, which is also what serializes the producers below.
func (t *Terminal) publishResize(width, height int) {
	latest := input.Resize{Width: width, Height: height}
	select {
	case t.resized <- latest:
		return
	default:
	}
	select {
	case <-t.resized:
	default:
	}
	// Producers are serialized by resizeMu. After removing their older value the
	// mailbox has room, even if the pump raced and consumed that value first.
	t.resized <- latest
}
