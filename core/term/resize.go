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

// noteResize records what a platform watcher measured and publishes it when it is
// news.
//
// The last size has one owner, and this and [Terminal.retakeResize] are the two
// paths that may advance it. A watcher keeping its own copy was the older shape and
// the reason a handover could leave the session the wrong size for good: while a
// child holds the terminal, its process group gets the resize signals and this one
// does not, so the watcher comes back remembering a size that is no longer true. If
// the measurement taken on the way back does not also become the watcher's, a later
// change back to that remembered size reads as no change at all.
func (t *Terminal) noteResize(width, height int, err error) {
	t.resizeMu.Lock()
	defer t.resizeMu.Unlock()
	if t.size.observe(width, height, err) {
		t.publishResize(t.size.point.X, t.size.point.Y)
	}
}

// retakeResize records the size measured on taking the terminal back and publishes
// it whether or not it changed. The program has to rebuild a screen whose contents
// the child replaced, and that is true at any size.
func (t *Terminal) retakeResize(width, height int) {
	t.resizeMu.Lock()
	defer t.resizeMu.Unlock()
	t.size.observe(width, height, nil)
	t.publishResize(width, height)
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
