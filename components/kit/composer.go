package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Composer is the thing the user types into: a prompt marker, a field that grows
// with what is in it, and a row of hints under it.
//
// It is the block at the bottom of a streaming interface, and the reason this
// package exists — assembling one out of [headless.Editor], a marker, a placeholder
// and a hint row is the same forty lines in every program, and forty lines is enough
// to stop someone finding out whether the library is any good.
//
// Enter is left alone, as [headless.Editor] leaves it: whether it sends or breaks the
// line is the interface's decision and not a widget's. Ask [Composer.Text] for what
// was typed and [Composer.Reset] to clear it.
type Composer struct {
	Theme Theme
	// Prompt marks the first row of the field. Empty draws no marker and gives the
	// columns back to the text.
	Prompt string
	// Placeholder is shown while the field is empty.
	Placeholder string
	// Hints are drawn under the field, in the order they matter. Empty draws no row.
	Hints []headless.Binding
	// MaxRows caps how tall the field grows before it scrolls. Zero uses
	// [DefaultComposerRows].
	MaxRows int

	editor headless.Editor
	// width is how wide the field was in the last frame, which is what a click has to
	// be resolved against: a click is about a frame that has already been drawn.
	width int
}

// DefaultComposerRows is how tall a composer grows before it starts scrolling:
// enough for a paragraph, few enough to leave the transcript visible above it.
const DefaultComposerRows = 8

// Text is what has been typed.
func (c *Composer) Text() string { return c.editor.Text() }

// SetText replaces what has been typed and puts the cursor at the end.
func (c *Composer) SetText(s string) { c.editor.SetText(s) }

// Empty reports whether nothing has been typed.
func (c *Composer) Empty() bool { return c.editor.Empty() }

// Reset empties the field, which is what to do once a message has been sent.
func (c *Composer) Reset() { c.editor.Clear() }

// Editor is the field itself, for an interface that needs the cursor, the undo
// history, or a completion offered against what is being typed.
func (c *Composer) Editor() *headless.Editor { return &c.editor }

// Focus takes the keyboard, or gives it up, and passes the news to the field. A
// composer without it draws no cursor — see [headless.Focusable].
func (c *Composer) Focus(has bool) { c.editor.Focus(has) }

// Handle passes input to the field.
//
// A mouse event is translated into the field's own box first, which is something only
// whatever drew the composer can do: the field is inset by the marker and the position
// on screen means nothing to it. The width is remembered from the last frame, because
// a click can only be about a frame that has already been drawn.
func (c *Composer) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		if c.width <= 0 {
			return false
		}
		mouse.Pos.X -= c.markerWidth()
		return c.editor.HandleMouse(mouse, c.width)
	}
	return c.editor.Handle(ev)
}

// Measure is how many rows the composer needs at this width: the field, and a row
// for the hints when there are any.
func (c *Composer) Measure(width int) int {
	c.editor.MaxRows = c.rows()
	return c.editor.Measure(max(width-c.markerWidth(), 0)) + c.hintRows()
}

// Draw paints the marker, the field and the hints.
func (c *Composer) Draw(v grid.View) {
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	c.editor.MaxRows = c.rows()
	c.width = max(width-c.markerWidth(), 0)
	c.editor.Style = c.Theme.Text
	c.editor.PlaceholderStyle = c.Theme.Subtle
	c.editor.SelectionStyle = c.Theme.Selection
	c.editor.Placeholder = c.Placeholder

	rows := layout.Rows(v,
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(c.hintRows())},
	)
	c.drawField(rows[0])
	if c.hintRows() > 0 {
		Help{Theme: c.Theme, Bindings: c.Hints}.Draw(rows[1])
	}
}

// drawField puts the marker on the first row and the field beside it.
func (c *Composer) drawField(v grid.View) {
	marker := c.markerWidth()
	if marker > 0 {
		v.Text(0, 0, c.Prompt, c.Theme.Accent)
	}
	width, height := v.Size()
	c.editor.Draw(v.Sub(grid.Rect(marker, 0, max(width-marker, 0), height)))
}

func (c *Composer) markerWidth() int { return text.Width(c.Prompt) }

func (c *Composer) hintRows() int {
	for _, b := range c.Hints {
		if !b.Hidden {
			return 1
		}
	}
	return 0
}

func (c *Composer) rows() int {
	if c.MaxRows > 0 {
		return c.MaxRows
	}
	return DefaultComposerRows
}
