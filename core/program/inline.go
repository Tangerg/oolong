package program

import "github.com/Tangerg/oolong/core/grid"

// InlineRuntime is a [Runtime] that can publish completed output into terminal
// scrollback. It is only constructed for [Config.Inline], and its zero value is
// inert.
type InlineRuntime struct{ *Runtime }

// inlineCanvas is the publication surface when this is a live inline runtime.
// Keeping the embedded-runtime and mode checks together makes every publishing
// operation share the same zero-value semantics.
func (r *InlineRuntime) inlineCanvas() *grid.Inline {
	if r == nil {
		return nil
	}
	p := r.owner()
	if p == nil {
		return nil
	}
	return p.inline
}

// Print publishes a measured drawable above an inline interface.
func (r *InlineRuntime) Print(p grid.Drawable) {
	inline := r.inlineCanvas()
	if inline == nil || p == nil {
		return
	}
	inline.Print(p)
}

// Append continues the last published row until draw reports completion.
func (r *InlineRuntime) Append(draw func(grid.View) bool) {
	inline := r.inlineCanvas()
	if inline == nil || draw == nil {
		return
	}
	for {
		before := inlineRoom(inline)
		more := false
		inline.Append(func(v grid.View) { more = draw(v) })
		if !more {
			return
		}
		if inlineRoom(inline) == before && before == 0 {
			// A whole row to itself and nothing drawn into it. No amount of room
			// would help, so asking again is asking forever.
			return
		}
		inline.Break()
	}
}

// inlineRoom is how much of the open row has been taken, or zero when the next
// thing published starts a row of its own.
func inlineRoom(inline *grid.Inline) int {
	col, open := inline.Tail()
	if !open {
		return 0
	}
	return col
}
