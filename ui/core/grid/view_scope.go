package grid

// viewScope is the drawing authority borrowed from a canvas. Copies and subviews
// share its lifetime; only the surface can revoke it. A frame borrow also observes
// publication, which temporarily excludes drawing while Print or Append runs.
type viewScope struct {
	publication *publication
	open        bool
}

func (s *viewScope) requireWritable() {
	if s == nil {
		return
	}
	if !s.open {
		panic("grid: drawing through an expired canvas view")
	}
	if s.publication != nil {
		s.publication.requireIdle()
	}
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
