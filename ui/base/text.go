package base

import (
	"context"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/setting"
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

// TextConfig seeds one answer. Later changes go through the returned owner.
type TextConfig struct {
	Key         string
	Initial     string
	Label       string
	Check       func(string) error
	Placeholder string
	// Mask presents every character as itself, for a password; empty shows the text.
	Mask        string
	Keys        *keymap.Map
	Clipboard   Clipboard
	Gutter      RowGutter
	CursorStyle grid.CursorStyle
}

// NewText panics for an unknown cursor shape or an invisible or joining Mask;
// one-line policy belongs to its edit session.
func NewText(config TextConfig) *Text {
	t := &Text{label: strings.Clone(config.Label), check: config.Check}
	t.editor.placeholder = strings.Clone(config.Placeholder)
	t.editor.keys.current = config.Keys
	t.editor.clipboard, t.editor.gutter = config.Clipboard, config.Gutter
	if err := config.CursorStyle.Shape.Validate(); err != nil {
		panic(err)
	}
	t.editor.cursorStyle = config.CursorStyle
	t.key = strings.Clone(config.Key)
	t.editor.SetMask(config.Mask)
	t.line().SetText(config.Initial)
	t.editor.ForgetHistory()
	return t
}

func (t *Text) Text() string { return t.editor.Text() }

// SetText replaces the owned answer and records one document edit. It panics during presentation or observation.
func (t *Text) SetText(value string) { t.line().SetText(value) }

// Cursor returns the caret's byte offset in the answer.
func (t *Text) Cursor() int { return t.editor.Cursor().Byte }

// SetCursor moves the view caret to a valid boundary. It does not change document revision.
func (t *Text) SetCursor(byteOffset int) { t.editor.SetCursor(Caret{Byte: byteOffset}) }

// Revision is the answer's document revision, independent of caret and scroll state.
func (t *Text) Revision() uint64 { return t.editor.Revision() }

// SetMask selects password presentation. It panics for an invisible or joining mask, or during presentation or observation.
func (t *Text) SetMask(mask string) { t.editor.SetMask(mask) }

// Mask returns the presentation glyph, or an empty string for visible text.
func (t *Text) Mask() string { return t.editor.Mask() }

func (t *Text) HeightForWidth(int) int { return layout.Sum(1, t.rows(t.label, t.ClipboardError())) }

func (t *Text) Draw(v scene.Frame) { t.DrawWith(v, Look{}) }

func (t *Text) DrawWith(v scene.Frame, look Look) {
	view := t.editor.lineView()
	view.draw(t.frame(v, t.label, look, t.ClipboardError()), look, &t.editor.presentation)
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

// ClipboardError exposes keyboard clipboard feedback from the field's existing
// editor. Programmatic clipboard failures go to their own return or completion
// receiver.
func (t *Text) ClipboardError() error { return t.editor.ClipboardError() }

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
	setting.Assign(t.element, &t.label, strings.Clone(value))
}

func (t *Text) Placeholder() string { return t.editor.Placeholder() }

func (t *Text) SetPlaceholder(value string) { t.editor.SetPlaceholder(value) }

func (t *Text) Keys() *keymap.Map { return t.editor.Keys() }

// KeysFor is what the effective key map binds to action, defaults included.
func (t *Text) KeysFor(action keymap.Action) []input.Keys { return t.editor.KeysFor(action) }

func (t *Text) SetKeys(value *keymap.Map) { t.editor.SetKeys(value) }

func (t *Text) Clipboard() Clipboard { return t.editor.Clipboard() }

func (t *Text) SetClipboard(value Clipboard) { t.editor.SetClipboard(value) }

func (t *Text) Gutter() RowGutter { return t.editor.Gutter() }

func (t *Text) SetGutter(value RowGutter) { t.editor.SetGutter(value) }

func (t *Text) CursorStyle() grid.CursorStyle { return t.editor.CursorStyle() }

func (t *Text) SetCursorStyle(value grid.CursorStyle) { t.editor.SetCursorStyle(value) }

func (t *Text) Element() scene.Element {
	if t.element.IsZero() {
		t.line()
		t.element = scene.NewElement(scene.ElementConfig{
			Key: t.key, Measure: layout.MeasureRows(t.HeightForWidth), Paint: t.Draw,
			Handle: func(e scene.Event) scene.EventResult { return partEvent(e, t.Handle, &t.geometry, &t.editor.geometry) }, Focusable: func() bool { return true }, FocusChanged: t.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleTextBox, Name: t.label, Value: t.editor.semanticValue(), Sensitive: t.Mask() != "", ContentRevision: t.Revision()}, t.feedbackError(t.ClipboardError()))
			}, Actions: []scene.ActionBinding{t.editor.fillAction()},
		})
	}
	t.editor.element = t.element
	t.editor.scroll.element = t.element
	t.editor.keys.attach(t.element, &t.editor.matcher)
	return t.element
}
