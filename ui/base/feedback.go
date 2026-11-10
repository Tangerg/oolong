package base

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

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

// An invalid flag alone cannot tell a user or assistive client how to recover.
// Error details are read from their owner, never retained as a second UI value.
func withFeedback(semantic scene.Semantic, err error) scene.Semantic {
	if err == nil {
		return semantic
	}
	semantic.State |= scene.StateInvalid
	if semantic.Description == "" {
		semantic.Description = err.Error()
	} else {
		semantic.Description += "\n" + err.Error()
	}
	return semantic
}

func feedbackRows(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

// Feedback has its own row so text and list hit testing cannot treat an error
// message as editable content. Semantics retain the full message when it clips.
func feedbackFrame(frame scene.Frame, err error, look Look) scene.Frame {
	width, height := frame.Size()
	rows := feedbackRows(err)
	if rows == 0 || height <= 0 {
		return frame
	}
	contentHeight := max(height-rows, 0)
	message := strings.ReplaceAll(err.Error(), "\n", "; ")
	text.Of(message, look.Danger).Truncate(width, look.Ellipsis).Draw(frame.View, 0, contentHeight)
	return frame.Sub(grid.Area(0, 0, width, contentHeight))
}

func (f *requestFeedback) clear() {
	f.begin()
	f.err = nil
}
