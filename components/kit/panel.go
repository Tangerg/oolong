package kit

import (
	"image"

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
	// Of is the child. Requiring a focusable child is what makes Panel's own Focus
	// and Handle methods truthful; use Box directly for passive content.
	Of headless.Focusable

	content headless.Snapshot[image.Rectangle]
	// held says the child took a press and therefore owns the gesture until it is
	// released, wherever the pointer goes in the meantime.
	held bool
}

// NewPanel returns a rounded panel around of.
func NewPanel(theme Theme, glyphs Glyphs, of headless.Focusable) *Panel {
	return &Panel{Box: Box{Theme: theme, Glyphs: glyphs}, Of: of}
}

// Draw paints the box and draws the child in its interior.
func (p *Panel) Draw(frame headless.Frame) {
	if p == nil {
		return
	}
	p.content.Stage(frame, image.Rectangle{})
	_ = p.Box.Draw(frame.View)
	inner := p.Box.InnerRect(frame.Bounds().Size())
	p.content.Stage(frame, inner)
	if p.Of != nil && !inner.Empty() {
		p.Of.Draw(frame.Sub(inner))
	}
}

// Measure reports the child's measured height plus the frame and padding.
func (p *Panel) Measure(across int) int {
	if p == nil {
		return 0
	}
	overhead := p.Box.Overhead()
	measurer, ok := p.Of.(layout.Measurer)
	if !ok {
		return overhead.Y
	}
	return overhead.Y + max(measurer.Measure(max(across-overhead.X, 0)), 0)
}

// Focus passes keyboard ownership to the child.
func (p *Panel) Focus(has bool) {
	if p != nil && p.Of != nil {
		p.Of.Focus(has)
	}
}

// Handle passes keys to the child and translates pointer events into the
// interior's coordinates. A press over the frame itself is declined.
func (p *Panel) Handle(event input.Event) bool {
	if p == nil || p.Of == nil {
		return false
	}
	if pointer, ok := event.(input.Mouse); ok {
		return p.mouse(pointer)
	}
	return p.Of.Handle(event)
}

// mouse routes a pointer event into the interior, keeping a gesture that began
// there after the pointer has left it.
//
// This is the rule [headless.Container] and [headless.Stack] already keep, and it
// has to be kept again here because a panel is another place a gesture is handed
// through. A press decides who owns the interaction; the drag and release that
// follow belong to that owner wherever the pointer wanders. Testing the interior on
// every event instead would stop a selection at the frame and never deliver the
// release that ends it, leaving the child believing it is still being dragged.
func (p *Panel) mouse(pointer input.Mouse) bool {
	content := p.content.Value()
	if content.Empty() {
		p.held = false
		return false
	}
	held := p.held && (pointer.Action == input.MouseDrag || pointer.Action == input.MouseUp)
	if !held && !pointer.Pos.In(content) {
		return false
	}
	if pointer.Action == input.MouseUp {
		p.held = false
	}
	pointer.Pos = pointer.Pos.Sub(content.Min)
	handled := p.Of.Handle(pointer)
	if pointer.Action == input.MouseDown {
		// Only a press the child wanted takes the gesture. A press it declined is
		// not an interaction, and holding one would swallow the release as well.
		p.held = handled
	}
	return handled
}
