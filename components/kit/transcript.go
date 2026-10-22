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

// Transcript renders caller-owned content, scrolling, selection and search. Its
// committed window and captured gesture own appearance routing; a Transcript must
// not be copied after first use.
type Transcript struct {
	noCopy noCopy

	Content *headless.Transcript
	Scroll  *headless.Scroll
	// Keys maps scrolling actions. Nil reads through
	// [headless.DefaultScrollKeys]. It is used only when Scroll is non-nil.
	Keys      *keymap.Map
	Selection *headless.Selection
	// Sticky pins a header above the window. Nil pins nothing.
	Sticky *headless.Sticky
	// Matches are highlighted where they fall inside the window, and Current is the
	// index of the highlighted match. Use [Transcript.RevealMatch] to navigate to it. Matches must be
	// in row order and non-overlapping; [headless.Result.Matches] already has that
	// shape. The order is what lets drawing depend on the visible window rather than
	// on the age of the session. A Current outside the matches means none is current.
	Matches []headless.Match
	Current int

	Theme  Theme
	Glyphs Glyphs

	presentation headless.Snapshot[transcriptPresentation]
	// clicked is the presentation lifetime that received the last counted press.
	// Selection owns the count; a different target starts a new click run.
	clicked *byte
	// dragged retains the selection and the presentation lifetime that took the
	// press. Another document's geometry cannot advance that selection.
	dragged *transcriptGesture
}

type transcriptGesture struct {
	selection *headless.Selection
	identity  *byte
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
	previous := t.presentation.Value()
	if t.Selection != nil && !v.Visible().Empty() {
		window.presentation.identity = previous.identity
		if previous.identity == nil || previous.content != t.Content || previous.selection != t.Selection {
			window.presentation.identity = new(byte)
		}
	}
	t.presentation.Stage(v, window.presentation)

	if window.pinned.Rows > 0 {
		t.drawHeader(content, v.View, window.pinned)
		// A pinned header is the same content rows shown somewhere else, and what is
		// selected is a property of the rows. A press lands in the header and the
		// selection takes those rows — see [transcriptPresentation.rowAt] — so
		// leaving the header unmarked meant dragging over it did something the screen
		// never admitted to.
		if header := window.presentation.header; !header.Empty() {
			t.mark(v.View.Sub(header), window.presentation.headerFrom)
		}
	}
	content.Draw(window.body, window.presentation.from)
	t.mark(window.body, window.presentation.from)
}

// window lays out the scroll and optional pinned header as one visual window. The
// resulting view, header and routing projection come from the same calculation, so
// drawing and pointer translation cannot acquire separate geometry paths.
func (t *Transcript) window(content headless.TranscriptLayout, frame headless.Frame) transcriptWindow {
	w, h := frame.Size()
	// Two steps, because the answers depend on each other: the room the content has
	// depends on the header, and which block is pinned depends on where the content is
	// scrolled to. Laying out against the full height leaves the last rows unreachable,
	// and sizing the header against the reduced height has no fixed point at all.
	// Staging the Scroll twice would make the last caller win, so ScrollLayout.Resize
	// refines the same pending layout.
	bodyRect := grid.Area(0, 0, w, h)
	from := content.StartRow()
	var scroll headless.ScrollLayout
	if t.Scroll != nil {
		scroll = t.Scroll.Stage(frame, content.Height(), h)
		from = content.StartRow() + scroll.Offset()
	}
	window := transcriptWindow{
		body: frame.View,
		presentation: transcriptPresentation{
			content:   t.Content,
			selection: t.Selection,
			scroll:    t.Scroll,
			body:      bodyRect,
			visible:   frame.Visible(),
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
	bodyRect = grid.Area(0, pinned.Rows, w, layout.Remaining(h, pinned.Rows))
	window.body = frame.Sub(bodyRect).View
	window.pinned = pinned
	window.presentation.body, window.presentation.from = bodyRect, from
	if top, _, exists := content.Extent(pinned.Block); exists && pinned.Visible() > 0 {
		window.presentation.header = grid.Area(0, 0, w, pinned.Visible())
		window.presentation.headerFrom = layout.Sum(top, pinned.ClipTop)
	}
	return window
}

// RevealMatch highlights index and requests that match's rows in the next complete
// frame. It returns false for an unavailable match, content or scroll. Current on
// its own only controls highlighting; redraws never repeat a navigation request.
func (t *Transcript) RevealMatch(index int) bool {
	if t.Content == nil || t.Scroll == nil || index < 0 || index >= len(t.Matches) {
		return false
	}
	match := t.Matches[index]
	if len(match.Spans) == 0 {
		return false
	}
	t.Current = index
	first := match.Row - t.Content.StartRow()
	t.Scroll.Reveal(first, layout.Sum(first, len(match.Spans)-1))
	return true
}

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

// Apply selection after content drawing so content need not own interaction state.
func (t *Transcript) mark(v grid.View, from int) {
	t.markSelection(v, from)
	t.markMatches(v, from)
}

func (t *Transcript) markSelection(v grid.View, from int) {
	if t.Selection == nil || !t.Selection.Active() {
		return
	}
	w, h := v.Size()
	for y := range h {
		for x := range w {
			if t.Selection.Covers(layout.Sum(from, y), x) {
				restyle(v, x, y, t.Theme.Selection)
			}
		}
	}
}

func (t *Transcript) markMatches(v grid.View, from int) {
	w, h := v.Size()
	first, last := visibleMatches(t.Matches, from, layout.Sum(from, h))
	for i := first; i < last; i++ {
		style := t.Theme.Selection
		if i == t.Current {
			style = t.Theme.Accent
		}
		for row, span := range t.Matches[i].Spans {
			y := layout.Relative(layout.Sum(t.Matches[i].Row, row), from)
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

// Merge overlays so selection cannot erase emphasis already present in the content.
func restyle(v grid.View, x, y int, style grid.Style) {
	if style == (grid.Style{}) {
		return
	}
	v.MergeStyle(x, y, style)
}

// Commit prints finished leading blocks through [headless.Transcript.Commit] at
// the committed width. Zero limit transfers all; negative limits panic because
// publication is irreversible. Nil printer or content transfers nothing. The result
// counts only accepted blocks; a printer error retains the refused block and suffix
// and is returned with any accepted prefix. The content and its scroll, selection
// and sticky state are borrowed together for this operation, even if p replaces the
// view's fields while publishing. Accepted blocks settle those models even if a
// later printer call panics or exits its goroutine; the abnormal exit propagates.
func (t *Transcript) Commit(p Printer, limit int) (int, error) {
	if limit < 0 {
		panic("kit: transcript commit limit cannot be negative")
	}
	if t.Content == nil || p == nil {
		return 0, nil
	}
	content, scroll, selection, sticky := t.Content, t.Scroll, t.Selection, t.Sticky
	before, first := content.StartRow(), content.FirstBlock()
	defer func() {
		if content.FirstBlock() == first {
			return
		}
		if scroll != nil {
			scroll.Discard(content.StartRow() - before)
		}
		if selection != nil {
			selection.DiscardBefore(content.StartRow())
		}
		if sticky != nil {
			sticky.DiscardBefore(content.FirstBlock())
		}
	}()
	committed := 0
	var err error
	gone := content.Commit(func(b headless.Block, _ int) bool {
		if limit > 0 && committed >= limit {
			return false
		}
		if err = p.Print(b); err != nil {
			return false
		}
		committed++
		return true
	})
	return gone, err
}

// Printer is somewhere finished output can be put permanently.
//
// It is declared here because Commit is its consumer. Concrete output transports can
// satisfy it without this package depending on them.
type Printer interface {
	// Print accepts content for finished output above the interface. A nil error
	// transfers ownership to the printer; an error refuses it and leaves content
	// with the caller. Acceptance does not
	// promise delivery: the printer owns reporting any later transport failure.
	Print(content grid.Drawable) error
}

// Handle accepts scrolling and left-button selection in local coordinates.
// Event timestamps count repeated clicks: two select a word, three a row.
// A matching or unspecified release settles the original selection even outside
// the window; foreign-button releases preserve it. Presses and wheel input use
// committed targets. Target replacement revokes drag coordinates and click runs
// without transferring release ownership to the replacement.
func (t *Transcript) Handle(event input.Event) bool {
	scroll := t.Scroll
	if mouse, ok := event.(input.Mouse); ok && (mouse.Action == input.WheelUp || mouse.Action == input.WheelDown) {
		presented := t.presentation.Value()
		if !mouse.Pos.In(presented.visible) {
			return false
		}
		scroll = presented.scroll
	}
	if scroll != nil && scroll.Handle(event, t.Keys) {
		return true
	}
	ev, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	switch ev.Action {
	case input.MouseCancel:
		t.clicked = nil
		t.release()
		return false
	case input.MouseDown:
		return t.press(ev)
	case input.MouseDrag:
		return t.drag(ev)
	case input.MouseUp:
		if !ev.Releases(input.ButtonLeft) {
			return false
		}
		return t.release()
	default:
		return false
	}
}

func (t *Transcript) press(event input.Mouse) bool {
	// A new press supersedes a release the terminal never reported. The gesture it
	// supersedes is ended and not merely forgotten: dropping the reference leaves
	// the selection saying a drag is still in progress, and this is the only thing
	// that was ever going to tell it otherwise.
	t.release()
	presented := t.presentation.Value()
	if event.Button != input.ButtonLeft || presented.content == nil || presented.selection == nil {
		return false
	}
	at, on := presented.pointAt(event.Pos, false)
	if !on {
		return false
	}
	if t.clicked != presented.identity {
		presented.selection.Clicks.Reset()
		t.clicked = presented.identity
	}
	run := presented.selection.Clicks.Press(event)
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
	t.dragged = &transcriptGesture{selection: presented.selection, identity: presented.identity}
	return true
}

func (t *Transcript) drag(event input.Mouse) bool {
	if t.dragged == nil || !t.dragged.selection.Dragging() || !event.Drags(input.ButtonLeft) {
		return false
	}
	presented := t.presentation.Value()
	if presented.identity != t.dragged.identity {
		return false
	}
	at, on := presented.pointAt(event.Pos, true)
	if !on {
		return false
	}
	t.dragged.selection.Extend(at)
	return true
}

// Release belongs to the selection that accepted the press, even outside the window
// or after its collapse. A new press also settles any missing terminal release.
func (t *Transcript) release() bool {
	if t.dragged == nil {
		return false
	}
	dragged := t.dragged
	t.dragged = nil
	dragged.selection.Done()
	return true
}

type transcriptPresentation struct {
	content          *headless.Transcript
	selection        *headless.Selection
	scroll           *headless.Scroll
	identity         *byte
	body, header     image.Rectangle
	visible          image.Rectangle
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
	if p.visible.Empty() {
		return headless.Point{}, false
	}
	if nearest {
		point.X = min(max(point.X, p.visible.Min.X), p.visible.Max.X-1)
		point.Y = min(max(point.Y, p.visible.Min.Y), p.visible.Max.Y-1)
	} else if !point.In(p.visible) {
		return headless.Point{}, false
	}
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
	header, body := p.header.Intersect(p.visible), p.body.Intersect(p.visible)
	if !header.Empty() && y < p.body.Min.Y {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, layout.Sum(p.headerFrom, layout.Relative(at, p.header.Min.Y)), true
	}
	if !body.Empty() {
		at := min(max(y, body.Min.Y), body.Max.Y-1)
		return p.body, layout.Sum(p.from, layout.Relative(at, p.body.Min.Y)), true
	}
	if !header.Empty() {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, layout.Sum(p.headerFrom, layout.Relative(at, p.header.Min.Y)), true
	}
	return image.Rectangle{}, 0, false
}
