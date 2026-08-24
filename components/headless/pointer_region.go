package headless

import (
	"image"

	"github.com/Tangerg/oolong/components/internal/identity"
	"github.com/Tangerg/oolong/core/input"
)

// PointerRegion is where one interactive child was drawn, and who owns a gesture that
// began there.
//
// # Why this is a type rather than a rule everyone keeps
//
// Anything that draws a child inside itself creates the same boundary: it has to move
// pointer coordinates into the child's box, and it has to decide what happens when the
// gesture leaves. Both answers are easy to get subtly wrong, and getting them wrong
// looks like a bug in the child. A wrapper that re-tests its own bounds on every event
// drops the drag at its frame and never delivers the release, so the child goes on
// believing it is being dragged. A wrapper that remembers the geometry instead of the
// child keeps translating by a rectangle the child has since moved out of.
//
// So the rule lives here once, in the ring that owns behaviour, and an appearance
// layer composes it rather than reimplementing it. [Container] and [Stack] keep the
// same rule for the several children and the several layers they own; this is the same
// rule for exactly one.
//
// # The rule
//
// A press over the region gives its child the gesture. The drag and release that
// follow belong to that child wherever the pointer then goes — and are translated by
// where **this** frame drew it, not where it was when the press landed, because the
// user is aiming at what is on the screen now. A child that is no longer presented has
// nowhere to send its gesture, so the remainder is dropped rather than handed to
// whatever took its place.
//
// The zero value has no child and declines everything. It must be staged during
// [Root.Draw] and read only from Handle. A PointerRegion must not be copied after
// first use: its committed child and captured gesture are one routing owner.
type PointerRegion struct {
	noCopy noCopy

	presented Snapshot[pointerRegionFrame]
	// held is the child that accepted a press. Identity is checked without comparing
	// interface values directly, so an external value implementation can never turn
	// routing into a comparability panic.
	held      Interactive
	heldFrame frameStamp
}

// pointerRegionFrame is one child and where it was drawn, published together so that
// neither can be read against the other's frame.
type pointerRegionFrame struct {
	area  image.Rectangle
	child Interactive
	frame frameStamp
}

// Stage publishes where child was drawn, to take effect with the complete root frame.
//
// A child that does not answer input is staged as an absence: the region is still not
// somewhere a press can land, and saying so here keeps the caller from having to.
func (r *PointerRegion) Stage(frame Frame, area image.Rectangle, child Widget) {
	if r == nil {
		return
	}
	target, _ := child.(Interactive)
	if target == nil {
		area = image.Rectangle{}
	}
	r.presented.Stage(frame, pointerRegionFrame{area: area, child: target, frame: frame.stamp()})
}

// Handle offers a pointer event to the child, in the child's own coordinates.
//
// Handled reports whether the child consumed it. Delivered reports whether it reached
// the child at all, which is a different question and the one a wrapper with a second
// pointer target asks: a strip of tabs above a pane needs to know that an event was
// outside the pane, not merely that the pane declined it.
func (r *PointerRegion) Handle(event input.Mouse) (handled, delivered bool) {
	if r == nil {
		return false, false
	}
	presented := r.presented.Value()

	// The owner is read before ownership can end, because the release that ends it is
	// also the last event it is owed.
	if owner := r.held; owner != nil &&
		(event.Action == input.MouseDrag || event.Action == input.MouseUp) {
		ownerFrame := r.heldFrame
		// The gesture is over once its release has been offered, whether or not the
		// child wanted it: consuming and owning are separate questions.
		if event.Action == input.MouseUp {
			r.held = nil
			r.heldFrame = frameStamp{}
		}
		if (presented.frame != ownerFrame && !identity.Same(presented.child, owner)) || presented.area.Empty() {
			return false, false
		}
		return r.deliver(presented, event), true
	}

	if presented.child == nil || presented.area.Empty() || !event.Pos.In(presented.area) {
		return false, false
	}
	// A press supersedes a gesture that never ended. Forgetting the old owner before
	// the child is called is what keeps a panic or a refusal from leaving it installed.
	if event.Action == input.MouseDown {
		r.held = nil
		r.heldFrame = frameStamp{}
	}
	handled = r.deliver(presented, event)
	if event.Action == input.MouseDown && handled {
		// Only a press the child wanted begins an interaction. Holding one it refused
		// would swallow the release belonging to whatever the pointer is really over.
		r.held = presented.child
		r.heldFrame = presented.frame
	}
	return handled, true
}

func (r *PointerRegion) deliver(presented pointerRegionFrame, event input.Mouse) bool {
	local := event
	local.Pos = event.Pos.Sub(presented.area.Min)
	return presented.child.Handle(local)
}
