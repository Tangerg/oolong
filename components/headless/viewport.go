package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Viewport shows a window onto content taller than the room there is for it.
//
// There was a scroll position and something that draws a bar, and nothing that put
// content in a box and scrolled it. Every interface that wanted one wrote the same
// three things by hand: measure the content, keep an offset, and hand the content a
// view starting above the window so the rows off the top fall away.
//
// That last part is the whole trick, and it is why this is so short. A view is already
// a clipped window onto a surface, and a widget drawn into one that begins above the
// box lays itself out at its full height and simply loses what is outside — so nothing
// has to be taught about being scrolled. The content does not know, and the cursor it
// places while it is off-screen is discarded rather than drawn in the wrong place.
//
// # What it is not
//
// It does not scroll content that scrolls itself. A [Transcript] measures incrementally
// and keeps its own position, because a session's output is too tall to re-measure
// every frame; a field taller than its box scrolls to keep its own cursor in view. Both
// would fight a window that also had an opinion.
//
// The zero Viewport is empty and shows nothing.
type Viewport struct {
	// Keys say which keystrokes scroll — see [Scroll]. Nil reads through
	// [DefaultScrollKeys], and they are tried only after the content has declined the
	// keystroke, so content with arrow keys of its own keeps them.
	Keys *keymap.Map

	content Sized
	scroll  Scroll
	// presentation identifies the content and offset shown by the last complete
	// root frame, so pointer routing cannot observe a half-built window.
	presentation Snapshot[viewportPresentation]
	// blurred says the window has been told it does not have the keyboard, which it
	// passes on to whatever is inside it.
	blurred bool
}

// NewViewport constructs a window around content.
func NewViewport(content Sized) *Viewport {
	p := &Viewport{}
	p.SetContent(content)
	return p
}

// Content returns what is shown through the window.
func (p *Viewport) Content() Sized {
	if p == nil {
		return nil
	}
	return p.content
}

// SetContent replaces what is shown and transfers keyboard ownership. The scroll
// position is preserved and clamped to the new content on its next layout.
func (p *Viewport) SetContent(content Sized) {
	if p == nil {
		return
	}
	tell(p.content, false)
	p.content = content
	tell(p.content, !p.blurred)
}

// Scroll is the window's position, for a scrollbar drawn beside it.
func (p *Viewport) Scroll() *Scroll { return &p.scroll }

// Measure is how tall the content wants to be, which is what a window inside a
// measured slot asks for: a window that is never scrolled is a window nobody notices.
func (p *Viewport) Measure(across int) int {
	if p.content == nil {
		return 0
	}
	return p.content.Measure(across)
}

// Draw paints as much of the content as fits.
func (p *Viewport) Draw(v Frame) {
	p.presentation.Stage(v, viewportPresentation{})
	w, h := v.Size()
	content := p.content
	if content == nil || w <= 0 || h <= 0 {
		return
	}
	total := content.Measure(w)
	scroll := p.scroll.Stage(v, total, h)
	p.presentation.Stage(v, viewportPresentation{
		content: content,
		offset:  scroll.Offset(),
	})
	// Above the window by however far it is scrolled. The content is given its whole
	// height and draws into it as though nothing were in the way, which is what keeps
	// the scrolling out of everything that is ever put in here.
	top := -scroll.Offset()
	content.Draw(v.Sub(grid.Rect(0, top, w, total)))
}

type viewportPresentation struct {
	content Sized
	offset  int
}

// Handle scrolls, and gives the content whatever is not about scrolling.
//
// The wheel is the window's: content that answered it as well would scroll twice as
// far as the reader asked. Everything else a pointer does belongs to the content, in
// the content's own coordinates, which here means the row it is over rather than the
// row on screen.
func (p *Viewport) Handle(ev input.Event) bool {
	presented := p.presentation.Value()
	if presented.content == nil {
		// A window with nothing in it is not a window. Scrolling it would be consuming
		// keystrokes on behalf of something that is not there.
		return false
	}
	if mouse, ok := ev.(input.Mouse); ok {
		switch mouse.Action {
		case input.WheelUp, input.WheelDown:
			return p.scroll.Handle(mouse, p.Keys)
		default:
		}
		handler, ok := presented.content.(Interactive)
		if !ok {
			return false
		}
		local := mouse
		local.Pos.Y = layout.Translate(local.Pos.Y, presented.offset)
		return handler.Handle(local)
	}
	if handler, ok := p.content.(Interactive); ok && !p.blurred && handler.Handle(ev) {
		return true
	}
	return p.scroll.Handle(ev, p.Keys)
}

// Do runs an action, the content's first and then the window's own. See [Doer].
func (p *Viewport) Do(action keymap.Action) bool {
	if doer, ok := p.content.(Doer); ok && doer.Do(action) {
		return true
	}
	return p.scroll.Do(action)
}

// Focus takes the keyboard, or gives it up, and passes the news to the content. A
// window is a widget like any other, so one goes in a [Container] beside anything else.
func (p *Viewport) Focus(has bool) {
	if p == nil || p.blurred == !has {
		return
	}
	p.blurred = !has
	tell(p.content, has)
}
