package headless

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/text"
)

// Editor is a multi-line text field.
//
// The cursor is a byte offset into a line, and it only ever sits on a grapheme
// cluster boundary. It cannot sit between a letter and the accent that modifies it,
// because that is not a place a terminal could draw it, and it cannot sit inside a
// double-width character for the same reason.
//
// Vertical movement is by visual row, not by logical line. In a field that wraps,
// pressing down inside a long paragraph has to move down the screen; a cursor that
// jumped to the next paragraph instead would be moving somewhere the user cannot
// see the reason for.
type Editor struct {
	// Placeholder is shown while the field is empty, and is not part of the text.
	Placeholder      string
	Style            grid.Style
	PlaceholderStyle grid.Style
	// SelectionStyle is laid over the selected text. The zero value draws no
	// selection, which is what a field that never selects wants — and which is what
	// this had before there was one, when a selection could be made and not seen.
	SelectionStyle grid.Style
	// Keys are the bindings the editor answers.
	Keys EditorKeys
	// Clipboard is where copy and cut send text and where paste asks for it. Nil
	// leaves those keys doing nothing, which is the right answer for a field in a
	// program that has no terminal to ask.
	//
	// A program's loop already offers exactly this, so wiring a field to the system
	// clipboard is one assignment.
	Clipboard Clipboard
	// MaxRows caps how tall the field grows. Beyond it the field scrolls and keeps
	// the cursor in view. Zero means it grows without limit, which only suits a
	// field that owns its whole pane.
	MaxRows int

	lines []string
	// line is the cursor's logical line; col is its byte offset within that line.
	line, col int
	// wantColumn is the visual column vertical movement aims for, so a cursor moving
	// through short lines comes back out where it went in. Negative means it has not
	// been set and the cursor's own column is the aim.
	wantColumn int

	// anchor is where a selection began, and selecting says there is one. The far end
	// is the cursor, so a selection needs nothing kept in step with movement.
	anchor    Caret
	selecting bool

	// rowEnd is where a click landed that meant the end of a wrapped row rather than
	// the start of the next, and set says there was one.
	//
	// It is cleared in [Editor.endTyping], which every movement and every edit already
	// passes through, rather than in the forty-odd places the cursor is assigned — and
	// it is checked against the cursor's own position as well, so a stale one cannot
	// apply to a position it was never about.
	rowEnd    Caret
	rowEndSet bool

	// marks are the runs of text that behave as one character, in the order they
	// appear, as offsets into the whole text. nextElement is the last identity handed
	// out. See [Editor.offsetOf] for why they are not kept in lines and columns.
	marks       []text.Mark
	nextElement uint64

	// killed is the last text cut, for putting back. One entry, like a terminal's.
	killed string

	// blurred says the field has been told it does not have the keyboard, so it
	// draws no cursor. Inverted, because a field that has never been told anything
	// is the whole interface and does have it — see [Focusable].
	blurred bool

	undo, redo []editorState
	// typing marks a run of plain insertions, so undo steps over a phrase rather
	// than a letter.
	typing bool

	scroll Scroll
	layout editorLayout
}

// editorState is a whole snapshot for undo.
//
// Whole, not a patch: the text in a composer is small, the snapshots are few, and a
// patch that reconstructed the wrong state would be a bug nobody could see coming.
type editorState struct {
	lines     []string
	line, col int
	// marks are part of the state and not derivable from the text: two chips with
	// the same words are two different things, and an undo that gave them back with
	// new identities would have given back different ones.
	marks []text.Mark
}

// EditorKeys are the keystrokes an editor answers.
//
// The control chords are the ones a terminal has always had, because they are the
// ones a reader's fingers already know and the ones that work when the terminal
// cannot report anything richer.
type EditorKeys struct {
	Left, Right, Up, Down     Binding
	WordLeft, WordRight       Binding
	LineStart, LineEnd        Binding
	DeleteBack, DeleteForward Binding
	DeleteWordBack            Binding
	KillToEnd, KillToStart    Binding
	Yank                      Binding
	Newline                   Binding
	Undo                      Binding
	// SelectAll, Copy, Cut and PasteFrom work on a selection. There is deliberately
	// no binding for selecting in a direction: shift with any way of moving is what
	// selects, so every movement selects and none of them was taught to.
	SelectAll            Binding
	Copy, Cut, PasteFrom Binding
}

// DefaultEditorKeys are the bindings a terminal text field is expected to have.
func DefaultEditorKeys() EditorKeys {
	ctrl := func(r rune, does string) Binding {
		return Binding{Key: input.Key{Code: input.Character, Rune: r, Mods: input.Ctrl}, Does: does}
	}
	alt := func(r rune, does string) Binding {
		return Binding{Key: input.Key{Code: input.Character, Rune: r, Mods: input.Alt}, Does: does}
	}
	plain := func(code input.Code, does string) Binding {
		return Binding{Key: input.Key{Code: code}, Does: does}
	}
	return EditorKeys{
		Left:       plain(input.Left, "left"),
		Right:      plain(input.Right, "right"),
		Up:         plain(input.Up, "up"),
		Down:       plain(input.Down, "down"),
		WordLeft:   alt('b', "word left"),
		WordRight:  alt('f', "word right"),
		LineStart:  ctrl('a', "line start"),
		LineEnd:    ctrl('e', "line end"),
		DeleteBack: plain(input.Backspace, "delete"),
		// Alt+Backspace is what a terminal sends for deleting a word, and Ctrl+W is
		// what it has always sent.
		DeleteWordBack: ctrl('w', "delete word"),
		DeleteForward:  plain(input.Delete, "delete forward"),
		KillToEnd:      ctrl('k', "cut to end"),
		KillToStart:    ctrl('u', "cut to start"),
		Yank:           ctrl('y', "put back"),
		Newline:        Binding{Key: input.Key{Code: input.Enter, Mods: input.Alt}, Does: "newline"},
		Undo:           ctrl('_', "undo"),
		// The chords a terminal leaves alone. Ctrl+C is the interrupt, Ctrl+V is the
		// literal-next escape, and Ctrl+A is where readline has always put the start
		// of the line — taking any of them would break something every terminal user
		// already relies on.
		SelectAll: alt('a', "select all"),
		Copy:      alt('c', "copy"),
		Cut:       alt('x', "cut"),
		PasteFrom: alt('v', "paste"),
	}
}

// NewEditor returns an empty editor with the usual bindings.
func NewEditor() *Editor {
	return &Editor{lines: []string{""}, Keys: DefaultEditorKeys(), wantColumn: -1}
}

// Text is the whole content, lines joined by newlines.
func (e *Editor) Text() string {
	e.ensure()
	return strings.Join(e.lines, "\n")
}

// SetText replaces the content and puts the cursor at the end, which is where
// someone who just had text put in front of them wants to carry on from.
func (e *Editor) SetText(s string) {
	e.endTyping()
	e.snapshot()
	e.lines = strings.Split(s, "\n")
	e.line = len(e.lines) - 1
	e.col = len(e.lines[e.line])
	e.invalidate()
}

// Empty reports whether there is nothing in the field.
func (e *Editor) Empty() bool {
	e.ensure()
	return len(e.lines) == 1 && e.lines[0] == ""
}

// Clear empties the field.
func (e *Editor) Clear() {
	e.endTyping()
	e.snapshot()
	e.lines = []string{""}
	e.line, e.col = 0, 0
	e.invalidate()
}

// Cursor is the cursor's logical line and byte offset, for anything that needs to
// know where the user is.
func (e *Editor) Cursor() (line, col int) {
	e.ensure()
	return e.line, e.col
}

// SetCursor moves the cursor to a logical line and a byte offset within it.
//
// Both are clamped to the text, and the offset is pulled back to the start of the
// cluster it lands inside: a cursor between a letter and the accent that modifies
// it is not a place a terminal could draw one.
//
// It is what a caller needs to restore a draft where they left it, and what
// placing the cursor from a click will be built on — the editor could report where
// its caret was and not be told where to put it, which made the round trip only
// half a round.
func (e *Editor) SetCursor(line, col int) {
	e.ensure()
	e.endTyping()
	e.line = min(max(line, 0), len(e.lines)-1)
	current := e.lines[e.line]
	col = min(max(col, 0), len(current))
	// NextCluster from the preceding boundary lands on the boundary at or before
	// col, which is the only offset a caret can occupy.
	if col > 0 && col < len(current) {
		if at := text.PrevCluster(current, col); text.NextCluster(current, at) > col {
			col = at
		}
	}
	e.col = col
	e.wantColumn = -1
}

// Insert puts text in at the cursor. Newlines in it split lines, so a paste arrives
// as the text that was pasted rather than as a run of keystrokes.
func (e *Editor) Insert(s string) {
	if s == "" {
		return
	}
	e.ensure()
	if !e.typing {
		e.snapshot()
	}
	// Typing over a selection replaces it, and does so inside the same undo step: a
	// user who selected a word and typed another did one thing.
	e.dropSelection()
	e.splice(s)
}

// Replace swaps the byte range [start, end) of the line the cursor is on for s, and
// leaves the cursor after what was put in. The range is clamped to the line.
//
// It is one edit rather than a delete and an insert so that it is one step to undo:
// accepting a completion is one thing the user did, and taking it back should not
// take two. A token never spans lines, which is why the range does not either.
func (e *Editor) Replace(start, end int, s string) {
	e.ensure()
	line := e.lines[e.line]
	start = min(max(start, 0), len(line))
	end = min(max(end, start), len(line))
	e.endTyping()
	e.snapshot()
	e.removed(Caret{Line: e.line, Col: start}, Caret{Line: e.line, Col: end}, "")
	e.lines[e.line] = line[:start] + line[end:]
	e.col = start
	e.splice(s)
}

// splice puts text in at the cursor, assuming the undo step has been opened already.
func (e *Editor) splice(s string) {
	// The marks are moved before anything else is, because an edit is described
	// against the document as it was.
	at := e.offsetOf(Caret{Line: e.line, Col: e.col})
	e.edited(text.Edit{Start: at, End: at, Text: s})

	parts := strings.Split(s, "\n")
	current := e.lines[e.line]
	head, tail := current[:e.col], current[e.col:]

	if len(parts) == 1 {
		e.lines[e.line] = head + parts[0] + tail
		e.col += len(parts[0])
		e.invalidate()
		return
	}
	inserted := make([]string, len(parts))
	inserted[0] = head + parts[0]
	copy(inserted[1:], parts[1:])
	last := len(inserted) - 1
	col := len(inserted[last])
	inserted[last] += tail

	// The inner append extends a slice this function just built, which is the
	// pattern makezero warns about when the slice came from elsewhere. It did not:
	// inserted holds the replacement lines and the tail is being put after them.
	//nolint:makezero // inserted is local and fully written above.
	e.lines = append(e.lines[:e.line], append(inserted, e.lines[e.line+1:]...)...)
	e.line += last
	e.col = col
	e.invalidate()
}

// InsertRune puts one character in.
func (e *Editor) InsertRune(r rune) {
	// A control character has no width and no business in the text: it would be
	// dropped at the cell, leaving a cursor position with nothing under it.
	if r != '\t' && unicode.IsControl(r) {
		return
	}
	e.Insert(string(r))
	// The run stays open, so a phrase becomes one undo step rather than a letter's
	// worth each.
	e.typing = true
}

// Newline splits the line at the cursor.
func (e *Editor) Newline() {
	e.endTyping()
	e.snapshot()
	e.ensure()
	current := e.lines[e.line]
	head, tail := current[:e.col], current[e.col:]
	e.removed(Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: e.col}, "\n")
	e.lines = append(e.lines[:e.line], append([]string{head, tail}, e.lines[e.line+1:]...)...)
	e.line++
	e.col = 0
	e.invalidate()
}

// DeleteBack removes the cluster before the cursor, or joins this line to the one
// above when the cursor is at the start of a line.
func (e *Editor) DeleteBack() {
	e.ensure()
	if e.col > 0 {
		if !e.typing {
			e.snapshot()
		}
		at := text.PrevCluster(e.lines[e.line], e.col)
		// A backspace that took a letter off the end of an element would leave a
		// fragment that still looks like the thing and no longer is, so it takes all
		// of it.
		if el, inside := e.insideElement(e.line, at); inside {
			at = el.Start
		}
		e.removed(Caret{Line: e.line, Col: at}, Caret{Line: e.line, Col: e.col}, "")
		e.lines[e.line] = e.lines[e.line][:at] + e.lines[e.line][e.col:]
		e.col = at
		e.invalidate()
		// Corrections belong to the burst they correct: typing a word, fixing a letter
		// and carrying on is one thought and should be one undo step.
		e.typing = true
		return
	}
	if e.line == 0 {
		return
	}
	e.endTyping()
	e.snapshot()
	above := e.lines[e.line-1]
	e.col = len(above)
	e.removed(Caret{Line: e.line - 1, Col: len(above)}, Caret{Line: e.line, Col: 0}, "")
	e.lines[e.line-1] = above + e.lines[e.line]
	e.lines = append(e.lines[:e.line], e.lines[e.line+1:]...)
	e.line--
	e.invalidate()
}

// DeleteForward removes the cluster after the cursor, or joins the line below.
func (e *Editor) DeleteForward() {
	e.ensure()
	e.endTyping()
	current := e.lines[e.line]
	if e.col < len(current) {
		e.snapshot()
		at := text.NextCluster(current, e.col)
		// The whole element or none of it, for the same reason a backspace takes all
		// of one.
		if el, inside := e.ElementAt(e.line, e.col); inside {
			at = el.End
		}
		e.removed(Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: at}, "")
		e.lines[e.line] = current[:e.col] + current[at:]
		e.invalidate()
		return
	}
	if e.line == len(e.lines)-1 {
		return
	}
	e.snapshot()
	e.removed(Caret{Line: e.line, Col: len(current)}, Caret{Line: e.line + 1, Col: 0}, "")
	e.lines[e.line] = current + e.lines[e.line+1]
	e.lines = append(e.lines[:e.line+1], e.lines[e.line+2:]...)
	e.invalidate()
}

// DeleteWordBack removes from the cursor back to the start of the word behind it.
func (e *Editor) DeleteWordBack() {
	e.ensure()
	e.endTyping()
	if e.col == 0 {
		e.DeleteBack()
		return
	}
	e.snapshot()
	at := wordStart(e.lines[e.line], e.col)
	e.killed = e.lines[e.line][at:e.col]
	e.lines[e.line] = e.lines[e.line][:at] + e.lines[e.line][e.col:]
	e.col = at
	e.invalidate()
}

// KillToEnd cuts from the cursor to the end of the line, keeping what it cut.
//
// On an already-empty tail it takes the line break instead, which is what makes
// repeated presses swallow a paragraph rather than stop at the first line.
func (e *Editor) KillToEnd() {
	e.ensure()
	e.endTyping()
	e.snapshot()
	current := e.lines[e.line]
	if e.col < len(current) {
		e.killed = current[e.col:]
		e.removed(Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: len(current)}, "")
		e.lines[e.line] = current[:e.col]
		e.invalidate()
		return
	}
	if e.line < len(e.lines)-1 {
		e.killed = "\n"
		e.removed(Caret{Line: e.line, Col: len(current)}, Caret{Line: e.line + 1, Col: 0}, "")
		e.lines[e.line] = current + e.lines[e.line+1]
		e.lines = append(e.lines[:e.line+1], e.lines[e.line+2:]...)
	}
	e.invalidate()
}

// KillToStart cuts from the start of the line to the cursor.
func (e *Editor) KillToStart() {
	e.ensure()
	e.endTyping()
	if e.col == 0 {
		return
	}
	e.snapshot()
	e.killed = e.lines[e.line][:e.col]
	e.removed(Caret{Line: e.line, Col: 0}, Caret{Line: e.line, Col: e.col}, "")
	e.lines[e.line] = e.lines[e.line][e.col:]
	e.col = 0
	e.invalidate()
}

// Yank puts back the last text cut.
func (e *Editor) Yank() {
	if e.killed == "" {
		return
	}
	// Insert takes the snapshot, once, now that the run is closed.
	e.endTyping()
	e.Insert(e.killed)
}

// MoveLeft moves one cluster left, over a line break when there is nowhere else.
func (e *Editor) MoveLeft() {
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col > 0 {
		// Stepping over an element rather than into it: it is one thing on screen,
		// and a cursor inside it has no position a reader could account for.
		e.col = e.snapElement(e.line, text.PrevCluster(e.lines[e.line], e.col), false)
		return
	}
	if e.line > 0 {
		e.line--
		e.col = len(e.lines[e.line])
	}
}

// MoveRight moves one cluster right, over a line break when there is nowhere else.
func (e *Editor) MoveRight() {
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col < len(e.lines[e.line]) {
		e.col = e.snapElement(e.line, text.NextCluster(e.lines[e.line], e.col), true)
		return
	}
	if e.line < len(e.lines)-1 {
		e.line++
		e.col = 0
	}
}

// MoveWordLeft moves to the start of the word behind the cursor.
func (e *Editor) MoveWordLeft() {
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col == 0 {
		e.MoveLeft()
		return
	}
	e.col = wordStart(e.lines[e.line], e.col)
}

// MoveWordRight moves past the end of the word in front of the cursor.
func (e *Editor) MoveWordRight() {
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col == len(e.lines[e.line]) {
		e.MoveRight()
		return
	}
	e.col = wordEnd(e.lines[e.line], e.col)
}

// MoveLineStart moves to the start of the logical line.
func (e *Editor) MoveLineStart() {
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	e.col = 0
}

// MoveLineEnd moves to the end of the logical line.
func (e *Editor) MoveLineEnd() {
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	e.col = len(e.lines[e.line])
}

// Undo steps back to before the last change.
func (e *Editor) Undo() {
	if len(e.undo) == 0 {
		return
	}
	e.redo = append(e.redo, e.state())
	last := len(e.undo) - 1
	e.restore(e.undo[last])
	e.undo = e.undo[:last]
	e.typing = false
}

// Redo steps forward again.
func (e *Editor) Redo() {
	if len(e.redo) == 0 {
		return
	}
	e.undo = append(e.undo, e.state())
	last := len(e.redo) - 1
	e.restore(e.redo[last])
	e.redo = e.redo[:last]
	e.typing = false
}

// Handle answers keys, reporting whether it consumed the event.
//
// Enter is deliberately not bound. Whether it sends or breaks the line is the
// container's decision, and an editor that swallowed it would take that decision
// away from every container that embeds one.
func (e *Editor) Handle(ev input.Event) bool {
	if paste, ok := ev.(input.Paste); ok {
		e.typing = false
		e.Insert(paste.Text)
		return true
	}
	key, ok := ev.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	e.ensure()

	k := e.keys()

	// Shift turns any way of moving into a way of selecting. The anchor is dropped
	// first and taken back if the key was not a movement after all, which is what
	// keeps this to one rule instead of a second binding for every direction.
	if key.Mods&input.Shift != 0 {
		unshifted := key
		unshifted.Mods &^= input.Shift
		had := e.selecting
		e.Anchor()
		if e.move(k, unshifted) {
			return true
		}
		e.selecting = had
	}
	if e.selectionKey(k, ev) {
		return true
	}
	if e.move(k, ev) {
		// Moving without shift is how a selection is let go of, which is what every
		// other editor does and what an arrow key means.
		e.SelectNone()
		return true
	}

	switch {
	case k.DeleteBack.Matches(ev):
		if !e.DeleteSelection() {
			e.DeleteBack()
		}
	case k.DeleteForward.Matches(ev):
		if !e.DeleteSelection() {
			e.DeleteForward()
		}
	case k.DeleteWordBack.Matches(ev):
		e.DeleteWordBack()
	case k.KillToEnd.Matches(ev):
		e.KillToEnd()
	case k.KillToStart.Matches(ev):
		e.KillToStart()
	case k.Yank.Matches(ev):
		e.Yank()
	case k.Newline.Matches(ev):
		e.Newline()
	case k.Undo.Matches(ev):
		e.Undo()
	default:
		// Text, and only text. A chord this editor has no use for belongs to whatever
		// is around it, and swallowing it would break that.
		if key.Mods&^input.Shift != 0 {
			return false
		}
		// What the terminal says the key produced wins over the key's own code. The
		// code is the unshifted key on the physical keyboard: on a layout where the
		// key beside "1" produces "@", inserting the code would type "2".
		if key.Text != "" {
			e.Insert(key.Text)
			e.typing = true
			return true
		}
		if key.Code == input.Character && key.Rune != 0 {
			e.InsertRune(key.Rune)
			return true
		}
		return false
	}
	return true
}

// keys are the bindings to answer, standing in the defaults for a caller who left
// them unset, without recording the answer. See [List.keys].
func (e *Editor) keys() EditorKeys {
	if e.Keys == (EditorKeys{}) {
		return DefaultEditorKeys()
	}
	return e.Keys
}

// ensure makes the zero editor usable: one empty line, with a cursor in it. An
// editor that took text but answered no arrow keys would be the worse kind of
// broken — it would look like it worked.
func (e *Editor) ensure() {
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	e.line = min(max(e.line, 0), len(e.lines)-1)
	e.col = min(max(e.col, 0), len(e.lines[e.line]))
	if e.wantColumn == 0 {
		e.wantColumn = -1
	}
}

// invalidate marks the layout out of date and drops the column vertical movement
// was aiming for, because the text it was aiming into has changed.
//
// It says nothing about whether a run of typing is still open. Conflating the two is
// what made every keystroke take its own undo step: the flag that decides whether to
// snapshot was being cleared by the very operation that had just set it.
func (e *Editor) invalidate() {
	e.layout.stale = true
	e.wantColumn = -1
}

// endTyping closes a run of typing, so the next insertion starts an undo step of its
// own. Every movement and every structural change ends one.
// endTyping closes a run of insertions, and with it the one thing a click could have
// said about where the cursor belongs.
//
// Every movement and every edit already calls this — it is the point they all pass
// through — so the affinity is cleared in one place rather than in the forty-odd places
// the cursor is assigned. A bit that every one of those had to reset would be reset in
// thirty-nine of them.
func (e *Editor) endTyping() {
	e.typing = false
	e.rowEndSet = false
}

// snapshot records the state for undo, coalescing a run of typing into one step so
// that undo steps over a phrase rather than a letter.
func (e *Editor) snapshot() {
	e.ensure()
	e.undo = append(e.undo, e.state())
	e.redo = nil
	if len(e.undo) > maxUndo {
		e.undo = e.undo[len(e.undo)-maxUndo:]
	}
}

// maxUndo bounds the history. A composer is not a document editor, and an unbounded
// history in a long-lived process is a leak with a friendly name.
const maxUndo = 200

func (e *Editor) state() editorState {
	return editorState{
		lines: append([]string(nil), e.lines...),
		line:  e.line,
		col:   e.col,
		marks: append([]text.Mark(nil), e.marks...),
	}
}

func (e *Editor) restore(s editorState) {
	e.lines = append([]string(nil), s.lines...)
	e.marks = append([]text.Mark(nil), s.marks...)
	e.line, e.col = s.line, s.col
	e.selecting = false
	e.layout.stale = true
	e.wantColumn = -1
}

// wordStart is the offset of the start of the word before i: any run of
// non-word characters, then the word itself, the way a terminal has always done it.
func wordStart(s string, i int) int {
	at := i
	for at > 0 {
		prev := text.PrevCluster(s, at)
		if isWord(s[prev:at]) {
			break
		}
		at = prev
	}
	for at > 0 {
		prev := text.PrevCluster(s, at)
		if !isWord(s[prev:at]) {
			break
		}
		at = prev
	}
	return at
}

// wordEnd is the offset past the end of the word after i.
func wordEnd(s string, i int) int {
	at := i
	for at < len(s) {
		next := text.NextCluster(s, at)
		if isWord(s[at:next]) {
			break
		}
		at = next
	}
	for at < len(s) {
		next := text.NextCluster(s, at)
		if !isWord(s[at:next]) {
			break
		}
		at = next
	}
	return at
}

// isWord reports whether a cluster is part of a word. Letters, digits and the
// underscore, so that a word motion in code stops where a reader expects.
func isWord(cluster string) bool {
	r, _ := utf8.DecodeRuneInString(cluster)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
