// Package focus tracks reentrant focus notifications received from scene.
// It does not select focus; scene remains the only holder owner.
package focus

// State remembers only the last received notification and its callback identity.
// A nested notification revokes validation work started by an older one. Turn
// nil means no notification has been received; fresh identities cannot wrap.
type State struct {
	focused bool
	turn    *byte
}

// Receive records scene's notification before invoking dependent callbacks.
// An implicit standalone initial focus is not a notification requiring blur validation.
func (s *State) Receive(has bool) (turn *byte, leaving bool) {
	received := s.turn != nil
	if received && s.focused == has {
		return s.turn, false
	}
	leaving = received && s.focused && !has
	s.focused = has
	s.turn = new(byte)
	return s.turn, leaving
}

func (s *State) Superseded(turn *byte) bool { return s.turn != turn }
