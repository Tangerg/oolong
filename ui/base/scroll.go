package base

import (
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
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
	element scene.Element
	noCopy  noCopy

	// current is the committed position and bounds. Keeping the four values in the
	// same entity used by a staged frame gives scrolling one implementation: input
	// changes current, while Draw changes a copy until the root commits it.
	current      scrollState
	reveal       *scrollRange
	revision     uint64
	presentation scene.Snapshot[scrollProjection]
	// wheel turns the terminal's reports into rows, keeping the part of a row a
	// report was worth but did not fill.
	wheel   input.Advance
	matcher keymap.Matcher
}

// layout updates committed bounds before an owner-side semantic operation. Drawing
// uses Stage so new bounds become visible only with the complete root frame.
func (s *Scroll) layout(total, window int) {
	s.begin()
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
func (s *Scroll) Wheel(w input.Wheel) {
	requireMutable(s.element)
	s.wheel.Wheel(w)
}

// Offset is the current scroll position. Input may change it before another frame
// commits; use [List.Hit] or [Filter.Hit] to query rows in the presented frame.
func (s *Scroll) Offset() int { return s.resolved().offset }

// FollowingEnd reports whether the window is following the end of the content.
func (s *Scroll) FollowingEnd() bool { return s.current.following }

// By scrolls a number of rows: negative towards the start, positive towards the end.
//
// Reaching the end starts following again, which is what every log viewer does and
// what a reader means by scrolling to the bottom.
func (s *Scroll) By(rows int) {
	s.begin()
	s.reveal = nil
	s.current.by(rows)
}

// ToBottom follows the end of the content.
func (s *Scroll) ToBottom() {
	s.begin()
	s.reveal = nil
	s.current.toBottom()
}

// ToTop shows the start of the content and stops following.
func (s *Scroll) ToTop() {
	s.begin()
	s.reveal = nil
	s.current.toTop()
}

// Discard removes rows from the start of the content while preserving the row under
// the reader when it still exists.
//
// A streaming transcript uses this after publishing a prefix. A following window
// stays at the new end; a reader above the new start lands on the first retained row.
func (s *Scroll) Discard(rows int) {
	s.begin()
	s.current.discard(rows)
	if s.reveal != nil {
		s.reveal = &scrollRange{
			first: layout.Remaining(s.reveal.first, rows), last: layout.Remaining(s.reveal.last, rows),
			origin: layout.Remaining(s.reveal.origin, rows),
		}
	}
}

// Reveal scrolls as little as it can to bring as much of [first, last] into the
// window as will fit.
//
// As little as it can, rather than centring it: a reader stepping through search
// results wants the surrounding text to stay put where it already fits, and a view
// that jumped every time would lose the context that made the result worth finding.
// A range already visible moves nothing at all. Passing the same row twice reveals
// one row. The request is applied again against the next complete frame
// before being consumed, so it also works before the first layout or after a resize.
// Manual scrolling cancels an uncommitted request.
//
// It stops following the end, because a range was asked for and following would
// immediately scroll away from it.
// When the range is taller than the window its start wins, because that is where
// reading begins. Anything else would show the end of a match and leave the reader
// to scroll backwards to find out what it was part of.
func (s *Scroll) Reveal(first, last int) {
	s.begin()
	s.reveal = &scrollRange{first: first, last: last, origin: s.current.offset}
	s.current.following = false
	s.current.revealRange(first, last)
}

// Pages scrolls whole windows, keeping one row of overlap so the reader has
// something to recognise on the other side of the jump.
func (s *Scroll) Pages(n int) {
	s.begin()
	s.reveal = nil
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
	requireMutable(s.element)
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
// current only when the complete [scene.Root] frame commits, so input during a nested draw
// continues to see the previous frame. [ScrollLayout.Reveal] updates this staged
// layout rather than the committed scroll. One Scroll may be staged once per frame;
// use the returned ScrollLayout to refine that one pending value.
// It panics during read-only projection or outside the attached owner's scene.
// An independent Scroll without an Element follows the supplied frame's lifetime.
func (s *Scroll) Stage(frame scene.Frame, total, window int) ScrollLayout {
	if !s.element.IsZero() {
		requireProjection(s.element, frame)
	}
	state := s.project(total, window)
	value := scrollProjection{state: state, revision: s.revision, reveal: s.reveal}
	staged := s.presentation.Stage(frame, value)
	return ScrollLayout{staged: staged, reveal: s.reveal, origin: s.current.offset}
}

// Resolve is the pure layout projection used when a scene group positions content.
func (s *Scroll) Resolve(total, window int) int { return s.project(total, window).offset }

func (s *Scroll) project(total, window int) scrollState {
	state := s.current
	if s.reveal != nil {
		state.offset = s.reveal.origin
	}
	state.layout(total, window)
	if s.reveal != nil {
		state.revealRange(s.reveal.first, s.reveal.last)
	}
	return state
}

func (s *Scroll) resolved() scrollState {
	shown := s.presentation.Value()
	if shown.revision == s.revision {
		return shown.state
	}
	return s.current
}

// A command starts at the last displayed projection, then owns its new intent.
// Painting never consumes that intent or advances the scroll model.
func (s *Scroll) begin() {
	requireMutable(s.element)
	shown := s.presentation.Value()
	if shown.revision == s.revision {
		s.current = shown.state
		if shown.state.window > 0 && shown.reveal == s.reveal {
			s.reveal = nil
		}
	}
	s.revision++
}

type scrollProjection struct {
	state    scrollState
	revision uint64
	reveal   *scrollRange
}

// ScrollLayout borrows the sole staged scroll projection during its owning Prepare
// call. It must not be copied after first use. Its operations panic after Prepare
// returns; read Scroll.Offset for the current controller position.
type ScrollLayout struct {
	noCopy noCopy
	staged *scene.Staged[scrollProjection]
	reveal *scrollRange
	origin int
}

// Offset returns the offset of this in-progress candidate layout.
// It panics after the frame's preparation borrow has expired.
func (l *ScrollLayout) Offset() int {
	if l == nil || l.staged == nil {
		return 0
	}
	return l.staged.Value().state.offset
}

// Reveal refines this candidate to include the requested rows without changing scroll intent.
// It panics after Prepare returns or during the frame owner's read-only projection.
func (l *ScrollLayout) Reveal(first, last int) {
	if l == nil || l.staged == nil {
		return
	}
	value := l.staged.Value()
	value.state.offset = l.origin
	value.state.revealRange(first, last)
	l.staged.Set(value)
	l.reveal = &scrollRange{first: first, last: last}
}

// Resize refines the candidate viewport, retaining its original offset before provisional clamping.
// It panics after Prepare returns or during the frame owner's read-only projection.
func (l *ScrollLayout) Resize(window int) {
	if l == nil || l.staged == nil {
		return
	}
	value := l.staged.Value()
	if !value.state.following {
		value.state.offset = l.origin
	}
	value.state.layout(value.state.total, window)
	if l.reveal != nil {
		value.state.revealRange(l.reveal.first, l.reveal.last)
	}
	l.staged.Set(value)
}

type scrollRange struct {
	first, last int
	// origin is the position before an eager Reveal. A new frame resolves the
	// request from there so its old window cannot over-scroll a larger new one.
	origin int
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

// Anchor maps a retained logical reading position into this candidate layout.
// Following and an explicit reveal have priority because they are user intent.
// A refinement panics after Prepare returns or during the frame owner's read-only projection.
func (l *ScrollLayout) Anchor(offset int) {
	if l == nil || l.staged == nil {
		return
	}
	value := l.staged.Value()
	if value.state.following || l.reveal != nil {
		return
	}
	value.state.offset = min(max(offset, 0), value.state.max())
	l.staged.Set(value)
}
