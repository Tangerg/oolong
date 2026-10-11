package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Text owns a one-line [Editor], validation and controlled-answer reconciliation.
// The zero value is ready. A Text must not be copied after first use. SetBinding
// changes answer ownership. Abnormal writes still settle against the accepted answer;
// failed reads defer settlement until the next semantic operation without suppressing
// the abnormal exit.
type Text struct {
	noCopy noCopy

	field
	Label string
	// Check says what is wrong with what has been entered, or nil. It is asked when the
	// keyboard leaves the field and when the form is submitted.
	Check       func(s string) error
	Placeholder string
	// Keys say which keystrokes edit. Nil reads through [DefaultEditorKeys].
	Keys *keymap.Map
	// Clipboard supplies copy, cut and paste services. Nil disables those actions.
	Clipboard Clipboard
	// Gutter optionally draws beside the single text row.
	Gutter RowGutter
	// CursorStyle chooses the focused terminal cursor's shape and blink.
	CursorStyle grid.CursorStyle

	editor Editor
	value  Accessor[string]
	// owner identifies the binding lifetime; only SetBinding advances it.
	owner *byte
	// operation revokes older semantic calls across accessor and clipboard callbacks.
	operation uint64
	// settling scopes an unfinished write to its owner. Sync cannot turn that
	// proposal into external adoption, but another binding can normalize immediately.
	settling *byte
	// pending owns settlement of one offered edit. A reentrant semantic operation
	// settles it against the accessor before advancing cursor, history or content.
	pending *textSettlement
}

// Text reads the accepted one-line value without synchronizing or writing its owner.
func (t *Text) Text() string {
	if binding := t.value; binding != nil {
		return oneLineText(binding.Value())
	}
	return t.editor.Text()
}

// SetBinding transfers answer ownership and abandons the previous owner's history,
// selection and validation. Nil takes the currently accepted answer into local state.
// The complete transfer precedes resolver cancellation; reentry cannot revive the old
// edit. Synchronization canonicalizes an external value at the next semantic operation.
// Required reads finish before the transfer. An abnormal read exit preserves the
// previous owner and its history.
func (t *Text) SetBinding(value Accessor[string]) {
	t.finishPending()
	var accepted string
	if value == nil {
		accepted = t.Text()
	} else {
		accepted = oneLineText(value.Value())
	}
	t.editor.matcher.Reset(func() {
		t.operation++
		t.value, t.owner = value, new(byte)
		t.editor.SetText(accepted)
		t.editor.SelectNone()
		t.editor.history.clear()
		t.invalidateValidation()
	})
}

// SetText replaces the answer through the same acceptance and history boundary as input.
func (t *Text) SetText(value string) {
	edit := t.beginEdit(t.beginOperation())
	if edit.operation != t.operation {
		return
	}
	t.editor.SetText(value)
	t.storeSince(edit)
}

// Cursor returns the byte column in the current one-line projection.
func (t *Text) Cursor() int { return t.editor.lineView(t.Text()).cursor }

// SetCursor synchronizes the answer, then moves to a grapheme boundary.
func (t *Text) SetCursor(column int) {
	if operation := t.beginOperation(); operation == t.operation {
		t.editor.SetCursor(0, column)
	}
}

// Revision reports the accepted editor content generation, after the last Sync or edit.
func (t *Text) Revision() uint64 { return t.editor.Revision() }

// SetMask chooses what is drawn in place of each input grapheme; empty removes
// masking.
// Invalid masks panic on the same terms as [Editor.SetMask].
func (t *Text) SetMask(mask string) { t.editor.SetMask(mask) }

// Mask returns the configured mask.
func (t *Text) Mask() string { return t.editor.Mask() }

// Prompt is what the field is asking for.
func (t *Text) Prompt() string { return t.Label }

// HeightForWidth is the label, a row of text, and the problem with it if there is one.
func (t *Text) HeightForWidth(int) int { return layout.Sum(1, t.rows(t.Label)) }

// Draw paints the label, the field and whatever was wrong with the answer.
func (t *Text) Draw(v Frame) {
	t.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (t *Text) DrawWith(v Frame, look Look) {
	view := t.editor.lineView(t.Text())
	view.placeholder, view.gutter, view.cursorStyle = t.Placeholder, t.Gutter, t.CursorStyle
	view.draw(t.frame(v, t.Label, look), look, &t.editor.presentation)
}

// Handle passes input to the field and keeps the value in step with it.
func (t *Text) Handle(ev input.Event) bool {
	operation := t.beginOperation()
	if operation != t.operation {
		return true
	}
	if key, ok := ev.(input.Key); ok {
		return t.editor.handleKey(key, t.Do, func(key input.Key) bool {
			edit := t.beginEdit(t.beginOperation())
			if edit.operation != t.operation {
				return true
			}
			handled := t.editor.typed(key)
			t.storeSince(edit)
			return handled
		})
	}
	if mouse, ok := ev.(input.Mouse); ok {
		ev = t.local(mouse)
	}
	edit := t.beginEdit(operation)
	handled := t.editor.Handle(ev)
	t.storeSince(edit)
	return handled
}

// Do runs one of the field's actions by name. A newer semantic operation started by
// Sync revokes this call before dispatch and returns false. See [Doer].
func (t *Text) Do(action keymap.Action) bool {
	edit := t.beginEdit(t.beginOperation())
	if edit.operation != t.operation {
		return false
	}
	if !t.editor.Do(action) {
		return false
	}
	t.storeSince(edit)
	return true
}

// Validate checks what has been entered.
func (t *Text) Validate() error {
	return t.validate(func() error {
		t.Sync()
		return nil
	}, t.checkAnswer)
}

func (t *Text) checkAnswer() fieldCheck {
	answer := t.Text()
	checked := fieldCheck{current: func() bool {
		return t.Text() == answer && (t.value == nil || t.value.Value() == answer)
	}}
	if !checked.current() {
		return fieldCheck{err: ErrInvalidBinding}
	}
	if t.Check != nil {
		checked.err = t.Check(answer)
	}
	return checked
}

// Focus takes the keyboard or gives it up, and checks the answer on the way out.
//
// On the way out and not on the way in: a form that greeted somebody with a column of
// complaints about answers they have not given yet would be a form nobody finishes. A
// field that has never had the keyboard has nothing to check.
func (t *Text) Focus(has bool) {
	t.focus(has, t.editor.Focus, func() { t.beginOperation() }, t.Validate)
}

// Sync makes the caller-owned value authoritative at semantic boundaries. Drawing
// projects that value without mutating the editor; input first reconciles the editor
// so cursor, history and the subsequent write all describe the same text.
func (t *Text) Sync() {
	binding := t.value
	t.editor.SetSingleLine(true)
	t.editor.Placeholder = t.Placeholder
	t.editor.Keys = t.Keys
	t.editor.Clipboard, t.editor.Gutter, t.editor.CursorStyle = t.Clipboard, t.Gutter, t.CursorStyle
	// An edit already with its owner has an owner: the operation that offered it,
	// which is the only one that knows what the answer coming back is an answer to.
	if t.value != nil && t.settling == t.owner {
		return
	}
	if binding == nil {
		return
	}
	t.finishPending()
	owned := binding.Value()
	value := oneLineText(owned)
	if value != owned {
		t.offer(value)
		value = t.Text()
	}
	if value != t.editor.Text() {
		t.adopt(value)
	}
}

// adopt replaces the editor with an owner-written or owner-accepted value.
// Reconciliation is a new owner state, not a user edit inside the old one. Keeping
// previous snapshots would let Undo overwrite an accessor with a value that stopped
// being current before this operation began.
func (t *Text) adopt(value string) {
	t.editor.SetText(value)
	t.editor.history.clear()
}

// store offers what has been typed to its owner and settles the same edit onto the
// value that owner accepted. A validator or normalizer must not leave the editor as a
// private shadow of the request it declined or changed, but its answer is not a new
// external state transition and therefore must not clear this edit's history.
func (t *Text) store(edit textEdit) {
	if t.value != nil {
		pending := &textSettlement{edit: edit, requested: t.editor.Text()}
		t.pending = pending
		// Abnormal exits still settle this proposal; reentry may already have done so.
		defer func() {
			if t.pending == pending {
				t.finishPending()
			}
		}()
		accepted := oneLineText(t.offer(pending.requested))
		if t.pending == pending {
			t.pending = nil
			t.settleEdit(pending, accepted)
		} else {
			// Reentry may also transfer the field to another accessor. Only its current
			// owner can supply a replacement after the old proposal lost settlement.
			t.Sync()
		}
	}
}

type textSettlement struct {
	edit      textEdit
	requested string
}

func (t *Text) finishPending() {
	if pending := t.pending; pending != nil {
		accepted := t.Text()
		t.pending = nil
		t.settleEdit(pending, accepted)
	}
}

func (t *Text) settleEdit(pending *textSettlement, accepted string) {
	switch {
	case accepted == pending.requested:
	case pending.edit.checkpointed && accepted == pending.edit.checkpoint.text:
		t.editor.rejectEdit(pending.edit.checkpoint)
	default:
		t.editor.reconcileEdit(accepted)
	}
}

// The binding owner may normalize a write; reconcile against its accepted value.
func (t *Text) offer(value string) string {
	previous := t.settling
	t.settling = t.owner
	defer func() { t.settling = previous }()
	return setAccessor(t.value, value)
}

type textEdit struct {
	operation    uint64
	revision     uint64
	checkpoint   editorCheckpoint
	checkpointed bool
}

func (t *Text) beginOperation() uint64 {
	t.finishPending()
	t.operation++
	operation := t.operation
	t.Sync()
	return operation
}

func (t *Text) beginEdit(operation uint64) textEdit {
	edit := textEdit{operation: operation}
	if edit.operation != t.operation {
		return edit
	}
	edit.revision = t.editor.Revision()
	// Bind is an exact assignment and therefore cannot reject or normalize. Every
	// open Accessor implementation gets a rollback checkpoint because its Set
	// postcondition, not its requested argument, decides whether an edit existed.
	if _, exact := t.value.(bound[string]); t.value != nil && !exact {
		edit.checkpoint, edit.checkpointed = t.editor.checkpointEdit(), true
	}
	return edit
}

// Consumed navigation and no-op edits are not assignments to the answer owner.
func (t *Text) storeSince(edit textEdit) {
	if edit.operation != t.operation {
		return
	}
	if t.editor.Revision() != edit.revision {
		t.store(edit)
	}
}

// Ask is the label, with the placeholder as the hint it already is.
func (t *Text) Ask() string {
	if t.Placeholder == "" {
		return t.Label
	}
	return t.Label + " (" + t.Placeholder + ")"
}

// Reply takes what was said as the whole of the answer.
func (t *Text) Reply(said string) error {
	return t.validate(func() error {
		t.SetText(said)
		return nil
	}, t.checkAnswer)
}
