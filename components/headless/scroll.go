package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Scroll shows a window onto something taller than the space available.
//
// It holds two things: how many rows are hidden above the window, and whether the
// window is following the end of the content.
//
// Both are needed, and neither on its own will do. Holding only the offset means a
// live log stops showing what arrives. Holding only a distance from the end means a
// reader who scrolled up gets dragged forward every time something is appended:
// twenty rows from the end becomes thirty rows from the end, and the text under
// their eyes moves even though they did not ask it to.
//
// The zero value shows the start and does not follow, which is what a list of items
// wants. A transcript asks to follow, once, with [Scroll.ToBottom].
type Scroll struct {
	// offset is how many rows are hidden above the window.
	offset int
	// following makes the window stick to the end as content arrives.
	following bool
	// total and window are what the last layout measured, remembered so a scroll
	// arriving between frames is clamped against something real.
	total, window int
	// wheel turns the terminal's reports into rows, keeping the part of a row a
	// report was worth but did not fill.
	wheel input.Advance
	// matcher owns how far into a multi-chord binding the keys have got.
	matcher keymap.Matcher

	// pendingLayout is derived during a component frame and becomes the scroll's
	// current bounds only with the complete root frame.
	pendingLayout scrollState
	staged        *transaction
}

// Layout tells the scroll how much content there is and how much of it is shown.
//
// It runs once per frame, before anything is drawn. A following window moves to the
// new end; a window that is not following keeps its place, clamped against content
// that may have grown or shrunk since the last frame.
func (s *Scroll) Layout(total, window int) {
	state := s.state()
	state.layout(total, window)
	s.apply(state)
}

// Wheel says what the terminal's wheel reports are worth, which is not a constant:
// terminals disagree about how many of them one notch is. Pass what
// [input.WheelFor] answered, once.
//
// Left alone, the common arrangement is assumed — which is right on most terminals and
// wrong by at most a factor of three on the rest. That is still better than the fixed
// number of rows per report this used to scroll, which was wrong by that factor on
// half of them.
func (s *Scroll) Wheel(w input.Wheel) { s.wheel.Wheel(w) }

// Offset is how many rows are hidden above the window, which is what a scrollbar and
// a hit test both want.
func (s *Scroll) Offset() int { return s.offset }

// AtBottom reports whether the window is following the end of the content.
func (s *Scroll) AtBottom() bool { return s.following }

// By scrolls a number of rows: negative towards the start, positive towards the end.
//
// Reaching the end starts following again, which is what every log viewer does and
// what a reader means by scrolling to the bottom.
func (s *Scroll) By(rows int) {
	s.offset = scrollOffset(s.offset, rows, s.max())
	s.following = s.offset >= s.max()
}

// ToBottom follows the end of the content.
func (s *Scroll) ToBottom() {
	s.following = true
	s.offset = s.max()
}

// ToTop shows the start of the content and stops following.
func (s *Scroll) ToTop() {
	s.following = false
	s.offset = 0
}

// Discard removes rows from the start of the content while preserving the row under
// the reader when it still exists.
//
// A streaming transcript uses this after publishing a prefix. A following window
// stays at the new end; a reader above the new start lands on the first retained row.
func (s *Scroll) Discard(rows int) {
	rows = max(rows, 0)
	s.total = layout.Remaining(s.total, rows)
	s.offset = layout.Remaining(s.offset, rows)
	if s.following {
		s.offset = s.max()
		return
	}
	s.clamp()
}

// Reveal scrolls as little as it can to bring a row into the window.
//
// As little as it can, rather than centring it: a reader stepping through search
// results wants the surrounding text to stay put where it already fits, and a view
// that jumped every time would lose the context that made the result worth finding.
// A row already visible moves nothing at all.
//
// It stops following the end, because a row was asked for and following would
// immediately scroll away from it.
func (s *Scroll) Reveal(row int) {
	s.RevealRange(row, row)
}

// RevealRange brings as much of [first, last] into the window as will fit.
//
// When the range is taller than the window its start wins, because that is where
// reading begins. Anything else would show the end of a match and leave the reader
// to scroll backwards to find out what it was part of.
func (s *Scroll) RevealRange(first, last int) {
	if s.window <= 0 {
		return
	}
	if last < first {
		first, last = last, first
	}
	s.following = false
	switch {
	case first < s.offset:
		s.offset = first
	case last >= layout.Sum(s.offset, s.window):
		s.offset = min(layout.Remaining(last, s.window-1), first)
	}
	s.clamp()
}

// Pages scrolls whole windows, keeping one row of overlap so the reader has
// something to recognise on the other side of the jump.
func (s *Scroll) Pages(n int) {
	s.By(scrollPages(n, max(s.window-1, 1)))
}

// max is the largest offset that still shows a full window.
func (s *Scroll) max() int { return layout.Remaining(s.total, s.window) }

func (s *Scroll) clamp() { s.offset = min(max(s.offset, 0), s.max()) }

// Handle scrolls in response to keys and the mouse wheel, reporting whether it
// consumed the event.
//
// The map is a parameter rather than a field because a scroll is a part rather than a
// widget: it is what a transcript, a list and a viewport each keep inside themselves,
// and each of them already has a map of its own to read through. Nil reads through
// [DefaultScrollKeys].
func (s *Scroll) Handle(ev input.Event, keys *keymap.Map) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		switch mouse.Action {
		case input.WheelUp:
			s.By(s.wheel.At(mouse.At, -1))
			return true
		case input.WheelDown:
			s.By(s.wheel.At(mouse.At, 1))
			return true
		default:
			return false
		}
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	if keys == nil {
		keys = scrollKeys()
	}
	_, handled := s.matcher.Handle(keys, key, s.Do)
	return handled
}

// Do runs one of the scroll's actions by name, reporting whether it was one a scroll
// knows. See [Doer].
func (s *Scroll) Do(action keymap.Action) bool {
	switch action {
	case ScrollUp:
		s.By(-1)
	case ScrollDown:
		s.By(1)
	case ScrollPageUp:
		s.Pages(-1)
	case ScrollPageDown:
		s.Pages(1)
	case ScrollTop:
		s.ToTop()
	case ScrollBottom:
		s.ToBottom()
	default:
		return false
	}
	return true
}

// Rows draws the visible slice of a set of rows.
//
// Each row is drawn by the caller's function, which is given a view one row tall.
// The rows are drawn rather than returned so a row can be as complicated as it
// likes without this type having to know.
func (s *Scroll) Rows(v grid.View, total int, row func(v grid.View, index int)) {
	width, height := v.Size()
	s.Layout(total, height)
	first := s.Offset()
	for y := range height {
		index := layout.Sum(first, y)
		if index >= total {
			return
		}
		row(v.Sub(grid.Rect(0, y, width, 1)), index)
	}
}

// Stage derives the scroll layout for a component frame.
//
// The returned layout is the one drawing should use. Its bounds and offset become
// current only when the complete [Root] frame commits, so input during a nested draw
// continues to see the previous frame. Reveal and RevealRange update this staged
// layout rather than the committed scroll.
func (s *Scroll) Stage(frame Frame, total, window int) ScrollLayout {
	state := s.state()
	if s.staged == frame.transaction {
		state = s.pendingLayout
	}
	state.layout(total, window)
	s.stageState(frame, state)
	return ScrollLayout{scroll: s, frame: frame, state: state}
}

// ScrollLayout is the derived position of one Scroll in a component frame.
//
// It is a short-lived value returned by [Scroll.Stage]. Offset is used to paint the
// frame; adjustments are staged with the same root transaction.
type ScrollLayout struct {
	scroll *Scroll
	frame  Frame
	state  scrollState
}

// Offset is the first row shown by this layout.
func (l *ScrollLayout) Offset() int {
	if l == nil {
		return 0
	}
	return l.state.offset
}

// Reveal brings row into this staged window with the least movement.
func (l *ScrollLayout) Reveal(row int) { l.RevealRange(row, row) }

// RevealRange brings as much of [first, last] into this staged window as fits.
func (l *ScrollLayout) RevealRange(first, last int) {
	if l == nil || l.scroll == nil || l.state.window <= 0 {
		return
	}
	if last < first {
		first, last = last, first
	}
	l.state.following = false
	switch {
	case first < l.state.offset:
		l.state.offset = first
	case last >= layout.Sum(l.state.offset, l.state.window):
		l.state.offset = min(layout.Remaining(last, l.state.window-1), first)
	}
	l.state.clamp()
	l.scroll.stageState(l.frame, l.state)
}

func (s *Scroll) stageState(frame Frame, state scrollState) {
	if frame.transaction == nil || !frame.transaction.active {
		panic("headless: scroll layout staged outside Root.Draw")
	}
	if s.staged != frame.transaction {
		if s.staged != nil {
			panic("headless: scroll layout staged by two roots")
		}
		s.staged = frame.transaction
		frame.transaction.states = append(frame.transaction.states, s)
	}
	s.pendingLayout = state
}

func (s *Scroll) commit(tx *transaction) {
	if s.staged != tx {
		return
	}
	s.apply(s.pendingLayout)
	s.pendingLayout = scrollState{}
	s.staged = nil
}

func (s *Scroll) abort(tx *transaction) {
	if s.staged != tx {
		return
	}
	s.pendingLayout = scrollState{}
	s.staged = nil
}

func (s *Scroll) state() scrollState {
	return scrollState{
		offset: s.offset, following: s.following,
		total: s.total, window: s.window,
	}
}

func (s *Scroll) apply(state scrollState) {
	s.offset, s.following = state.offset, state.following
	s.total, s.window = state.total, state.window
}

type scrollState struct {
	offset, total, window int
	following             bool
}

func (s *scrollState) layout(total, window int) {
	s.total, s.window = max(total, 0), max(window, 0)
	if s.following {
		s.offset = s.max()
		return
	}
	s.clamp()
}

func (s *scrollState) max() int { return layout.Remaining(s.total, s.window) }

func (s *scrollState) clamp() { s.offset = min(max(s.offset, 0), s.max()) }

// scrollOffset applies a signed movement inside [0, limit] without letting either
// direction wrap before it is clamped.
func scrollOffset(at, by, limit int) int {
	limit = max(limit, 0)
	at = min(max(at, 0), limit)
	if by >= 0 {
		return min(layout.Sum(at, by), limit)
	}
	if by <= -at {
		return 0
	}
	return at + by
}

// scrollPages multiplies a signed page count by a positive page size, saturating
// before multiplication. By then clamps the result to the actual scroll range.
func scrollPages(pages, size int) int {
	size = max(size, 1)
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	switch {
	case pages > maxInt/size:
		return maxInt
	case pages < minInt/size:
		return minInt
	default:
		return pages * size
	}
}
