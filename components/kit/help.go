package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// Help is a row of key hints.
//
// The keys come from the same [headless.Binding] values the handlers match against,
// which is what stops the hints and the behaviour from disagreeing.
type Help struct {
	Bindings  []headless.Binding
	KeyStyle  grid.Style
	DoesStyle grid.Style
	// Separator sits between hints. Empty uses two spaces, which separates without
	// adding another thing to look at.
	Separator      string
	SeparatorStyle grid.Style
}

// Measure is one row.
func (h Help) Measure(int) int { return 1 }

// Draw writes as many hints as fit, in order, dropping the rest.
//
// Dropping rather than truncating the last one: half a hint is not a hint, and the
// bindings are listed in the order they matter so the ones that survive a narrow
// terminal are the ones worth keeping.
func (h Help) Draw(v grid.View) {
	w, height := v.Size()
	if w <= 0 || height <= 0 {
		return
	}
	separator := h.Separator
	if separator == "" {
		separator = "  "
	}
	sepWidth := text.Width(separator)

	x := 0
	for _, b := range h.Bindings {
		if b.Hidden {
			continue
		}
		key := b.Key.String()
		hint := text.Width(key) + 1 + text.Width(b.Does)
		need := hint
		if x > 0 {
			need += sepWidth
		}
		if x+need > w {
			return
		}
		if x > 0 {
			x += v.Text(x, 0, separator, h.SeparatorStyle)
		}
		x += v.Text(x, 0, key, h.KeyStyle)
		x += v.Text(x, 0, " ", h.DoesStyle)
		x += v.Text(x, 0, b.Does, h.DoesStyle)
	}
}
