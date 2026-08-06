package kit

import (
	"image"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// Transcript draws a session's output: the window of it that fits, the header pinned
// above, and whatever is selected or found picked out.
//
// It owns no content. The blocks, where the view is scrolled to, what is selected and
// what a search turned up are all the caller's, held in the headless types that answer
// those questions — this decides what they look like and nothing else.
type Transcript struct {
	// Content is what to draw.
	Content *headless.Transcript
	// Scroll is where in it the window sits.
	Scroll *headless.Scroll
	// Selection picks out cells the user dragged over. Nil selects nothing.
	Selection *headless.Selection
	// Sticky pins a header above the window. Nil pins nothing.
	Sticky *headless.Sticky
	// Matches are highlighted where they fall inside the window, and Current is the
	// index of the one being stepped to, which is drawn differently. A Current
	// outside the matches means none of them is current.
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
}

// Draw fills v with as much of the transcript as fits.
func (t Transcript) Draw(v grid.View) {
	w, h := v.Size()
	if t.Content == nil || w <= 0 || h <= 0 {
		return
	}
	t.Content.Resize(w)

	// Laid out twice, because the two answers depend on each other: how much room the
	// content has depends on the header, and which block is pinned depends on where
	// the content is scrolled to. One pass settles it — the first says roughly where
	// the window is, which is enough to know which header goes above it, and the
	// second lays the scroll out against the room actually left.
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
	t.reveal(h)

	body, from := v, t.layout(h)
	if t.Sticky != nil {
		if pinned, ok := t.Sticky.At(t.Content, from, h); ok && pinned.Rows < h {
			from = t.layout(h - pinned.Rows)
			t.drawHeader(v, pinned)
			body = v.Sub(image.Rect(0, pinned.Rows, w, h))
		}
	}

	t.Content.Draw(body, from)
	t.mark(body, from)
}

// reveal brings the current match into the window.
//
// The whole match, when it fits: a match that crosses a break the width made covers
// several rows, and showing the first and cutting the rest is showing half of what the
// reader asked to see.
func (t Transcript) reveal(rows int) {
	if t.Scroll == nil || t.Current < 0 || t.Current >= len(t.Matches) {
		return
	}
	m := t.Matches[t.Current]
	if len(m.Spans) == 0 {
		return
	}
	t.Scroll.Layout(t.Content.Height(), rows)
	t.Scroll.RevealRange(m.Row, m.Row+len(m.Spans)-1)
}

// layout puts the scroll against a window of the given height and reports where it
// starts. A transcript with no scroll shows its beginning.
func (t Transcript) layout(rows int) int {
	if t.Scroll == nil {
		return 0
	}
	t.Scroll.Layout(t.Content.Height(), max(rows, 0))
	return t.Scroll.Offset()
}

// drawHeader draws the pinned block and the rule under it.
func (t Transcript) drawHeader(v grid.View, pinned headless.Pinned) {
	w, _ := v.Size()
	block := t.Content.Block(pinned.Block)
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
func (t Transcript) mark(v grid.View, from int) {
	w, h := v.Size()
	if t.Selection != nil && t.Selection.Active() {
		for y := range h {
			for x := range w {
				if t.Selection.Covers(from+y, x) {
					restyle(v, x, y, t.Theme.Selection)
				}
			}
		}
	}
	for i, m := range t.Matches {
		style := t.Theme.Selection
		if i == t.Current {
			style = t.Theme.Accent
		}
		for row, span := range m.Spans {
			y := m.Row + row - from
			if y < 0 || y >= h {
				continue
			}
			for x := span.Col; x < span.Col+span.Width && x < w; x++ {
				restyle(v, x, y, style)
			}
		}
	}
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
//	view.Commit(output)
//
// Nothing is committed unless this is called. A block given to the terminal is no
// longer selectable, searchable, or re-wrapped when the window changes, so the choice
// is the program's and is made block by block with
// [headless.Transcript.Finish].
func (t Transcript) Commit(p Printer) int {
	if t.Content == nil || p == nil {
		return 0
	}
	return t.Content.Commit(func(b headless.Sized, rows int) bool {
		p.PrintRows(rows, b.Draw)
		return true
	})
}

// Printer is somewhere finished output can be put permanently.
//
// It is declared here because Commit is its consumer. Concrete output transports can
// satisfy it without this package depending on them.
type Printer interface {
	// PrintRows draws rows into the terminal's own output, above the interface, where
	// they stay after the program exits.
	PrintRows(rows int, draw func(grid.View))
}

// Handle answers a mouse event over the transcript, reporting whether it took it.
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
func (t Transcript) Handle(event input.Event) bool {
	ev, ok := event.(input.Mouse)
	if !ok || t.Content == nil || t.Selection == nil {
		return false
	}
	at := headless.Point{Row: t.offset() + ev.Pos.Y, Col: ev.Pos.X}
	switch ev.Action {
	case input.MouseDown:
		if ev.Button != input.ButtonLeft {
			return false
		}
		run := t.Selection.Clicks.Press(ev)
		// A word or a row, when there is one there. A double-click in the margin has
		// nothing to take, and falls back to starting a selection like any press.
		switch run {
		case 2:
			if t.Selection.SelectWord(t.Content, at) {
				return true
			}
		case 3:
			if t.Selection.SelectLine(t.Content, at) {
				return true
			}
		}
		t.Selection.Begin(at)
		return true
	case input.MouseDrag:
		t.Selection.Extend(at)
		return true
	case input.MouseUp:
		t.Selection.Done()
		return true
	default:
		return false
	}
}

// offset is the row the drawn window starts at, which a position on screen has to be
// added to before it means anything to the content.
func (t Transcript) offset() int {
	if t.Scroll == nil {
		return 0
	}
	return t.Scroll.Offset()
}
