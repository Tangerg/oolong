package headless

import (
	"sync"

	"github.com/Tangerg/oolong/core/input"
)

// The actions an [Editor] answers to.
//
// Nothing about a keystroke is here. What produces one of these is a [input.Keymap]'s
// business, which is what lets the same field be driven by a key, by a menu, or by a
// command typed by name — see [Editor.Do].
//
// There is deliberately no action for selecting in a direction. Shift with any way of
// moving is what selects, so every movement selects and none of them was taught to.
const (
	MoveLeft      input.Action = "move-left"
	MoveRight     input.Action = "move-right"
	MoveUp        input.Action = "move-up"
	MoveDown      input.Action = "move-down"
	MoveWordLeft  input.Action = "move-word-left"
	MoveWordRight input.Action = "move-word-right"
	MoveLineStart input.Action = "move-line-start"
	MoveLineEnd   input.Action = "move-line-end"

	DeleteBack     input.Action = "delete-back"
	DeleteForward  input.Action = "delete-forward"
	DeleteWordBack input.Action = "delete-word-back"
	KillToEnd      input.Action = "kill-to-end"
	KillToStart    input.Action = "kill-to-start"
	Yank           input.Action = "yank"
	InsertNewline  input.Action = "newline"
	Undo           input.Action = "undo"
	Redo           input.Action = "redo"

	SelectAll input.Action = "select-all"
	Copy      input.Action = "copy"
	Cut       input.Action = "cut"
	Paste     input.Action = "paste"
)

// The actions a [List] answers to. They are the list's own and not the editor's or a
// scroll's, because moving a selection, moving a cursor and moving a window are three
// things a reader can tell apart and would be dismayed to find bound together.
const (
	SelectPrev     input.Action = "select-prev"
	SelectNext     input.Action = "select-next"
	SelectPageUp   input.Action = "select-page-up"
	SelectPageDown input.Action = "select-page-down"
	SelectFirst    input.Action = "select-first"
	SelectLast     input.Action = "select-last"
)

// The actions a [Scroll] answers to.
const (
	ScrollUp       input.Action = "scroll-up"
	ScrollDown     input.Action = "scroll-down"
	ScrollPageUp   input.Action = "scroll-page-up"
	ScrollPageDown input.Action = "scroll-page-down"
	ScrollTop      input.Action = "scroll-top"
	ScrollBottom   input.Action = "scroll-bottom"
)

// The actions a [Completion] answers to, on top of the list movement inside it.
const (
	Accept  input.Action = "accept"
	Dismiss input.Action = "dismiss"
)

// Close is what dismisses the top layer of a [Stack].
const Close input.Action = "close"

// The actions a form and its fields answer to, on top of the list movement a choice
// uses and the editing a line of text uses.
const (
	// Toggle takes the choice under the cursor, or gives it back.
	Toggle input.Action = "toggle"
	// Submit finishes a [Form], and Cancel abandons it.
	Submit input.Action = "submit"
	Cancel input.Action = "cancel"
)

// The actions a [Container] answers to: which of its children has the keyboard.
const (
	FocusNext input.Action = "focus-next"
	FocusPrev input.Action = "focus-prev"
)

// DefaultEditorKeys are the keystrokes a terminal text field is expected to answer.
//
// The control chords are the ones a terminal has always had, because they are the ones
// a reader's fingers already know and the ones that still work when the terminal cannot
// report anything richer.
//
// Enter is deliberately unbound. Whether it sends or breaks the line is the interface's
// decision, and a field that took it would take that decision away from every interface
// that has one.
func DefaultEditorKeys() *input.Keymap {
	m := &input.Keymap{}
	m.Bind(MoveLeft, input.Chord{Code: input.Left})
	m.Bind(MoveRight, input.Chord{Code: input.Right})
	m.Bind(MoveUp, input.Chord{Code: input.Up})
	m.Bind(MoveDown, input.Chord{Code: input.Down})
	m.Bind(MoveWordLeft, input.Alt.Rune('b'))
	m.Bind(MoveWordRight, input.Alt.Rune('f'))
	m.Bind(MoveLineStart, input.Ctrl.Rune('a'))
	m.Bind(MoveLineEnd, input.Ctrl.Rune('e'))

	m.Bind(DeleteBack, input.Chord{Code: input.Backspace})
	m.Bind(DeleteForward, input.Chord{Code: input.Delete})
	// Ctrl+W is what a terminal has always sent for this, and Alt+Backspace is what
	// the keyboard in front of the user says. Both, in that order, so the hint row
	// shows the one that works everywhere.
	m.Bind(DeleteWordBack, input.Ctrl.Rune('w'))
	m.Bind(DeleteWordBack, input.Alt.With(input.Backspace))
	m.Bind(KillToEnd, input.Ctrl.Rune('k'))
	m.Bind(KillToStart, input.Ctrl.Rune('u'))
	m.Bind(Yank, input.Ctrl.Rune('y'))
	m.Bind(InsertNewline, input.Alt.With(input.Enter))
	m.Bind(Undo, input.Ctrl.Rune('_'))

	// The chords a terminal leaves alone. Ctrl+C is the interrupt, Ctrl+V is the
	// literal-next escape, and Ctrl+A is where readline has always put the start of the
	// line — taking any of them would break something every terminal user already
	// relies on.
	m.Bind(SelectAll, input.Alt.Rune('a'))
	m.Bind(Copy, input.Alt.Rune('c'))
	m.Bind(Cut, input.Alt.Rune('x'))
	m.Bind(Paste, input.Alt.Rune('v'))
	return m
}

// DefaultListKeys are the keystrokes a terminal list is expected to answer.
func DefaultListKeys() *input.Keymap {
	m := &input.Keymap{}
	m.Bind(SelectPrev, input.Chord{Code: input.Up})
	m.Bind(SelectNext, input.Chord{Code: input.Down})
	m.Bind(SelectPageUp, input.Chord{Code: input.PageUp})
	m.Bind(SelectPageDown, input.Chord{Code: input.PageDown})
	m.Bind(SelectFirst, input.Chord{Code: input.Home})
	m.Bind(SelectLast, input.Chord{Code: input.End})
	return m
}

// DefaultScrollKeys are the keystrokes a terminal reader is expected to answer.
func DefaultScrollKeys() *input.Keymap {
	m := &input.Keymap{}
	m.Bind(ScrollUp, input.Chord{Code: input.Up})
	m.Bind(ScrollDown, input.Chord{Code: input.Down})
	m.Bind(ScrollPageUp, input.Chord{Code: input.PageUp})
	m.Bind(ScrollPageDown, input.Chord{Code: input.PageDown})
	m.Bind(ScrollTop, input.Chord{Code: input.Home})
	m.Bind(ScrollBottom, input.Chord{Code: input.End})
	return m
}

// DefaultCompletionKeys are the keystrokes a terminal completion is expected to answer:
// the list's, because the candidates are a list, and the two of its own.
func DefaultCompletionKeys() *input.Keymap {
	m := DefaultListKeys()
	m.Bind(Accept, input.Chord{Code: input.Tab})
	m.Bind(Dismiss, input.Chord{Code: input.Esc})
	return m
}

// DefaultStackKeys are the keystrokes a layered interface is expected to answer.
func DefaultStackKeys() *input.Keymap {
	m := &input.Keymap{}
	m.Bind(Close, input.Chord{Code: input.Esc})
	return m
}

// DefaultContainerKeys are the keystrokes that walk the keyboard around an interface.
//
// Shift+Tab and not backtab: they are one keystroke, and [input] reports them as one —
// tab with shift held, whichever way the terminal spelled it.
func DefaultContainerKeys() *input.Keymap {
	m := &input.Keymap{}
	m.Bind(FocusNext, input.Chord{Code: input.Tab})
	m.Bind(FocusPrev, input.Shift.With(input.Tab))
	return m
}

// DefaultMultiSelectKeys are the keystrokes a list of choices answers: the movement any
// list has, and the one that takes what is under the cursor.
func DefaultMultiSelectKeys() *input.Keymap {
	m := DefaultListKeys()
	m.Bind(Toggle, input.Chord{Code: input.Character, Rune: ' '})
	return m
}

// DefaultConfirmKeys are the keystrokes a yes or no answers.
//
// Left and right rather than up and down, because the two answers are drawn side by
// side and a key that moved the other way would be pointing at nothing.
func DefaultConfirmKeys() *input.Keymap {
	m := &input.Keymap{}
	m.Bind(SelectPrev, input.Chord{Code: input.Left})
	m.Bind(SelectNext, input.Chord{Code: input.Right})
	m.Bind(Toggle, input.Chord{Code: input.Character, Rune: ' '})
	m.Bind(Toggle, input.Chord{Code: input.Tab})
	return m
}

// DefaultFormKeys are the keystrokes a form answers: the two that walk between its
// fields, and the two that finish with it.
//
// Enter submits rather than moving on, because a form is finished by saying so and a
// field that took enter to mean "next" would leave nothing to mean "done". A field with
// its own use for enter keeps it, since a field is asked first.
func DefaultFormKeys() *input.Keymap {
	m := DefaultContainerKeys()
	m.Bind(Submit, input.Chord{Code: input.Enter})
	m.Bind(Cancel, input.Chord{Code: input.Esc})
	return m
}

// The maps a widget reads through when its caller set none.
//
// One apiece for the whole program rather than one per widget: a default map is a
// table, it is the same table every time, and nothing can reach these to change one.
// The exported constructors above build a fresh map each call, which is what a caller
// adding bindings of their own needs.
//
// They are not written back into the widget's own field. A read path that changed an
// exported field would mean the zero value a caller set up survived only until the
// first keystroke, and a widget compared, copied or logged after that would not be the
// one they wrote.
var (
	editorKeys      = sync.OnceValue(DefaultEditorKeys)
	listKeys        = sync.OnceValue(DefaultListKeys)
	scrollKeys      = sync.OnceValue(DefaultScrollKeys)
	completionKeys  = sync.OnceValue(DefaultCompletionKeys)
	stackKeys       = sync.OnceValue(DefaultStackKeys)
	containerKeys   = sync.OnceValue(DefaultContainerKeys)
	multiSelectKeys = sync.OnceValue(DefaultMultiSelectKeys)
	confirmKeys     = sync.OnceValue(DefaultConfirmKeys)
	formKeys        = sync.OnceValue(DefaultFormKeys)
)
