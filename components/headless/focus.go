package headless

import (
	"github.com/Tangerg/oolong/components/internal/focus"
	"github.com/Tangerg/oolong/core/keymap"
)

type focusState struct{ state focus.State }

func (s *focusState) begin() uint64               { return s.state.Begin() }
func (s *focusState) superseded(turn uint64) bool { return s.state.Superseded(turn) }
func (s *focusState) change(has bool, settle func(), holder *Widget, matcher *keymap.Matcher) {
	change := func() { s.state.Change(has, settle, func(has bool) { tell(*holder, has) }) }
	if has {
		change()
	} else {
		matcher.Reset(change)
	}
}

func (s *focusState) settleOne(holder *Widget, child Widget) {
	focus.SettleOne(&s.state, holder, child, tell)
}
