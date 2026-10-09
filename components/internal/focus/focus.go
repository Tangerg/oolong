// Package focus owns reentrant child focus transfers shared by headless and kit.
package focus

import "github.com/Tangerg/oolong/components/internal/identity"

// State owns outer focus and the progress of synchronous child notifications.
// Publish the new holder and settlement before calling a child. A later attachment
// transfer revokes the interrupted transfer; outer focus is read at notification time.
type State struct {
	settled bool
	blurred bool
	turn    uint64
}

// Settled reports whether the current attachment has published its focus holder.
func (s *State) Settled() bool { return s.settled }

// Focused reports the outer focus to use at child notification time.
func (s *State) Focused() bool { return !s.blurred }

// Unsettle requires the next settlement to account for changed attachments.
func (s *State) Unsettle() { s.settled = false }

// Begin publishes settlement before callbacks and revokes earlier transfers.
func (s *State) Begin() uint64 {
	s.settled = true
	s.turn++
	return s.turn
}

// Superseded reports whether a later focus or attachment transfer revoked turn.
func (s *State) Superseded(turn uint64) bool { return s.turn != turn }

// Receive records a direct focus notification before dependent callbacks. It
// reports whether previously received focus is being released; the implicit focus
// of a lone widget is not a notification that can require blur validation.
func (s *State) Receive(has bool) (turn uint64, leaving bool) {
	if s.settled && s.blurred == !has {
		return s.turn, false
	}
	leaving = s.settled && !s.blurred && !has
	s.blurred = !has
	return s.Begin(), leaving
}

// Forward consumes a child's declined operation if its routing scope changed during
// the callback. A stale decline cannot authorize fallback against a new attachment
// or outer focus, including a transition away and back to the original state.
func (s *State) Forward(handle func() bool) bool {
	turn := s.turn
	handled := handle()
	return handled || s.Superseded(turn)
}

// Change settles attachments before notifying their holder of current outer focus.
func (s *State) Change(has bool, settle func(), notify func(bool)) {
	if s.blurred == !has {
		settle()
		return
	}
	wasSettled := s.settled
	s.turn++
	s.blurred = !has
	settle()
	if wasSettled {
		notify(s.Focused())
	}
}

// SettleOne publishes child before notifications and stops if a callback replaces it.
func SettleOne[T any](s *State, holder *T, child T, notify func(T, bool)) {
	if s.settled && identity.Same(*holder, child) {
		return
	}
	previous := *holder
	*holder = child
	turn := s.Begin()
	if !identity.Same(previous, child) {
		notify(previous, false)
		if s.Superseded(turn) {
			return
		}
	}
	notify(child, s.Focused())
}
