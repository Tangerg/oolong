package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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
	// Keys say which keystrokes produce which actions. It is the field's map as well
	// as the hint row's, which is what lets a program bind its own send key and the
	// field's own editing keys in one table and have the hints tell the truth about
	// both. Nil leaves the field on [headless.DefaultEditorKeys].
	Keys *keymap.Map
	// Hints are the actions drawn under the field, in the order they matter. An action
	// with nothing bound to it is not shown, so an empty row costs nothing to ask for.
	Hints []keymap.Action
	// MaxRows caps how tall the field grows before it scrolls. Zero uses
	// [DefaultComposerRows].
	MaxRows int

	editor headless.Editor
	// field is the editor box from the last complete root frame.
	field headless.PointerRegion
	// defaults is this composer's private copy of the editor defaults. Keeping it
	// avoids rebuilding the trie at every entry point without sharing a mutable map
	// between independent composers.
	defaults *keymap.Map
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
	c.configure()
	if mouse, ok := ev.(input.Mouse); ok {
		handled, _ := c.field.Handle(mouse)
		return handled
	}
	return c.editor.Handle(ev)
}

// Measure is how many rows the composer needs at this width: the field, and a row
// for the hints when there are any.
func (c *Composer) Measure(width int) int {
	c.configure()
	return c.editor.Measure(max(width-c.markerWidth(), 0)) + c.hintRows()
}

// Draw paints the marker, the field and the hints.
func (c *Composer) Draw(v headless.Frame) {
	c.field.Clear(v)
	c.configure()
	width, height := v.Size()
	if width <= 0 || height <= 0 {
		return
	}
	hints := c.hintRows()
	rows := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(hints)},
	))
	c.drawField(rows[0])
	if hints > 0 {
		Help{Theme: c.Theme, Keys: c.editor.Keys, Show: c.Hints}.Draw(rows[1].View)
	}
}

// drawField puts the marker on the first row and the field beside it.
func (c *Composer) drawField(v headless.Frame) {
	marker := c.markerWidth()
	if marker > 0 {
		v.Text(0, 0, c.Prompt, c.Theme.Accent)
	}
	width, height := v.Size()
	field := grid.Rect(marker, 0, max(width-marker, 0), height)
	c.field.Stage(v, field, &c.editor)
	c.editor.Draw(v.Sub(field))
}

func (c *Composer) markerWidth() int { return text.Width(c.Prompt) }

// configure is the single ownership seam between the composition and its editor.
// Every entry point passes through it, so changing a Composer setting cannot leave
// measurement, drawing and input with three different versions of the same field.
func (c *Composer) configure() {
	c.editor.MaxRows = c.rows()
	if c.Keys != nil {
		c.editor.Keys = c.Keys
	} else {
		if c.defaults == nil {
			c.defaults = headless.DefaultEditorKeys()
		}
		c.editor.Keys = c.defaults
	}
	// No glyph set: the marks beside a choice are the only part of a look that comes
	// from one, and an editor has no choices.
	c.editor.Look = c.Theme.Look(Glyphs{})
	c.editor.Placeholder = c.Placeholder
}

// hintRows is whether there is a hint row: one, if any of the actions asked for is
// bound to something. A row of hints nobody can press is a row of nothing, and the
// space is the field's.
func (c *Composer) hintRows() int {
	for _, action := range c.Hints {
		if len(c.editor.Keys.Keys(action)) > 0 {
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
