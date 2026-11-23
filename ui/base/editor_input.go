package base

import (
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Handle answers keys, pastes and pointer selection, reporting whether it consumed
// the event. A left-button press starts a drag. A release ends the drag only when it
// names the left button or leaves the button unspecified, even outside the editor.
//
// Enter is deliberately not bound. Whether it sends or breaks the line is the
// container's decision, and an editor that swallowed it would take that decision
// away from every container that embeds one.
func (e *Editor) Handle(ev input.Event) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if paste, ok := ev.(input.Paste); ok {
		e.Insert(paste.Text)
		return true
	}
	if mouse, ok := ev.(input.Mouse); ok {
		// The geometry the editor last drew with, taken from its own committed
		// presentation. A press is aimed at what is on the screen, so that is the
		// only geometry it can be about — and an editor that has never been drawn
		// has none, which is how it declines a press it was never shown for.
		return e.handleMouse(mouse, e.geometry.read(&e.presentation))
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	return e.handleKey(key, e.Do, e.typed)
}

// handleKey leaves action and insertion ownership with the controller. Deferred
// keymap resolution must re-enter that same controller's acceptance boundary.
func (e *Editor) handleKey(key input.Key, do func(keymap.Action) bool, typed func(input.Key) bool) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if !key.Down() {
		return false
	}
	e.ensure()

	matched, handled := e.matcher.Handle(e.resolvedKeys(), key, keyboardReceipt(&e.element), do)
	if !matched {
		return typed(key)
	}
	return handled
}

func (e *Editor) Do(action keymap.Action) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	for _, movement := range editorMovements {
		switch action {
		case movement.extend:
			e.Anchor()
			movement.run(e)
			return true
		case movement.move:
			movement.run(e)
			e.SelectNone()
			return true
		}
	}
	do, ok := editorActions[action]
	if !ok {
		return false
	}
	do(e)
	return true
}

func (e *Editor) typed(key input.Key) bool {
	typed := key.Typed()
	if typed == "" {
		return false
	}
	e.insert(text.Printable(typed), true)
	return true
}

func (e *Editor) resolvedKeys() *keymap.Map { return e.keys.resolve(editorKeys) }

// KeysFor returns the sequences bound to action in the map this editor handles
// keys with, including its defaults when no map is set, so help text can never
// describe bindings the editor does not use.
func (e *Editor) KeysFor(action keymap.Action) []input.Keys { return e.keys.bound(editorKeys, action) }

// Losing the keyboard ends transient input. Cursor painting reads scene focus directly.
func (e *Editor) focusChanged(has bool) {
	if !has {
		e.endTyping()
		e.matcher.Clear()
	}
}

var editorActions = map[keymap.Action]func(*Editor){
	DeleteBack: func(e *Editor) {
		if !e.DeleteSelection() {
			e.DeleteBack()
		}
	},
	DeleteForward: func(e *Editor) {
		if !e.DeleteSelection() {
			e.DeleteForward()
		}
	},
	DeleteWordBack: (*Editor).DeleteWordBack,
	KillToEnd:      (*Editor).KillToEnd,
	KillToStart:    (*Editor).KillToStart,
	Yank:           (*Editor).Yank,
	YankPop:        (*Editor).YankPop,
	InsertNewline:  (*Editor).Newline,
	Undo:           (*Editor).Undo,
	Redo:           (*Editor).Redo,
	SelectAll:      (*Editor).SelectAll,
	Copy:           func(e *Editor) { e.recordClipboard(e.Copy()) },
	Cut:            func(e *Editor) { e.recordClipboard(e.Cut()) },
	Paste:          (*Editor).pasteFromInput,
}
