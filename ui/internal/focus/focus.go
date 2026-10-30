// Package focus tracks reentrant focus notifications received from scene.
// It does not select focus; scene remains the only holder owner.
package focus

import "math"

// State remembers only the last received notification and its callback epoch.
// A nested notification revokes validation work started by an older one.
type State struct {
	received bool
	focused  bool
	turn     uint64
}

// Receive records scene's notification before invoking dependent callbacks.
// An implicit standalone initial focus is not a notification requiring blur validation.
func (s *State) Receive(has bool) (turn uint64, leaving bool) {
	if s.received && s.focused == has {
		return s.turn, false
	}
	if s.turn == math.MaxUint64 {
		panic("focus: notification epochs exhausted")
	}
	leaving = s.received && s.focused && !has
	s.received, s.focused = true, has
	s.turn++
	return s.turn, leaving
}

// Superseded reports whether a later notification revoked the caller's callback epoch.
func (s *State) Superseded(turn uint64) bool { return s.turn != turn }
