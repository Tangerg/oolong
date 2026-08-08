package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// Panel puts a framed surface around one focusable widget.
//
// It is the live counterpart to [Box]. Box is deliberately only chrome: it can
// frame text, a widget, or nothing, and therefore owns no child or interaction
// lifecycle. A Panel is deliberately narrower. Its child can take the keyboard,
// so the panel can preserve that capability, translate pointer coordinates through
// the frame, and participate in a [headless.Container] without pretending a passive
// child is focusable.
//
// Use [NewPanel] for the ordinary rounded panel. The zero Panel has no child and
// draws the zero Box.
type Panel struct {
	// Box is the panel's frame, fill, padding, title, and footer. Replacing it changes
	// appearance and interior geometry, never the child's behavior.
	Box Box

	child   headless.Focusable
	blurred bool
	content headless.PointerRegion
}

// NewPanel returns a rounded panel around of.
func NewPanel(theme Theme, glyphs Glyphs, of headless.Focusable) *Panel {
	p := &Panel{Box: Box{Theme: theme, Glyphs: glyphs}}
	p.SetContent(of)
	return p
}

// Content returns the child inside the panel.
func (p *Panel) Content() headless.Focusable {
	if p == nil {
		return nil
	}
	return p.child
}

// SetContent replaces the child and transfers the panel's keyboard ownership.
// Requiring a focusable child is what makes Panel's own Focus and Handle methods
// truthful; use [Box] directly for passive content.
func (p *Panel) SetContent(child headless.Focusable) {
	if p == nil {
		return
	}
	if p.child != nil {
		p.child.Focus(false)
	}
	p.child = child
	if p.child != nil {
		p.child.Focus(!p.blurred)
	}
}

// Draw paints the box and draws the child in its interior.
func (p *Panel) Draw(frame headless.Frame) {
	if p == nil {
		return
	}
	inner := p.Box.InnerRect(frame.Bounds().Size())
	p.Box.paint(frame.View)
	child := p.child
	p.content.Stage(frame, inner, child)
	if child != nil && !inner.Empty() {
		child.Draw(frame.Sub(inner))
	}
}

// Measure reports the child's measured height plus the frame and padding.
func (p *Panel) Measure(across int) int {
	if p == nil {
		return 0
	}
	overhead := p.Box.Overhead()
	measurer, ok := p.child.(layout.Measurer)
	if !ok {
		return overhead.Y
	}
	return layout.Sum(overhead.Y, measurer.Measure(layout.Remaining(across, overhead.X)))
}

// Focus passes keyboard ownership to the child.
func (p *Panel) Focus(has bool) {
	if p == nil || p.blurred == !has {
		return
	}
	p.blurred = !has
	if p.child != nil {
		p.child.Focus(has)
	}
}

// Handle passes keys to the child and translates pointer events into the
// interior's coordinates. A press over the frame itself is declined.
func (p *Panel) Handle(event input.Event) bool {
	if p == nil {
		return false
	}
	if pointer, ok := event.(input.Mouse); ok {
		handled, _ := p.content.Handle(pointer)
		return handled
	}
	if p.child == nil {
		return false
	}
	return p.child.Handle(event)
}
