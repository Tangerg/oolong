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
// The clock, source and sink are arguments because the loop owns policy rather than
// either mechanism: Windows supplies a ticker and Terminal.Size, while tests can
// prove ordering, error recovery and shutdown without sleeping. Whether an
// observation is news belongs to the terminal that keeps the last one — see
// [Terminal.noteResize].
func pollResize(
	stop <-chan struct{},
	ticks <-chan time.Time,
	size func() (int, int, error),
	note func(width, height int, err error),
) {
	for {
		select {
		case _, ok := <-ticks:
			if !ok {
				return
			}
			note(size())
		case <-stop:
			return
		}
	}
}
