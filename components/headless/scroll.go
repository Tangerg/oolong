package headless

import (
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
//
// A Scroll must not be copied after first use: its committed and staged positions,
// wheel remainder and key sequence are one mutable owner.
type Scroll struct {
	noCopy noCopy

	// current is the committed position and bounds. Keeping the four values in the
	// same entity used by a staged frame gives scrolling one implementation: input
	// changes current, while Draw changes a copy until the root commits it.
	current scrollState
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

// layout updates committed bounds before an owner-side semantic operation. Drawing
// uses Stage so new bounds become visible only with the complete root frame.
func (s *Scroll) layout(total, window int) {
	s.current.layout(total, window)
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
func (s *Scroll) Offset() int { return s.current.offset }

// AtBottom reports whether the window is following the end of the content.
func (s *Scroll) AtBottom() bool { return s.current.following }

// By scrolls a number of rows: negative towards the start, positive towards the end.
//
// Reaching the end starts following again, which is what every log viewer does and
// what a reader means by scrolling to the bottom.
func (s *Scroll) By(rows int) {
	s.current.by(rows)
}

// ToBottom follows the end of the content.
func (s *Scroll) ToBottom() {
	s.current.toBottom()
}

// ToTop shows the start of the content and stops following.
func (s *Scroll) ToTop() {
	s.current.toTop()
}

// Discard removes rows from the start of the content while preserving the row under
// the reader when it still exists.
//
// A streaming transcript uses this after publishing a prefix. A following window
// stays at the new end; a reader above the new start lands on the first retained row.
func (s *Scroll) Discard(rows int) {
	s.current.discard(rows)
}

// Reveal scrolls as little as it can to bring as much of [first, last] into the
// window as will fit.
//
// As little as it can, rather than centring it: a reader stepping through search
// results wants the surrounding text to stay put where it already fits, and a view
// that jumped every time would lose the context that made the result worth finding.
// A range already visible moves nothing at all. Passing the same row twice reveals
// one row; there is deliberately no second spelling for that degenerate range.
//
// It stops following the end, because a range was asked for and following would
// immediately scroll away from it.
// When the range is taller than the window its start wins, because that is where
// reading begins. Anything else would show the end of a match and leave the reader
// to scroll backwards to find out what it was part of.
func (s *Scroll) Reveal(first, last int) {
	s.current.revealRange(first, last)
}

// Pages scrolls whole windows, keeping one row of overlap so the reader has
// something to recognise on the other side of the jump.
func (s *Scroll) Pages(n int) {
	s.current.pages(n)
}

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
			s.By(s.wheel.Rows(mouse.At, -1))
			return true
		case input.WheelDown:
			s.By(s.wheel.Rows(mouse.At, 1))
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

// Stage derives the scroll layout for a component frame.
//
// The returned layout is the one drawing should use. Its bounds and offset become
// current only when the complete [Root] frame commits, so input during a nested draw
// continues to see the previous frame. [ScrollLayout.Reveal] updates this staged
// layout rather than the committed scroll. One Scroll may be staged once per frame;
// use the returned ScrollLayout to refine that one pending value.
func (s *Scroll) Stage(frame Frame, total, window int) ScrollLayout {
	state := s.current
	state.layout(total, window)
	frame.enlist(s, &s.staged)
	s.pendingLayout = state
	return ScrollLayout{scroll: s, frame: frame, state: state}
}

// ScrollLayout is the derived position of one Scroll in a component frame.
//
// It is a short-lived value returned by [Scroll.Stage]. Offset is used to paint the
// frame; adjustments are staged with the same root transaction.
//
// A ScrollLayout must not be copied after construction. Two copies would carry two
// refinements of one pending Scroll, and whichever wrote last would silently erase
// the other's geometry.
type ScrollLayout struct {
	noCopy noCopy

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

// Reveal brings as much of [first, last] into this staged window as fits. Passing the
// same row twice reveals one row, matching [Scroll.Reveal].
func (l *ScrollLayout) Reveal(first, last int) {
	if l == nil || l.scroll == nil || l.state.window <= 0 {
		return
	}
	l.state.revealRange(first, last)
	l.scroll.updateState(l.frame, l.state)
}

// Resize changes how many rows this frame-local layout can show while retaining its
// total and any Reveal adjustment already made. A transcript uses it after discovering
// that a sticky header consumes part of the provisional window; creating a second
// layout for the same Scroll would make sibling and call order decide which one wins.
func (l *ScrollLayout) Resize(window int) {
	if l == nil || l.scroll == nil {
		return
	}
	l.state.layout(l.state.total, window)
	l.scroll.updateState(l.frame, l.state)
}

func (s *Scroll) updateState(frame Frame, state scrollState) {
	if !frame.owns(s.staged) {
		panic("headless: scroll layout updated outside its owning frame")
	}
	s.pendingLayout = state
}

func (s *Scroll) commit(tx *transaction) {
	if s.staged != tx {
		return
	}
	s.current = s.pendingLayout
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

func (s *scrollState) by(rows int) {
	s.offset = scrollOffset(s.offset, rows, s.max())
	s.following = s.offset >= s.max()
}

func (s *scrollState) toBottom() {
	s.following = true
	s.offset = s.max()
}

func (s *scrollState) toTop() {
	s.following = false
	s.offset = 0
}

func (s *scrollState) discard(rows int) {
	rows = max(rows, 0)
	s.total = layout.Remaining(s.total, rows)
	s.offset = layout.Remaining(s.offset, rows)
	if s.following {
		s.offset = s.max()
		return
	}
	s.clamp()
}

func (s *scrollState) revealRange(first, last int) {
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

func (s *scrollState) pages(n int) {
	s.by(scrollPages(n, max(s.window-1, 1)))
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
