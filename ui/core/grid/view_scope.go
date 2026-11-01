package grid

// viewScope is the drawing authority borrowed from a canvas. Copies and subviews
// share its lifetime; only the surface can revoke it. A frame borrow also observes
// publication, which excludes drawing while its output transaction is unsettled.
type viewScope struct {
	parent      *viewScope
	guard       func() error
	publication *publication
	open        bool
}

func (s *viewScope) requireWritable() {
	for ; s != nil; s = s.parent {
		if !s.open {
			panic("grid: drawing through an expired canvas view")
		}
		if s.guard != nil {
			if err := s.guard(); err != nil {
				panic(err)
			}
		}
		if s.publication != nil {
			s.publication.requireIdle()
		}
	}
}

// RestrictWrites returns a view that also checks guard before every write.
// Returning an error makes the write panic before changing cells, cursor or
// native regions. The guard must be pure. Subviews retain every parent guard.
func (v View) RestrictWrites(guard func() error) View {
	if guard != nil {
		v.scope = &viewScope{parent: v.scope, guard: guard, open: true}
	}
	return v
}

func (s *Surface) borrow(owner *publication) View {
	s.endDrawing()
	s.drawing = &viewScope{publication: owner, open: true}
	view := s.View()
	view.scope = s.drawing
	return view
}

func (s *Surface) endDrawing() {
	if s.drawing != nil {
		s.drawing.open = false
		s.drawing.publication = nil
		s.drawing = nil
	}
}
