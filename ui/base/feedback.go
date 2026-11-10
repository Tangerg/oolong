package base

import "github.com/Tangerg/oolong/ui/core/scene"

// A callback may start a newer UI request before returning. Its feedback must
// not replace the newer request's outcome when that older callback finishes.
type requestFeedback struct {
	turn uint64
	err  error
}

func (f *requestFeedback) begin() uint64 {
	f.turn++
	return f.turn
}

func (f *requestFeedback) finish(turn uint64, err error) bool {
	if turn != f.turn {
		return false
	}
	changed := f.err != nil || err != nil
	f.err = err
	return changed
}

// withFeedback projects a refused physical request into the control's semantics.
// Keyboard and pointer requests have no caller to return an error to, so the
// control is the only place a user or an assistive client can learn of it.
func withFeedback(semantic scene.Semantic, err error) scene.Semantic {
	if err == nil {
		return semantic
	}
	semantic.State |= scene.StateInvalid
	if semantic.Description == "" {
		semantic.Description = err.Error()
	}
	return semantic
}

func (f *requestFeedback) clear() {
	f.begin()
	f.err = nil
}
