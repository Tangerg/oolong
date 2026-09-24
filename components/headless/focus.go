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
	// turn numbers the transfers of the keyboard this owner has made.
	//
	// Telling a widget where it stands is a call into somebody else's code, and that
	// code may answer by moving the keyboard again before the first move has finished
	// saying so. The rest of the interrupted transfer would then speak from premises
	// that no longer hold — telling the newer holder it does not have the keyboard,
	// and the one it replaced that it does — and its last words would be the ones the
	// widgets kept. A transfer that finds a newer one has begun has nothing left to
	// say: the newer one told everybody.
	turn uint64
}

// begin claims the next transfer of the keyboard.
func (s *focusState) begin() uint64 {
	s.turn++
	return s.turn
}

// superseded reports whether a newer transfer has replaced the one numbered turn.
func (s *focusState) superseded(turn uint64) bool { return s.turn != turn }

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
	turn := s.begin()
	// The one that had it is told first and by name: a widget replaced by another is
	// no longer anybody's to report to, and would go on believing it has the keyboard.
	if previous != nil && !identity.Same(previous, child) {
		tell(previous, false)
		if s.superseded(turn) {
			return
		}
	}
	tell(child, !s.blurred)
}
