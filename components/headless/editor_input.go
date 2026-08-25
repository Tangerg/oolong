package headless

import (
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// Handle answers keys, reporting whether it consumed the event.
//
// Enter is deliberately not bound. Whether it sends or breaks the line is the
// container's decision, and an editor that swallowed it would take that decision
// away from every container that embeds one.
func (e *Editor) Handle(ev input.Event) bool {
	if paste, ok := ev.(input.Paste); ok {
		e.endTyping()
		e.Insert(paste.Text)
		return true
	}
	if mouse, ok := ev.(input.Mouse); ok {
		// The geometry the editor last drew with, taken from its own committed
		// presentation. A press is aimed at what is on the screen, so that is the
		// only geometry it can be about — and an editor that has never been drawn
		// has none, which is how it declines a press it was never shown for.
		return e.handleMouse(mouse, e.presentation.Value())
	}
	key, ok := ev.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	e.ensure()

	// Shift turns any way of moving into a way of selecting. The chord is looked up
	// with the shift taken off it, and the anchor is dropped first and taken back if
	// what it named was not a movement after all — which is what keeps this to one
	// rule instead of a second binding for every direction.
	if key.Mods&input.Shift != 0 {
		unshifted := key.Chord()
		unshifted.Mods &^= input.Shift
		if action, bound := e.keys().Action(unshifted); bound {
			had := e.selecting
			e.Anchor()
			if e.move(action) {
				return true
			}
			e.selecting = had
		}
	}

	matched, handled := e.matcher.Handle(e.keys(), key, e.Do)
	if !matched {
		return e.typed(key)
	}
	return handled
}

// Do runs one of the field's actions by name, reporting whether it was one this field
// knows. See [Doer] for why a widget answers to a name at all.
func (e *Editor) Do(action keymap.Action) bool {
	e.ensure()
	if e.move(action) {
		// Moving is how a selection is let go of, which is what every other editor does
		// and what an arrow key means. Selecting is the same movement with the anchor
		// put down first — see [Editor.Handle].
		e.SelectNone()
		return true
	}
	switch action {
	case DeleteBack:
		if !e.DeleteSelection() {
			e.DeleteBack()
		}
	case DeleteForward:
		if !e.DeleteSelection() {
			e.DeleteForward()
		}
	case DeleteWordBack:
		e.DeleteWordBack()
	case KillToEnd:
		e.KillToEnd()
	case KillToStart:
		e.KillToStart()
	case Yank:
		e.Yank()
	case YankPop:
		e.YankPop()
	case InsertNewline:
		e.Newline()
	case Undo:
		e.Undo()
	case Redo:
		e.Redo()
	case SelectAll:
		e.SelectAll()
	case Copy:
		e.Copy()
	case Cut:
		e.Cut()
	case Paste:
		e.Paste()
	default:
		return false
	}
	return true
}

// typed puts a keystroke in as text, when it is text.
//
// Text, and only text. A chord this field has no use for belongs to whatever is around
// it, and swallowing it would break that.
func (e *Editor) typed(key input.Key) bool {
	if key.Mods&^input.Shift != 0 {
		return false
	}
	// What the terminal says the key produced wins over the key's own code. The code is
	// the unshifted key on the physical keyboard: on a layout where the key beside "1"
	// produces "@", inserting the code would type "2".
	if key.Text != "" {
		e.typeText(key.Text)
		return true
	}
	if key.Code == input.Character && key.Rune != 0 {
		e.InsertRune(key.Rune)
		return true
	}
	return false
}

// keys is the map to read through, standing in the default for a caller who set none.
func (e *Editor) keys() *keymap.Map {
	if e.Keys != nil {
		return e.Keys
	}
	return editorKeys()
}
