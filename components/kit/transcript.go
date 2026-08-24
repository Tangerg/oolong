package kit

import (
	"image"
	"sort"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Transcript draws a session's output: the window of it that fits, the header pinned
// above, and whatever is selected or found picked out.
//
// It owns no content. The blocks, where the view is scrolled to, what is selected and
// what a search turned up are all the caller's, held in the headless types that answer
// those questions — this decides what they look like and nothing else.
//
// A Transcript must not be copied after first use: its committed window and captured
// selection gesture are one appearance owner even though the content remains external.
type Transcript struct {
	noCopy noCopy

	// Content is what to draw.
	Content *headless.Transcript
	// Scroll is where in it the window sits.
	Scroll *headless.Scroll
	// Keys maps scrolling actions. Nil reads through
	// [headless.DefaultScrollKeys]. It is used only when Scroll is non-nil.
	Keys *keymap.Map
	// Selection picks out cells the user dragged over. Nil selects nothing.
	Selection *headless.Selection
	// Sticky pins a header above the window. Nil pins nothing.
	Sticky *headless.Sticky
	// Matches are highlighted where they fall inside the window, and Current is the
	// index of the one being stepped to, which is drawn differently. Matches must be
	// in row order and non-overlapping; [headless.Result.Matches] already has that
	// shape. The order is what lets drawing depend on the visible window rather than
	// on the age of the session. A Current outside the matches means none is current.
	Matches []headless.Match
	Current int

	// Theme is the look. Every part of a transcript has a fixed role in one — a
	// selection is the selection, the match being stepped to is the accent, a pinned
	// header sits on a surface — so there is nothing here to choose between.
	//
	// A caller who wants one of them different passes a theme with that role
	// changed, which is one line and keeps the look in the one place a look lives:
	//
	//	quiet := theme
	//	quiet.Selection = quiet.Sunken
	Theme Theme
	// Glyphs are the characters the rule under a pinned header is drawn with, which
	// is a fact about the terminal rather than about the look.
	Glyphs Glyphs

	presentation headless.Snapshot[transcriptPresentation]
	// dragged is the selection that accepted the current press. Selection may be
	// replaced between frames; the old gesture must be settled on its old owner and
	// never transferred to the replacement.
	dragged *headless.Selection
}

// Draw fills v with as much of the transcript as fits.
func (t *Transcript) Draw(v headless.Frame) {
	w, h := v.Size()
	if t.Content == nil || w <= 0 || h <= 0 {
		t.presentation.Stage(v, transcriptPresentation{content: t.Content, selection: t.Selection})
		return
	}
	content := t.Content.Stage(v, w)
	window := t.window(content, v)
	t.presentation.Stage(v, window.presentation)

	if window.pinned.Rows > 0 {
		t.drawHeader(content, v.View, window.pinned)
	}
	content.Draw(window.body, window.presentation.from)
	t.mark(window.body, window.presentation.from)
}

// window lays out the scroll and optional pinned header as one visual window. The
// resulting view, header and routing projection come from the same calculation, so
// drawing and pointer translation cannot acquire separate geometry paths.
func (t *Transcript) window(content headless.TranscriptLayout, frame headless.Frame) transcriptWindow {
	w, h := frame.Size()
	// One frame-local layout is refined in two steps, because the two answers depend
	// on each other: how much room the
	// content has depends on the header, and which block is pinned depends on where
	// the content is scrolled to. One pass settles it — the first says roughly where
	// the window is, which is enough to know which header goes above it, and the
	// second resizes that same pending layout against the room actually left. Staging
	// the Scroll twice would make the last sibling or call win, so ScrollLayout.Resize
	// is the explicit refinement operation.
	//
	// Doing it once either way is worse. Laying out against the full height leaves the
	// last rows of a transcript unreachable, which a session that follows its own
	// output notices immediately; sizing the header against the reduced height makes
	// the header's own presence change how much of it there is, which has no fixed
	// point at all.
	// The current match is brought into view before anything is laid out against the
	// scroll, because stepping to a match that cannot be seen is not stepping to it.
	// It is done here rather than left to a caller because the caller has no way to
	// know how tall the window turned out to be.
	bodyRect := grid.Rect(0, 0, w, h)
	from := content.StartRow()
	var scroll headless.ScrollLayout
	if t.Scroll != nil {
		scroll = t.Scroll.Stage(frame, content.Height(), h)
		t.reveal(content, &scroll)
		from = content.StartRow() + scroll.Offset()
	}
	window := transcriptWindow{
		body: frame.View,
		presentation: transcriptPresentation{
			content:   t.Content,
			selection: t.Selection,
			body:      bodyRect,
			from:      from,
		},
	}
	if t.Sticky == nil {
		return window
	}
	pinned, ok := t.Sticky.At(content, from, h)
	if !ok || pinned.Rows >= h {
		return window
	}
	if t.Scroll != nil {
		scroll.Resize(layout.Remaining(h, pinned.Rows))
		from = layout.Sum(content.StartRow(), scroll.Offset())
	}
	bodyRect = grid.Rect(0, pinned.Rows, w, layout.Remaining(h, pinned.Rows))
	window.body = frame.Sub(bodyRect).View
	window.pinned = pinned
	window.presentation.body, window.presentation.from = bodyRect, from
	if top, _, exists := content.Extent(pinned.Block); exists && pinned.Visible() > 0 {
		window.presentation.header = grid.Rect(0, 0, w, pinned.Visible())
		window.presentation.headerFrom = layout.Sum(top, pinned.ClipTop)
	}
	return window
}

// reveal brings the current match into the window.
//
// The whole match, when it fits: a match that crosses a break the width made covers
// several rows, and showing the first and cutting the rest is showing half of what the
// reader asked to see.
func (t *Transcript) reveal(content headless.TranscriptLayout, scroll *headless.ScrollLayout) {
	if scroll == nil || t.Current < 0 || t.Current >= len(t.Matches) {
		return
	}
	m := t.Matches[t.Current]
	if len(m.Spans) == 0 {
		return
	}
	start := content.StartRow()
	scroll.Reveal(m.Row-start, m.Row-start+len(m.Spans)-1)
}

// drawHeader draws the pinned block and the rule under it.
func (t *Transcript) drawHeader(content headless.TranscriptLayout, v grid.View, pinned headless.Pinned) {
	w, _ := v.Size()
	block := content.Block(pinned.Block)
	if block == nil {
		return
	}
	// Drawn into a view that starts above the space available, so the rows clipped
	// off the top are discarded rather than each widget being taught about being
	// pushed — the same arrangement a transcript uses for a block the window cuts.
	visible := pinned.Visible()
	block.Draw(v.Sub(image.Rect(0, -pinned.ClipTop, w, visible)))
	for y := range visible {
		for x := range w {
			restyle(v, x, y, t.Theme.Surface)
		}
	}
	// The header dissolves into whatever it is drawn on as the next one pushes it
	// off, rather than sliding out at full strength and vanishing at the edge. What
	// it fades toward is different in every cell — its own background where the
	// theme gave it one, the terminal's where it did not — which is why this is a
	// fade and not a sheet of colour.
	v.Fade(image.Rect(0, 0, w, visible), 1-pinned.Fade)
	if t.Glyphs.Horizontal != "" && pinned.Rows > visible {
		for x := range w {
			v.Text(x, visible, t.Glyphs.Horizontal, t.Theme.Divider)
		}
	}
}

// mark lays the selection and the search results over what was drawn.
//
// Over, rather than into: what is selected is a property of the cells and not of the
// content, so it is applied after the content drew itself and without the content
// knowing. A block that had to be told it was selected would have to be told again
// every time the drag moved.
func (t *Transcript) mark(v grid.View, from int) {
	w, h := v.Size()
	if t.Selection != nil && t.Selection.Active() {
		for y := range h {
			for x := range w {
				if t.Selection.Covers(layout.Sum(from, y), x) {
					restyle(v, x, y, t.Theme.Selection)
				}
			}
		}
	}
	start, end := visibleMatches(t.Matches, from, layout.Sum(from, h))
	for i := start; i < end; i++ {
		m := t.Matches[i]
		style := t.Theme.Selection
		if i == t.Current {
			style = t.Theme.Accent
		}
		for row, span := range m.Spans {
			y := layout.Relative(layout.Sum(m.Row, row), from)
			if y < 0 || y >= h {
				continue
			}
			start := max(span.Col, 0)
			end := min(layout.Sum(start, span.Width), w)
			for x := start; x < end; x++ {
				restyle(v, x, y, style)
			}
		}
	}
}

// visibleMatches returns the matches that can touch [from, to). Matches are ordered
// and non-overlapping, so at most the last one beginning before from can cross into
// the window. Search already produces exactly that shape.
func visibleMatches(matches []headless.Match, from, to int) (start, end int) {
	if len(matches) == 0 || from >= to {
		return 0, 0
	}
	start = sort.Search(len(matches), func(i int) bool { return matches[i].Row >= from })
	if start > 0 {
		previous := matches[start-1]
		if layout.Sum(previous.Row, len(previous.Spans)) > from {
			start--
		}
	}
	end = sort.Search(len(matches), func(i int) bool { return matches[i].Row >= to })
	return start, max(start, end)
}

// restyle lays a style over a cell without touching what is in it.
//
// Over, not instead of: a header tinted by replacing every style would lose the accent
// on its own first word, and a selection drawn that way would lose the emphasis inside
// the sentence it covers. Merging keeps whatever the overlay did not state.
func restyle(v grid.View, x, y int, style grid.Style) {
	if style == (grid.Style{}) {
		return
	}
	v.MergeStyle(x, y, style)
}

// Commit gives the transcript's finished leading blocks to a printer, which is what
// puts them in the terminal's own output for good.
//
// It is here rather than on the transcript because printing needs a width to draw at
// and the transcript does not have one until it has been laid out. The rest of the
// rule — leading, in order, once each — is [headless.Transcript.Commit]'s, and this
// only supplies the drawing.
//
// Any output sink with the small Printer method set can receive committed blocks:
//
//	view.Commit(output, 0)
//
// Nothing is committed unless this is called. A block given to the terminal is no
// longer selectable, searchable, or re-wrapped when the window changes, so the choice
// is the program's and is made block by block with [headless.Transcript.Finish].
//
// limit is the most blocks to transfer. Zero transfers every finished block; a
// positive limit lets an application retain a recent window and publish only its
// excess stable prefix. A negative limit is a programmer error.
func (t *Transcript) Commit(p Printer, limit int) int {
	if limit < 0 {
		panic("kit: transcript commit limit cannot be negative")
	}
	if t.Content == nil || p == nil {
		return 0
	}
	before := t.Content.StartRow()
	committed := 0
	gone := t.Content.Commit(func(b headless.Block, _ int) bool {
		if limit > 0 && committed >= limit {
			return false
		}
		p.Print(b)
		committed++
		return true
	})
	if gone == 0 {
		return 0
	}
	if t.Scroll != nil {
		t.Scroll.Discard(t.Content.StartRow() - before)
	}
	if t.Selection != nil {
		t.Selection.DiscardBefore(t.Content.StartRow())
	}
	if t.Sticky != nil {
		t.Sticky.DiscardBefore(t.Content.FirstBlock())
	}
	return gone
}

// Printer is somewhere finished output can be put permanently.
//
// It is declared here because Commit is its consumer. Concrete output transports can
// satisfy it without this package depending on them.
type Printer interface {
	// Print draws content into the terminal's own output, above the interface, where
	// it stays after the program exits.
	Print(content grid.Drawable)
}

// Handle answers scrolling and selection over the transcript, reporting whether it
// took the event.
//
// A press starts a selection, a drag moves its far end, a second press in the same
// place takes the word and a third takes the row. That is what selecting text means
// everywhere, and it took five pieces wired together by hand until this: a selection, a
// click counter, the word rule, the scroll offset, and the translation from a position
// on screen into a row of the transcript.
//
// The last of those is why it lives here rather than on the selection. A point on
// screen means nothing without knowing where the transcript was drawn and how far it is
// scrolled, and this is the thing that drew it.
//
// The clicks are counted from the time the event arrived with, which the terminal's
// reader stamped on it. A caller feeding events it made up itself gets single clicks,
// because there is nothing to tell one press from another by.
//
// The position is in the transcript's own coordinates, like everything else a widget
// is handed. Whoever drew it is responsible for that, which for anything inside a
// [headless.Container] is the container.
func (t *Transcript) Handle(event input.Event) bool {
	if t.Scroll != nil && t.Scroll.Handle(event, t.Keys) {
		return true
	}
	ev, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	switch ev.Action {
	case input.MouseDown:
		return t.press(ev)
	case input.MouseDrag:
		return t.drag(ev)
	case input.MouseUp:
		return t.release()
	default:
		return false
	}
}

func (t *Transcript) press(event input.Mouse) bool {
	// A new press supersedes a release the terminal never reported.
	t.dragged = nil
	presented := t.presentation.Value()
	if event.Button != input.ButtonLeft || presented.content == nil || presented.selection == nil {
		return false
	}
	at, on := presented.pointAt(event.Pos, false)
	if !on {
		return false
	}
	run := presented.selection.Clicks.Press(event)
	// A word or a row, when there is one there. A double-click in the margin has
	// nothing to take, and falls back to starting a selection like any press.
	switch run {
	case 2:
		if presented.selection.SelectWord(presented.content, at) {
			return true
		}
	case 3:
		if presented.selection.SelectLine(presented.content, at) {
			return true
		}
	}
	presented.selection.Begin(at)
	t.dragged = presented.selection
	return true
}

func (t *Transcript) drag(event input.Mouse) bool {
	if t.dragged == nil || !t.dragged.Dragging() {
		return false
	}
	at, on := t.presentation.Value().pointAt(event.Pos, true)
	if !on {
		return false
	}
	t.dragged.Extend(at)
	return true
}

func (t *Transcript) release() bool {
	if t.dragged == nil {
		return false
	}
	// A release is a lifetime transition, not a hit test. Once this selection
	// accepted the press, it must be settled even when the pointer is now outside the
	// transcript or the visible window has collapsed.
	dragged := t.dragged
	t.dragged = nil
	dragged.Done()
	return true
}

type transcriptPresentation struct {
	content          *headless.Transcript
	selection        *headless.Selection
	body, header     image.Rectangle
	from, headerFrom int
}

// transcriptWindow is one frame's complete visual projection. Keeping its body,
// optional header and routing geometry together makes it impossible to draw one
// arrangement and publish another by assembling them along separate paths.
type transcriptWindow struct {
	body         grid.View
	presentation transcriptPresentation
	pinned       headless.Pinned
}

// pointAt translates a screen point to the transcript. When nearest is true, a point
// outside the visible rows is clamped to their nearest edge. That is the continuation
// of an existing drag, not a new hit: dragging beyond a text window extends to the
// first or last visible cell, while a press beyond it still belongs to nobody here.
func (p transcriptPresentation) pointAt(point image.Point, nearest bool) (headless.Point, bool) {
	area, row, ok := p.rowAt(point)
	if !ok && nearest {
		area, row, ok = p.nearestRow(point.Y)
	}
	if !ok || area.Empty() {
		return headless.Point{}, false
	}
	col := point.X - area.Min.X
	if nearest {
		col = min(max(col, 0), area.Dx()-1)
	} else if col < 0 || col >= area.Dx() {
		return headless.Point{}, false
	}
	return headless.Point{Row: row, Col: col}, true
}

func (p transcriptPresentation) rowAt(point image.Point) (image.Rectangle, int, bool) {
	switch {
	case point.In(p.header):
		return p.header, layout.Sum(p.headerFrom, layout.Relative(point.Y, p.header.Min.Y)), true
	case point.In(p.body):
		return p.body, layout.Sum(p.from, layout.Relative(point.Y, p.body.Min.Y)), true
	default:
		return image.Rectangle{}, 0, false
	}
}

func (p transcriptPresentation) nearestRow(y int) (image.Rectangle, int, bool) {
	if !p.header.Empty() && y < p.body.Min.Y {
		at := min(max(y, p.header.Min.Y), p.header.Max.Y-1)
		return p.header, layout.Sum(p.headerFrom, layout.Relative(at, p.header.Min.Y)), true
	}
	if !p.body.Empty() {
		at := min(max(y, p.body.Min.Y), p.body.Max.Y-1)
		return p.body, layout.Sum(p.from, layout.Relative(at, p.body.Min.Y)), true
	}
	if !p.header.Empty() {
		at := min(max(y, p.header.Min.Y), p.header.Max.Y-1)
		return p.header, layout.Sum(p.headerFrom, layout.Relative(at, p.header.Min.Y)), true
	}
	return image.Rectangle{}, 0, false
}
