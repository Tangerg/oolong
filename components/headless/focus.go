package headless

// focusState is the outer keyboard state shared by compound component owners.
//
// The zero value is focused and unsettled: a lone widget assumes it has the keyboard,
// while the first owner that contains it still has to tell every child where it
// stands. Once settled, repeating the same report is observationally empty. Changing
// outer focus notifies the existing holder without rebuilding ownership; changing
// children is left to the owner's settle operation.
type focusState struct {
	settled bool
	blurred bool
}

func (s *focusState) change(has bool, settle func(), holder *Widget) {
	blurred := !has
	if s.blurred == blurred {
		settle()
		return
	}
	wasSettled := s.settled
	s.blurred = blurred
	settle()
	if wasSettled {
		tell(*holder, has)
	}
}
