package program

import "github.com/Tangerg/oolong/core/grid"

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
