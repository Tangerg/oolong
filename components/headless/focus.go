package headless

import "github.com/Tangerg/oolong/components/internal/identity"

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
		// Settlement can accept a nested Focus call. Both the holder and its
		// focus must come from the current owner, never the superseded request.
		tell(*holder, !s.blurred)
	}
}

// settleOne is settlement for an owner that contains exactly one widget: holder is
// whatever was last told where it stands, and child is what should hold it now.
//
// Owners with one child used to answer this question each in their own way, and the
// part every copy got wrong or right by accident was the first report. A widget with
// nothing above it assumes it has the keyboard, so the first thing an owner says is
// never a repetition — it is the moment the widget stops assuming. Reading that off
// blurred alone makes it look like a no-op, because the answer it would repeat is
// one nobody has given yet.
func (s *focusState) settleOne(holder *Widget, child Widget) {
	if s.settled && identity.Same(*holder, child) {
		return
	}
	previous := *holder
	*holder, s.settled = child, true
	// The one that had it is told first and by name: a widget replaced by another is
	// no longer anybody's to report to, and would go on believing it has the keyboard.
	if previous != nil && !identity.Same(previous, child) {
		tell(previous, false)
	}
	tell(child, !s.blurred)
}
