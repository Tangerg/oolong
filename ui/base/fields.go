package base

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Text owns a single-line edit session and validation of its current revision.
// An application reads this owner when submitting; there is no mutable binding mirror.
// Text must not be copied after first use; its collaborators share one live identity.
type Text struct {
	noCopy noCopy
	field
	Label       string
	Check       func(string) error
	Placeholder string
	Keys        *keymap.Map
	Clipboard   Clipboard
	Gutter      RowGutter
	CursorStyle grid.CursorStyle
	editor      Editor
}

func (t *Text) Text() string { return t.editor.Text() }

// SetText replaces the owned answer and records one document edit. It panics during presentation or observation.
func (t *Text) SetText(value string) { t.configureEditor(); t.editor.SetText(value) }

// Cursor returns the caret's byte offset in the answer.
func (t *Text) Cursor() int { _, col := t.editor.Cursor(); return col }

// SetCursor moves the view caret to a valid boundary. It does not change document revision.
func (t *Text) SetCursor(column int) { t.editor.SetCursor(0, column) }

// Revision is the answer's document revision, independent of caret and scroll state.
func (t *Text) Revision() uint64 { return t.editor.Revision() }

// SetMask selects password presentation. It panics for an invisible or joining mask, or during presentation or observation.
func (t *Text) SetMask(mask string) { t.editor.SetMask(mask) }

// Mask returns the presentation glyph, or an empty string for visible text.
func (t *Text) Mask() string { return t.editor.Mask() }

func (t *Text) Prompt() string { return t.Label }

func (t *Text) HeightForWidth(int) int { return layout.Sum(1, t.rows(t.Label)) }

func (t *Text) Draw(v scene.Frame) { t.DrawWith(v, Look{}) }

func (t *Text) DrawWith(v scene.Frame, look Look) {
	view := t.editor.lineView()
	view.placeholder, view.gutter, view.cursorStyle = t.Placeholder, t.Gutter, t.CursorStyle
	view.draw(t.frame(v, t.Label, look), look, &t.editor.presentation)
}

// Handle applies local edit input. Mounted applications route events through Element so focus, capture and written-frame geometry remain scene-owned.
func (t *Text) Handle(event input.Event) bool {
	t.configureEditor()
	if mouse, ok := event.(input.Mouse); ok {
		event = t.local(mouse)
	}
	return t.editor.Handle(event)
}

// Do executes one named editing action against the same editor owner.
func (t *Text) Do(action keymap.Action) bool { t.configureEditor(); return t.editor.Do(action) }

func (t *Text) Validate() error { return t.validate(nil, t.Revision, t.checkAnswer) }

func (t *Text) checkAnswer() error {
	if t.Check == nil {
		return nil
	}
	return t.Check(t.Text())
}

func (t *Text) focusChanged(has bool) {
	t.focus(has, t.editor.focusChanged, t.configureEditor, t.Validate)
}

func (t *Text) configureEditor() {
	requireMutable(t.element)
	t.editor.SetSingleLine(true)
	t.editor.Placeholder, t.editor.Keys = t.Placeholder, t.Keys
	t.editor.Clipboard, t.editor.Gutter, t.editor.CursorStyle = t.Clipboard, t.Gutter, t.CursorStyle
}

func (t *Text) Ask() string {
	if t.Placeholder == "" {
		return t.Label
	}
	return t.Label + " (" + t.Placeholder + ")"
}

// Reply sets and validates a spoken answer through the same document owner.
func (t *Text) Reply(said string) error {
	return t.validate(func() error { t.SetText(said); return nil }, t.Revision, t.checkAnswer)
}
