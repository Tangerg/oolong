package term

import (
	"image"
	"time"

	"github.com/Tangerg/oolong/core/input"
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

func (d *dimensions) observe(width, height int, err error) bool {
	if err != nil {
		return false
	}
	next := image.Pt(width, height)
	if d.known && d.point == next {
		return false
	}
	d.point, d.known = next, true
	return true
}

// pollResize observes a platform whose console has no independent resize signal.
// The clock, source, and sink are arguments because the loop owns policy rather than
// either mechanism: Windows supplies a ticker and Terminal.Size, while tests can
// prove ordering, deduplication, error recovery, and shutdown without sleeping.
func pollResize(
	stop <-chan struct{},
	ticks <-chan time.Time,
	last dimensions,
	size func() (int, int, error),
	report func(width, height int),
) {
	for {
		select {
		case _, ok := <-ticks:
			if !ok {
				return
			}
			if last.observe(size()) {
				report(last.point.X, last.point.Y)
			}
		case <-stop:
			return
		}
	}
}

// reportResize offers the newest measured size to the input pump. Dimensions are
// replaceable state: when the pump has not consumed an older observation, replace it
// instead of dropping the newer truth or blocking the platform watcher.
func (t *Terminal) reportResize(width, height int) {
	t.resizeMu.Lock()
	defer t.resizeMu.Unlock()

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
