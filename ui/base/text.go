package base

import (
	"context"
	"strings"

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
	label  string
	check  func(string) error
	editor Editor
}

func (t *Text) Text() string { return t.editor.Text() }

// SetText replaces the owned answer and records one document edit. It panics during presentation or observation.
func (t *Text) SetText(value string) { t.line().SetText(value) }

// Cursor returns the caret's byte offset in the answer.
func (t *Text) Cursor() int { _, byteOffset := t.editor.Cursor(); return byteOffset }

// SetCursor moves the view caret to a valid boundary. It does not change document revision.
func (t *Text) SetCursor(byteOffset int) { t.editor.SetCursor(0, byteOffset) }

// Revision is the answer's document revision, independent of caret and scroll state.
func (t *Text) Revision() uint64 { return t.editor.Revision() }

// SetMask selects password presentation. It panics for an invisible or joining mask, or during presentation or observation.
func (t *Text) SetMask(mask string) { t.editor.SetMask(mask) }

// Mask returns the presentation glyph, or an empty string for visible text.
func (t *Text) Mask() string { return t.editor.Mask() }

func (t *Text) Prompt() string { return t.label }

func (t *Text) HeightForWidth(int) int { return layout.Sum(1, t.rows(t.label)) }

func (t *Text) Draw(v scene.Frame) { t.DrawWith(v, Look{}) }

func (t *Text) DrawWith(v scene.Frame, look Look) {
	view := t.editor.lineView()
	view.draw(t.frame(v, t.label, look), look, &t.editor.presentation)
}

// Handle applies local edit input. Mounted applications route events through Element so focus, capture and written-frame geometry remain scene-owned.
func (t *Text) Handle(event input.Event) bool {
	if mouse, ok := event.(input.Mouse); ok {
		event = t.local(mouse)
	}
	return t.line().Handle(event)
}

func (t *Text) Do(action keymap.Action) bool {
	return t.line().Do(action)
}

// Paste uses the field's single-line edit owner and the same directed completion
// contract as Editor.Paste, including attachment and cancellation checks.
func (t *Text) Paste(ctx context.Context, complete func(error)) error {
	return t.line().Paste(ctx, complete)
}

// line is the field's editor. Every entry that can change the editor reaches it
// here, so the field is one line even when it was not built by NewText.
func (t *Text) line() *Editor {
	t.editor.SetSingleLine(true)
	return &t.editor
}

// PasteError exposes keyboard paste feedback from the field's existing editor.
// Programmatic paste failures go to their own return or completion receiver.
func (t *Text) PasteError() error { return t.editor.PasteError() }

func (t *Text) Validate() error { return t.validate(nil, t.Revision, t.checkAnswer) }

// SetCheck revokes old results because validation belongs to the rule as well as
// the answer. Reinstalling the same callback also revokes receipts; nil removes it.
func (t *Text) SetCheck(check func(string) error) {
	requireMutable(t.element)
	t.check = check
	t.InvalidateValidation()
}

func (t *Text) checkAnswer() error {
	if t.check == nil {
		return nil
	}
	return t.check(t.Text())
}

func (t *Text) focusChanged(has bool) {
	t.focus(has, t.editor.focusChanged, t.Validate)
}

func (t *Text) Ask() string {
	if t.editor.placeholder == "" {
		return t.label
	}
	return t.label + " (" + t.editor.placeholder + ")"
}

// Reply sets and validates a spoken answer through the same document owner.
func (t *Text) Reply(said string) error {
	return t.validate(func() error { t.SetText(said); return nil }, t.Revision, t.checkAnswer)
}

func (t *Text) Label() string { return t.label }

func (t *Text) SetLabel(value string) {
	element := t.Element()
	requireMutable(element)
	if t.label == value {
		return
	}
	t.label = strings.Clone(value)
	element.Invalidate()
}

func (t *Text) Placeholder() string { return t.editor.Placeholder() }

func (t *Text) SetPlaceholder(value string) {
	t.Element()
	t.editor.SetPlaceholder(value)
}

func (t *Text) Keys() *keymap.Map { return t.editor.Keys() }

func (t *Text) SetKeys(value *keymap.Map) {
	t.Element()
	t.editor.SetKeys(value)
}

func (t *Text) Clipboard() Clipboard { return t.editor.Clipboard() }

func (t *Text) SetClipboard(value Clipboard) {
	t.Element()
	t.editor.SetClipboard(value)
}

func (t *Text) Gutter() RowGutter { return t.editor.Gutter() }

func (t *Text) SetGutter(value RowGutter) {
	t.Element()
	t.editor.SetGutter(value)
}

func (t *Text) CursorStyle() grid.CursorStyle { return t.editor.CursorStyle() }

func (t *Text) SetCursorStyle(value grid.CursorStyle) {
	t.Element()
	t.editor.SetCursorStyle(value)
}
