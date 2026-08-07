package kit

import (
	"image"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
)

// pointerRegion is one interactive child as it appeared in the last complete frame.
//
// Appearance wrappers all create the same boundary: they draw chrome around a child
// and therefore have to move pointer coordinates through that chrome. The boundary
// also owns the hand-off of a gesture. Keeping those two rules in one object prevents
// a wrapper from accepting a press and then losing its drag or release when the
// pointer leaves the content, the layout collapses, or a different child is installed
// before the next frame.
type pointerRegion struct {
	presentation headless.Snapshot[pointerPresentation]
	captured     pointerPresentation
}

type pointerPresentation struct {
	area   image.Rectangle
	target pointerHandler
}

// pointerHandler is deliberately narrower than headless.Interactive. A composer
// routes through Editor.HandleMouse because the editor also needs its drawn width;
// ordinary widgets route through Handle.
type pointerHandler interface {
	handlePointer(event input.Mouse) bool
}

type interactivePointer struct{ headless.Interactive }

func (p interactivePointer) handlePointer(event input.Mouse) bool {
	return p.Handle(event)
}

type editorPointer struct {
	editor *headless.Editor
	width  int
}

func (p editorPointer) handlePointer(event input.Mouse) bool {
	return p.editor != nil && p.editor.HandleMouse(event, p.width)
}

// stage prepares a target and its area for publication with the complete root frame.
func (r *pointerRegion) stage(
	frame headless.Frame,
	area image.Rectangle,
	target pointerHandler,
) {
	r.presentation.Stage(frame, pointerPresentation{area: area, target: target})
}

// stageWidget is stage for the ordinary case: a child that answers input through
// headless.Interactive. A passive child still publishes its area, but has no target.
func (r *pointerRegion) stageWidget(
	frame headless.Frame,
	area image.Rectangle,
	child headless.Widget,
) {
	target, _ := child.(headless.Interactive)
	if target == nil {
		r.stage(frame, area, nil)
		return
	}
	r.stage(frame, area, interactivePointer{target})
}

// clear publishes the absence of a child region.
func (r *pointerRegion) clear(frame headless.Frame) {
	r.stage(frame, image.Rectangle{}, nil)
}

// handle translates and offers a pointer event. Delivered distinguishes an event
// that reached the child from one outside this region; the distinction matters to a
// wrapper with another pointer target, such as a tab strip above its pane.
func (r *pointerRegion) handle(event input.Mouse) (handled, delivered bool) {
	presented := r.presentation.Value()
	captured := r.captured.target != nil &&
		(event.Action == input.MouseDrag || event.Action == input.MouseUp)
	if captured {
		// Capture keeps the whole accepted presentation, not the old target paired
		// with a replacement target's geometry. That mixed frame is precisely what
		// transactional routing exists to prevent.
		presented = r.captured
	} else if presented.target == nil || presented.area.Empty() || !event.Pos.In(presented.area) {
		return false, false
	}

	// A new press supersedes an incomplete old gesture. Clear before calling the
	// child so a panic or a declined press cannot leave the previous owner installed.
	if event.Action == input.MouseDown {
		r.captured = pointerPresentation{}
	}
	// Likewise, ownership ends when a release is delivered even when the child
	// declines it. Consuming and ownership are separate questions.
	if captured && event.Action == input.MouseUp {
		r.captured = pointerPresentation{}
	}

	local := event
	local.Pos = event.Pos.Sub(presented.area.Min)
	handled = presented.target.handlePointer(local)
	if event.Action == input.MouseDown && handled {
		r.captured = presented
	}
	return handled, true
}
