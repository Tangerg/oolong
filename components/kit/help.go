package kit

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"
)

// Help is a row of key hints.
//
// The keystrokes are read out of the same map the handlers read, so a hint cannot
// disagree with what the key does: there is one place a key is bound, and this asks it.
// An action with nothing bound to it is not shown, because a hint nobody can press is
// worse than no hint.
type Help struct {
	// Theme is the look. Each part of a hint has a fixed role in one — the key is
	// the thing to press, what it does is a note about it — so there is nothing here
	// to choose between.
	Theme Theme
	// Keys is where the keystrokes come from.
	Keys *keymap.Map
	// Show are the actions to show, in the order they matter. Hiding one is leaving it
	// out: an action that works and that nobody needs told about is simply not listed.
	//
	// Each identifier is shown as words by this appearance layer; the key map carries
	// only identity and binding policy.
	Show []keymap.Action
	// Separator sits between hints. Empty uses two spaces, which separates without
	// adding another thing to look at.
	Separator string
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
	for _, action := range h.Show {
		bound := h.Keys.Keys(action)
		if len(bound) == 0 {
			continue
		}
		// The first sequence bound to it, which is the one a caller put first.
		key := bound[0].String()
		does := actionLabel(action)
		need := text.Width(key) + 1 + text.Width(does)
		if x > 0 {
			need += sepWidth
		}
		if x+need > w {
			return
		}
		if x > 0 {
			x += v.Text(x, 0, separator, h.Theme.Subtle)
		}
		x += v.Text(x, 0, key, h.Theme.Accent)
		x += v.Text(x, 0, " ", h.Theme.Muted)
		x += v.Text(x, 0, does, h.Theme.Muted)
	}
}

// actionLabel is presentation policy: action identifiers use hyphens, while this
// appearance renders them as words. It deliberately does not live in keymap.
func actionLabel(action keymap.Action) string {
	return strings.ReplaceAll(action.String(), "-", " ")
}
