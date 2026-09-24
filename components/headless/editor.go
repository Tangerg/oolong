package headless

import (
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Editor is a multi-line text field.
//
// The cursor is a byte offset into a line, and it only ever sits on a grapheme
// cluster boundary. It cannot sit between a letter and the accent that modifies it,
// because that is not a place a terminal could draw it, and it cannot sit inside a
// multi-column display atom for the same reason.
//
// Vertical movement is by visual row, not by logical line. In a field that wraps,
// pressing down inside a long paragraph has to move down the screen; a cursor that
// jumped to the next paragraph instead would be moving somewhere the user cannot
// see the reason for.
//
// The zero value is ready to use. An Editor must not be copied after first use.
type Editor struct {
	noCopy noCopy

	// Placeholder is shown while the field is empty, and is not part of the text.
	Placeholder string
	// Look is how the text, the placeholder and the selection are drawn — see [Look],
	// which is the one way anything here that draws itself is dressed. The zero value
	// draws in the terminal's own colours and lays nothing over a selection, which is
	// what a field that never selects wants.
	Look Look
	// Keys say which keystrokes produce which of the actions this field answers to —
	// see [Editor.Do]. Nil reads through [DefaultEditorKeys].
	//
	// It is a map and not a struct of one field per action, so a program can hand the
	// same map to a field, to the container around it and to its own keys, and rebind
	// any of them without replacing anything.
	Keys *keymap.Map
	// Clipboard is where copy and cut send text and where paste asks for it. Nil
	// leaves those keys doing nothing, which is the right answer for a field in a
	// program that has no terminal to ask.
	//
	// A runtime adapter can satisfy this directly; an editor depends only on these
	// two operations.
	Clipboard Clipboard
	// MaxRows caps how tall the field grows. Beyond it the field scrolls and keeps
	// the cursor in view. Zero means it grows without limit, which only suits a
	// field that owns its whole pane.
	MaxRows int
	// Gutter draws beside the field's visual rows. Nil gives every column to the
	// text. The gutter is not part of selection or clipboard content, and pointer
	// input in it is left for a containing component to interpret.
	Gutter RowGutter
	// CursorStyle chooses the terminal cursor's shape and blink while this editor has
	// the keyboard. The zero value leaves both to the terminal's configured default.
	CursorStyle grid.CursorStyle

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
	dragging  bool

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
	// appear, as offsets into the whole text. elementIDs owns their stable identities.
	// See [Editor.offsetOf] for why they are not kept in lines and columns.
	marks      []text.Mark
	elementIDs identitySequence

	// kills owns bounded cut history. continuation says whether another kill may join
	// the newest entry or a yank-pop may replace the immediately preceding yank.
	kills        editorKillRing
	continuation editorContinuation
	yank         editorYank

	// matcher owns how far into a multi-chord binding the keys have got. It is the
	// field's own and not the map's — see [keymap.Matcher].
	matcher keymap.Matcher

	// blurred says the field has been told it does not have the keyboard, so it
	// draws no cursor. Inverted, because a field that has never been told anything
	// is the whole interface and does have it — see [Focusable].
	blurred bool

	history editorHistory
	// typing marks a run of plain insertions, so undo steps over a phrase rather
	// than a letter.
	typing bool
	// revision is the semantic content generation. Cursor, selection and presentation
	// state deliberately live outside it.
	revision uint64
	// singleLine and mask are private because changing either can change the semantic
	// one-line invariant. Their setters perform that transition; public fields would
	// let existing multi-line content become invisible without changing the model.
	singleLine bool
	mask       string

	scroll Scroll
	// cursorReveal identifies the pending navigation request. Scroll owns its
	// consumption and cancellation; drawing only resolves its row at the new width.
	cursorReveal *scrollRange
	layout       editorLayout
	// presentation is the committed wrap width and viewport origin used by pointer
	// and vertical cursor routing.
	presentation Snapshot[editorPresentation]
}

type editorPresentation struct {
	width, gutter, first, left int
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

// editorCheckpoint is the part of an edit that a controlled owner may reject after
// the editor has applied it. It owns its slice headers so history mutation during the
// attempted operation cannot erase the state needed to settle that rejection.
type editorCheckpoint struct {
	text      string
	state     editorState
	history   editorHistory
	anchor    Caret
	selecting bool
	revision  uint64
}

// Text is the whole content, lines joined by newlines.
func (e *Editor) Text() string {
	e.ensure()
	return strings.Join(e.lines, "\n")
}

// Revision reports the generation of the editor's semantic content.
//
// It advances once for every change to text or atomic elements, whether the change
// came from input, a programmatic editing method, undo or redo. Cursor movement,
// selection, scrolling, focus, copying and an edit that has no effect leave it alone.
// This is what lets a caller decide whether to validate, persist or mark a draft dirty
// without guessing from a key or an action name.
//
// A revision is an opaque, process-local observation token. Compare it with an earlier
// value from this editor; do not persist it or give the number itself meaning.
func (e *Editor) Revision() uint64 { return e.revision }

// SingleLine reports whether this field was explicitly configured to hold one line.
// A non-empty [Editor.Mask] also makes the effective field one-line.
func (e *Editor) SingleLine() bool { return e.singleLine }

// SetSingleLine changes whether the field holds one line.
//
// Enabling it turns existing line breaks into spaces as one semantic change, keeps
// element identities, settles the cursor from the same whole-document offset, and
// clears undo history that could otherwise restore an invalid multi-line state.
// Disabling it leaves the current one-line content in place; later insertions may add
// lines.
func (e *Editor) SetSingleLine(enabled bool) {
	if e.singleLine == enabled {
		return
	}
	wasOneLine := e.oneLine()
	e.singleLine = enabled
	e.oneLineChanged(wasOneLine)
}

// Mask reports what each text cluster is drawn as. Empty means text is shown.
func (e *Editor) Mask() string { return e.mask }

// SetMask changes what each text cluster is drawn as.
//
// A mask must be visible terminal text without tabs or control characters, and must
// still be itself when written twice in a row — see [maskable]; invalid
// configuration panics. A non-empty mask makes the field one-line, applying
// the same semantic transition as [Editor.SetSingleLine].
func (e *Editor) SetMask(mask string) {
	if mask != "" && !maskable(mask) {
		panic("headless: editor mask must be visible terminal text that does not join a copy of itself")
	}
	if e.mask == mask {
		return
	}
	wasOneLine := e.oneLine()
	e.mask = strings.Clone(mask)
	e.oneLineChanged(wasOneLine)
}

// maskable is what a mask has to be for the projection that consumes it: visible
// terminal text, wide enough to see, and still itself when the next copy is written
// beside it.
//
// The last condition is the one a mask judged on its own misses. A field draws one
// mask per cluster of text and finds a column by multiplying, so a mask that joins
// to a copy of itself — two regional indicators are a flag — draws fewer clusters
// than the text has characters and leaves the cursor at a column the row does not
// have.
func maskable(mask string) bool {
	return text.Printable(mask) == mask &&
		!strings.Contains(mask, "\t") &&
		text.Width(mask) > 0 &&
		clusters(mask+mask) == 2*clusters(mask)
}

func clusters(s string) int {
	n := 0
	for range text.Clusters(s) {
		n++
	}
	return n
}

// oneLineChanged maintains the storage invariant after a mode transition.
func (e *Editor) oneLineChanged(wasOneLine bool) {
	defer e.revealCursor()
	nowOneLine := e.oneLine()
	if wasOneLine == nowOneLine {
		return
	}
	e.endTyping()
	e.layout.stale = true
	if !nowOneLine {
		return
	}
	// Even when the current value already has one line, an older undo snapshot may
	// not. Configuration is not user history, so entering the stricter mode settles
	// the old history rather than letting Undo violate the new storage invariant.
	e.history.clear()
	if len(e.lines) <= 1 {
		return
	}
	e.requireContentRevision()
	at := e.offsetOf(Caret{Line: e.line, Col: e.col})
	e.lines = []string{strings.Join(e.lines, " ")}
	e.line, e.col = 0, e.snapElement(0, at, true)
	e.selecting = false
	e.contentChanged()
}

// SetText replaces the content and puts the cursor at the end, which is where
// someone who just had text put in front of them wants to carry on from.
func (e *Editor) SetText(s string) {
	e.ensure()
	e.endTyping()
	start := Caret{}
	end := Caret{Line: len(e.lines) - 1, Col: len(e.lines[len(e.lines)-1])}
	s, changed := e.prepareReplacement(start, end, s)
	if !changed {
		e.finishReplacement(end)
		return
	}
	e.snapshot()
	e.replaceRange(start, end, s)
}

// reconcileEdit replaces the representation accepted for the content mutation that
// just completed. It is still that mutation: revision and history have already been
// advanced by the editing operation and must not acquire a second step merely because
// a controlled owner normalized its result.
//
// Interaction positions are translated through the changed interval. Text before and
// after that interval keeps its exact position; a position inside it keeps its byte
// distance from the start as far as the accepted interval permits, then snaps to a
// valid grapheme and element boundary.
func (e *Editor) reconcileEdit(s string) {
	e.ensure()
	s = e.canonicalText(s)
	requested := e.Text()
	if requested == s {
		return
	}

	// Accepted text owns edit coordinates; normalization ends kill/yank continuation.
	e.breakContinuation()
	cursor := reconciledOffset(requested, s, e.offsetOf(Caret{Line: e.line, Col: e.col}))
	anchor := reconciledOffset(requested, s, e.offsetOf(e.anchor))
	rowEnd := reconciledOffset(requested, s, e.offsetOf(e.rowEnd))
	selecting, rowEndSet := e.selecting, e.rowEndSet

	start := Caret{}
	end := Caret{Line: len(e.lines) - 1, Col: len(e.lines[len(e.lines)-1])}
	e.removed(start, end, s)
	e.lines = strings.Split(s, "\n")
	for i := range e.lines {
		e.lines[i] = strings.Clone(e.lines[i])
	}
	e.settleMarks()

	caret := e.caretAt(cursor)
	e.line, e.col = caret.Line, e.snapElement(caret.Line, caret.Col, true)
	e.anchor = e.caretAt(anchor)
	e.anchor.Col = e.snapElement(e.anchor.Line, e.anchor.Col, false)
	e.rowEnd = e.caretAt(rowEnd)
	e.rowEnd.Col = e.snapElement(e.rowEnd.Line, e.rowEnd.Col, false)
	e.selecting, e.rowEndSet = selecting, rowEndSet
	e.wantColumn = -1
	e.layout.stale = true
}

func (e *Editor) checkpointEdit() editorCheckpoint {
	return editorCheckpoint{
		text: e.Text(), state: e.state(),
		history: editorHistory{
			undo: slices.Clone(e.history.undo),
			redo: slices.Clone(e.history.redo),
		},
		anchor: e.anchor, selecting: e.selecting, revision: e.revision,
	}
}

// rejectEdit restores the semantic state and history from before a controlled edit.
// The rejected action still ends typing and interaction affinity, just like any other
// handled edit with no effect; otherwise the next insertion could coalesce without a
// snapshot of its own.
func (e *Editor) rejectEdit(checkpoint editorCheckpoint) {
	e.lines, e.marks = checkpoint.state.lines, checkpoint.state.marks
	e.line, e.col = checkpoint.state.line, checkpoint.state.col
	e.anchor, e.selecting = checkpoint.anchor, checkpoint.selecting
	e.history, e.revision = checkpoint.history, checkpoint.revision
	e.endTyping()
	e.wantColumn = -1
	e.layout.stale = true
}

// reconciledOffset maps one position through the smallest single replacement that
// turns before into after. Equal-rune prefix and suffix boundaries keep matching
// valid for UTF-8 whose leading bytes happen to agree even when the runes do not. A
// byte distance preserved inside the changed interval can still land inside a
// multi-byte rune, so this mapping owns the UTF-8 boundary. Its consumers convert the
// result back to a line position and apply their directional grapheme and element
// policy there, without rescanning the whole document here.
func reconciledOffset(before, after string, at int) int {
	at = min(max(at, 0), len(before))
	prefix := commonRunePrefix(before, after)
	beforeEnd, afterEnd := commonRuneSuffixStarts(before, after, prefix)
	var mapped int
	switch {
	case at < prefix:
		mapped = at
	case at == prefix:
		if beforeEnd == prefix {
			mapped = afterEnd
		} else {
			mapped = prefix
		}
	case at >= beforeEnd:
		mapped = afterEnd + at - beforeEnd
	default:
		mapped = prefix + min(at-prefix, afterEnd-prefix)
	}
	for mapped > 0 && mapped < len(after) && !utf8.RuneStart(after[mapped]) {
		mapped--
	}
	return mapped
}

func commonRunePrefix(a, b string) int {
	at := 0
	for at < len(a) && at < len(b) {
		ra, sa := utf8.DecodeRuneInString(a[at:])
		rb, sb := utf8.DecodeRuneInString(b[at:])
		if ra != rb || sa != sb {
			break
		}
		at += sa
	}
	return at
}

func commonRuneSuffixStarts(a, b string, prefix int) (int, int) {
	aEnd, bEnd := len(a), len(b)
	for aEnd > prefix && bEnd > prefix {
		ra, sa := utf8.DecodeLastRuneInString(a[:aEnd])
		rb, sb := utf8.DecodeLastRuneInString(b[:bEnd])
		if ra != rb || sa != sb || aEnd-sa < prefix || bEnd-sb < prefix {
			break
		}
		aEnd, bEnd = aEnd-sa, bEnd-sb
	}
	return aEnd, bEnd
}

// Empty reports whether there is nothing in the field.
func (e *Editor) Empty() bool {
	e.ensure()
	return len(e.lines) == 1 && e.lines[0] == ""
}

// Clear empties the field.
func (e *Editor) Clear() {
	e.ensure()
	e.endTyping()
	start := Caret{}
	end := Caret{Line: len(e.lines) - 1, Col: len(e.lines[len(e.lines)-1])}
	_, changed := e.prepareReplacement(start, end, "")
	if !changed {
		e.finishReplacement(start)
		return
	}
	e.snapshot()
	e.replaceRange(start, end, "")
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
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.line = min(max(line, 0), len(e.lines)-1)
	current := e.lines[e.line]
	col = min(max(col, 0), len(current))
	e.col = e.snapElement(e.line, col, false)
	e.wantColumn = -1
}

// Insert puts text in at the cursor. Newlines in it split lines, so a paste arrives
// as the text that was pasted rather than as a run of keystrokes.
func (e *Editor) Insert(s string) {
	if s == "" {
		return
	}
	e.ensure()
	e.breakContinuation()
	start, end := Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: e.col}
	if selected, selectedEnd, ok := e.Selection(); ok {
		start, end = selected, selectedEnd
	}
	s, changed := e.prepareReplacement(start, end, s)
	if !changed {
		e.finishReplacement(end)
		return
	}
	if !e.typing {
		e.snapshot()
	}
	// Typing over a selection replaces it, and does so inside the same undo step: a
	// user who selected a word and typed another did one thing.
	e.replaceRange(start, end, s)
}

// Replace swaps the byte range [start, end) of the line the cursor is on for s, and
// leaves the cursor after what was put in. The range is clamped to the line and
// expanded to whole grapheme clusters; a terminal cursor cannot address half of one.
//
// It is one edit rather than a delete and an insert so that it is one step to undo:
// accepting a completion is one thing the user did, and taking it back should not
// take two. A token never spans lines, which is why the range does not either.
func (e *Editor) Replace(start, end int, s string) {
	e.ensure()
	line := e.lines[e.line]
	start = min(max(start, 0), len(line))
	end = min(max(end, start), len(line))
	start, end = completeClusters(line, start, end)
	e.endTyping()
	from := Caret{Line: e.line, Col: start}
	to := Caret{Line: e.line, Col: end}
	s, changed := e.prepareReplacement(from, to, s)
	if !changed {
		e.finishReplacement(to)
		return
	}
	e.snapshot()
	e.replaceRange(from, to, s)
}

// completeClusters makes a caller-supplied byte range safe for an editor whose
// cursor and cells speak in grapheme clusters. An insertion inside a cluster lands
// before it; a non-empty range expands to cover every cluster it touched. Cutting a
// rune or an emoji in half would leave invalid UTF-8 or a cursor position no terminal
// cell can represent.
func completeClusters(line string, start, end int) (int, int) {
	empty := start == end
	for at, cluster := range text.Clusters(line) {
		after := at + len(cluster)
		if start > at && start < after {
			start = at
		}
		if end > at && end < after {
			end = after
		}
	}
	if empty {
		end = start
	}
	return start, end
}

// canonicalText is the one boundary between caller text and editor storage.
//
// Line endings become the editor's one line separator, invalid UTF-8 becomes
// replacement text, and terminal controls other than tabs are removed. A field that
// holds one line turns separators into spaces instead. Applying these rules here
// means typing, paste, SetText and Replace cannot build four subtly different kinds
// of document. InsertElement uses the sibling oneLineText boundary because an atomic
// element is one run of cells even in a multi-line editor.
func (e *Editor) canonicalText(s string) string {
	if e.oneLine() {
		return oneLineText(s)
	}
	if strings.Contains(s, "\r") {
		s = logicalLineBreaks.Replace(s)
	}
	if !strings.Contains(s, "\n") {
		return text.Printable(s)
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = text.Printable(lines[i])
	}
	return strings.Join(lines, "\n")
}

// oneLineText is the one storage boundary shared by one-line fields and atomic
// elements. Keeping it separate from Editor state lets a pure render projection use
// the same policy without constructing or mutating an Editor merely to sanitize text.
func oneLineText(s string) string { return text.Printable(flattenLines(s)) }

// flattenLines is the canonical projection of text onto one logical line. Single-line
// editors apply it to every replacement; atomic elements apply it to their body even
// in a multi-line editor because one element is one contiguous run of cells.
func flattenLines(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	return flatLineBreaks.Replace(s)
}

// Replacers are immutable after construction and safe for concurrent use. Keeping
// the two storage policies here avoids rebuilding their search automata on every
// keystroke while leaving the distinction between logical and flattened breaks
// explicit.
var (
	logicalLineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")
	flatLineBreaks    = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")
)

func (e *Editor) oneLine() bool { return e.singleLine || e.mask != "" }

// InsertRune puts one character in.
func (e *Editor) InsertRune(r rune) {
	// A rune action is not the line-breaking action. Printable keeps a tab and drops
	// controls; canonicalText then owns the same storage rule as every other way text
	// enters instead of this method maintaining another character classifier.
	e.typeText(text.Printable(string(r)))
}

// typeText inserts terminal-produced text and keeps a real insertion open as one undo
// run. A handled no-op must close the run: without a snapshot of its own, allowing the
// next insertion to coalesce with it would leave that insertion no state to undo to.
func (e *Editor) typeText(s string) {
	before := e.revision
	e.Insert(s)
	e.typing = e.revision != before
}

// Newline splits the line at the cursor, and does nothing at all in a field that holds
// one line.
func (e *Editor) Newline() {
	e.endTyping()
	if e.oneLine() {
		return
	}
	e.Insert("\n")
}

// DeleteBack removes the cluster before the cursor, or joins this line to the one
// above when the cursor is at the start of a line.
func (e *Editor) DeleteBack() {
	e.ensure()
	e.breakContinuation()
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
		e.replaceRange(Caret{Line: e.line, Col: at}, Caret{Line: e.line, Col: e.col}, "")
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
	e.replaceRange(Caret{Line: e.line - 1, Col: len(above)}, Caret{Line: e.line, Col: 0}, "")
}

// DeleteForward removes the cluster after the cursor, or joins the line below.
func (e *Editor) DeleteForward() {
	e.ensure()
	e.endTyping()
	current := e.lines[e.line]
	if e.col < len(current) {
		e.snapshot()
		at := nextClusterBoundary(current, e.col)
		// The whole element or none of it, for the same reason a backspace takes all
		// of one.
		if el, inside := e.ElementAt(e.line, e.col); inside {
			at = el.End
		}
		e.replaceRange(Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: at}, "")
		return
	}
	if e.line == len(e.lines)-1 {
		return
	}
	e.snapshot()
	e.replaceRange(Caret{Line: e.line, Col: len(current)}, Caret{Line: e.line + 1, Col: 0}, "")
}

// DeleteWordBack removes from the cursor back to the start of the word behind it.
func (e *Editor) DeleteWordBack() {
	e.ensure()
	join := e.continuation == editorContinuationKill
	e.endTyping()
	if e.col == 0 {
		e.DeleteBack()
		return
	}
	e.snapshot()
	at := wordStart(e.lines[e.line], e.col)
	// A word boundary may fall inside an atomic element (the dot in a file chip is
	// not a word character). Once the deletion touches the element it must take the
	// whole value, just as backspace and forward delete do.
	if element, inside := e.insideElement(e.line, at); inside {
		at = element.Start
	}
	e.rememberKill(e.lines[e.line][at:e.col], true, join)
	e.replaceRange(Caret{Line: e.line, Col: at}, Caret{Line: e.line, Col: e.col}, "")
}

// KillToEnd cuts from the cursor to the end of the line, keeping what it cut.
//
// On an already-empty tail it takes the line break instead, which is what makes
// repeated presses swallow a paragraph rather than stop at the first line.
func (e *Editor) KillToEnd() {
	e.ensure()
	join := e.continuation == editorContinuationKill
	e.endTyping()
	current := e.lines[e.line]
	if e.col >= len(current) && e.line == len(e.lines)-1 {
		return
	}
	e.snapshot()
	if e.col < len(current) {
		e.rememberKill(current[e.col:], false, join)
		e.replaceRange(Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: len(current)}, "")
		return
	}
	if e.line < len(e.lines)-1 {
		e.rememberKill("\n", false, join)
		e.replaceRange(Caret{Line: e.line, Col: len(current)}, Caret{Line: e.line + 1, Col: 0}, "")
	}
}

// KillToStart cuts from the start of the line to the cursor.
func (e *Editor) KillToStart() {
	e.ensure()
	join := e.continuation == editorContinuationKill
	e.endTyping()
	if e.col == 0 {
		return
	}
	e.snapshot()
	e.rememberKill(e.lines[e.line][:e.col], true, join)
	e.replaceRange(Caret{Line: e.line}, Caret{Line: e.line, Col: e.col}, "")
}

// Yank puts back the most recently killed text.
func (e *Editor) Yank() {
	e.endTyping()
	killed, ok := e.kills.newest()
	if !ok {
		return
	}
	// Insert takes the snapshot, once, now that the run is closed.
	start := Caret{Line: e.line, Col: e.col}
	if selected, _, ok := e.Selection(); ok {
		start = selected
	}
	killed = e.canonicalText(killed)
	offset := e.offsetOf(start)
	e.Insert(killed)
	e.yank = editorYank{
		start: start,
		end:   e.caretAt(offset + len(killed)),
	}
	e.continuation = editorContinuationYank
}

// YankPop replaces the immediately preceding yank with the next older kill, cycling
// through the bounded ring. Any intervening edit, movement, or selection ends the
// sequence and makes this a no-op.
func (e *Editor) YankPop() {
	if e.continuation != editorContinuationYank {
		e.endTyping()
		return
	}
	killed, next, ok := e.kills.older(e.yank.ring)
	if !ok {
		e.endTyping()
		return
	}
	yank := e.yank
	offset := e.offsetOf(yank.start)
	killed, changed := e.prepareReplacement(yank.start, yank.end, killed)
	if changed {
		e.snapshot()
		e.replaceRange(yank.start, yank.end, killed)
	} else {
		e.finishReplacement(yank.end)
	}
	e.yank = editorYank{
		start: yank.start,
		end:   e.caretAt(offset + len(killed)),
		ring:  next,
	}
	e.continuation = editorContinuationYank
}

// MoveLeft moves one cluster left, over a line break when there is nowhere else.
func (e *Editor) MoveLeft() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col > 0 {
		// Stepping over an element rather than into it: it is one thing on screen,
		// and a cursor inside it has no position a reader could account for.
		e.col = e.snapElementBoundary(e.line, text.PrevCluster(e.lines[e.line], e.col), false)
		return
	}
	if e.line > 0 {
		e.line--
		e.col = len(e.lines[e.line])
	}
}

// MoveRight moves one cluster right, over a line break when there is nowhere else.
func (e *Editor) MoveRight() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col < len(e.lines[e.line]) {
		e.col = e.snapElementBoundary(e.line, nextClusterBoundary(e.lines[e.line], e.col), true)
		return
	}
	if e.line < len(e.lines)-1 {
		e.line++
		e.col = 0
	}
}

// MoveWordLeft moves to the start of the word behind the cursor.
func (e *Editor) MoveWordLeft() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col == 0 {
		e.MoveLeft()
		return
	}
	e.col = e.snapElementBoundary(e.line, wordStart(e.lines[e.line], e.col), false)
}

// MoveWordRight moves past the end of the word in front of the cursor.
func (e *Editor) MoveWordRight() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	if e.col == len(e.lines[e.line]) {
		e.MoveRight()
		return
	}
	e.col = e.snapElementBoundary(e.line, wordEnd(e.lines[e.line], e.col), true)
}

// MoveLineStart moves to the start of the logical line.
func (e *Editor) MoveLineStart() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	e.col = 0
}

// MoveLineEnd moves to the end of the logical line.
func (e *Editor) MoveLineEnd() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.wantColumn = -1
	e.col = len(e.lines[e.line])
}

// Undo steps back to before the last change.
func (e *Editor) Undo() {
	e.endTyping()
	if !e.history.canBack() {
		return
	}
	previous, ok := e.history.back(e.state())
	if !ok {
		return
	}
	e.restore(previous)
	e.typing = false
}

// Redo steps forward again.
func (e *Editor) Redo() {
	e.endTyping()
	if !e.history.canForward() {
		return
	}
	next, ok := e.history.forward(e.state())
	if !ok {
		return
	}
	e.restore(next)
	e.typing = false
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

// contentChanged advances the semantic generation, marks the layout out of date and
// drops the column vertical movement was aiming for.
//
// It says nothing about whether a run of typing is still open. Conflating the two is
// what made every keystroke take its own undo step: the flag that decides whether to
// snapshot was being cleared by the very operation that had just set it.
func (e *Editor) contentChanged() {
	e.revision++
	e.layout.stale = true
	e.wantColumn = -1
	e.revealCursor()
}

// requireContentRevision keeps exhaustion on the caller's side of a mutation, so a
// recovered panic cannot leave changed content carrying its old observation token.
func (e *Editor) requireContentRevision() {
	if e.revision == math.MaxUint64 {
		panic("headless: editor exhausted content revisions")
	}
}

// endTyping closes a run of insertions, the one thing a click could have said about
// where the cursor belongs, and any consecutive kill or yank operation.
//
// Every movement and every edit already calls this — it is the point they all pass
// through — so the affinity is cleared in one place rather than in the forty-odd places
// the cursor is assigned. A bit that every one of those had to reset would be reset in
// thirty-nine of them.
func (e *Editor) endTyping() {
	e.dragging = false
	e.typing = false
	e.rowEndSet = false
	e.breakContinuation()
}

// snapshot records the state for undo, coalescing a run of typing into one step so
// that undo steps over a phrase rather than a letter.
func (e *Editor) snapshot() {
	e.ensure()
	e.history.record(e.state())
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
	changed := !slices.Equal(e.lines, s.lines) || !slices.Equal(e.marks, s.marks)
	if changed {
		e.requireContentRevision()
	}
	e.lines = append([]string(nil), s.lines...)
	e.marks = append([]text.Mark(nil), s.marks...)
	e.line, e.col = s.line, s.col
	e.selecting = false
	if changed {
		e.contentChanged()
	}
}

// wordStart is the offset of the start of the word before i: any run of
// non-word characters, then the word itself, the way a terminal has always done it.
func wordStart(s string, i int) int {
	start := 0
	inWord := false
	for at, cluster := range text.Clusters(s[:i]) {
		word := isWord(cluster)
		if word && !inWord {
			start = at
		}
		inWord = word
	}
	return start
}

func wordEnd(s string, i int) int {
	found := false
	for at, cluster := range text.Clusters(s[i:]) {
		word := isWord(cluster)
		if found && !word {
			return i + at
		}
		found = found || word
	}
	return len(s)
}

// nextClusterBoundary advances from a position the editor already knows is a
// grapheme boundary. Unlike text.NextCluster it need not recover from an arbitrary
// byte offset, so segmentation can begin at that boundary instead of rescanning the
// complete prefix.
func nextClusterBoundary(s string, at int) int {
	if at >= len(s) {
		return len(s)
	}
	for _, cluster := range text.Clusters(s[at:]) {
		return at + len(cluster)
	}
	return at
}

// isWord reports whether a cluster is part of a word. Letters, digits and the
// underscore, so that a word motion in code stops where a reader expects.
func isWord(cluster string) bool {
	r, _ := utf8.DecodeRuneInString(cluster)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// InsertElement puts text at the cursor as one atomic unit, and returns it.
//
// Line breaks in body become spaces even in a multi-line editor. An element is one
// contiguous run of cells; allowing its source to span logical lines would make its
// returned line-local range describe only a fragment of what was inserted.
//
// A separator space follows it, which is what makes a chip in a prompt something a
// user can type after. The space is ordinary text and not part of the element: it is
// there to be deleted. One goes in front as well when the body would otherwise join
// what it lands after — see [Editor.joinsWhatPrecedes].
//
// An empty body inserts nothing and returns the zero [Element]. Identities are never
// reused, and InsertElement panics once every one has been issued: an [Element] the
// caller kept in order to replace or remove what it stands for would otherwise begin
// naming a different insertion.
func (e *Editor) InsertElement(kind ElementKind, body string) Element {
	body = elementBody(body)
	if body == "" {
		return Element{}
	}
	e.ensure()
	id, ok := e.elementIDs.next()
	if !ok {
		panic("headless: editor exhausted element identities")
	}
	e.endTyping()
	e.snapshot()
	start, end := Caret{Line: e.line, Col: e.col}, Caret{Line: e.line, Col: e.col}
	if selected, selectedEnd, ok := e.Selection(); ok {
		start, end = selected, selectedEnd
	}
	lead := ""
	if e.joinsWhatPrecedes(start, body) {
		lead = " "
	}
	at := e.offsetOf(start) + len(lead)
	replacement, changedText := e.prepareReplacement(start, end, lead+body+" ")
	if changedText {
		e.replaceRange(start, end, replacement)
	} else {
		e.requireContentRevision()
		e.finishReplacement(end)
	}
	mark := text.Mark{
		ID:     id,
		Kind:   int(kind),
		Start:  at,
		End:    at + len(body),
		Atomic: true,
	}
	// Put in order rather than appended and sorted: the marks are kept in the order
	// they appear, which is the order a caller expects and the order that makes them
	// readable in a test.
	where, _ := slices.BinarySearchFunc(e.marks, mark, func(a, b text.Mark) int {
		return a.Start - b.Start
	})
	e.marks = slices.Insert(e.marks, where, mark)
	if !changedText {
		e.contentChanged()
	}
	return e.elementOf(mark)
}

// joinsWhatPrecedes reports whether body would become part of the cluster in front
// of it.
//
// An element is one contiguous run of cells, and a run of cells begins at a grapheme
// boundary. A body that starts with a combining character has no boundary of its own:
// dropped after a letter it joins that letter's cluster, so the element's first cell
// belongs half to text the element does not own — and deleting the element leaves the
// mark behind on a character that was never part of it.
//
// The question is asked of the text it is actually landing after rather than of the
// body alone, because that is what decides it: two regional indicators are a flag,
// and a flag is a perfectly good label except directly after another one.
func (e *Editor) joinsWhatPrecedes(at Caret, body string) bool {
	if at.Line < 0 || at.Line >= len(e.lines) {
		return false
	}
	line := e.lines[at.Line]
	before := line[:min(max(at.Col, 0), len(line))]
	if before == "" {
		return false
	}
	return clusters(before+body) != clusters(before)+clusters(body)
}

// Elements is every element in the text, in the order they appear. The slice is a
// copy: a caller cannot move an element by writing to it.
func (e *Editor) Elements() []Element {
	out := make([]Element, 0, len(e.marks))
	for _, m := range e.marks {
		out = append(out, e.elementOf(m))
	}
	return out
}

// ElementAt is the element covering a position, and whether there is one. The end is
// exclusive, so the position just after an element is outside it.
func (e *Editor) ElementAt(line, col int) (Element, bool) {
	at := e.offsetOf(Caret{Line: line, Col: col})
	for _, m := range e.marks {
		if m.Covers(at) {
			return e.elementOf(m), true
		}
	}
	return Element{}, false
}

// RemoveElement deletes an element's text and forgets it, reporting whether it was
// there to remove.
func (e *Editor) RemoveElement(id uint64) bool {
	for _, m := range e.marks {
		if m.ID != id {
			continue
		}
		el := e.elementOf(m)
		e.endTyping()
		e.snapshot()
		// The space after it goes too, when there is one. It was put there with the
		// element and leaving it behind gives a prompt a gap where a chip used to be,
		// which is the sort of thing a user has to notice and tidy up by hand.
		end := el.End
		if line := e.lines[el.Line]; end < len(line) && line[end] == ' ' {
			end++
		}
		e.replaceRange(Caret{Line: el.Line, Col: el.Start}, Caret{Line: el.Line, Col: end}, "")
		return true
	}
	return false
}

// insideElement is the element a cursor position falls strictly within.
//
// Strictly, unlike [Editor.ElementAt]: an element's two ends are places a cursor may
// sit, and only what is between them is not. They are different questions and the
// difference matters — treating the start as inside would mean a cursor arriving from
// the left skipped straight past the element, and nothing could be typed in front of
// one.
func (e *Editor) insideElement(line, col int) (Element, bool) {
	at := e.offsetOf(Caret{Line: line, Col: col})
	for _, m := range e.marks {
		if m.Within(at) {
			return e.elementOf(m), true
		}
	}
	return Element{}, false
}

// snapElement moves a position out of any element it lands inside.
//
// Which way out depends on which way the cursor was going, which is the only thing
// that makes stepping over an element feel like stepping over a character: moving
// right from inside one has to come out at the far side, and moving left at the near
// side. A position that is not inside anything is returned as it is.
func (e *Editor) snapElement(line, col int, forward bool) int {
	// Before first use the zero editor has one conceptual empty line. Keeping this
	// primitive total preserves that zero-value contract even for an internal caller
	// that only needs to settle a position and has not initialized storage yet.
	if len(e.lines) == 0 {
		return 0
	}
	line = min(max(line, 0), len(e.lines)-1)
	col = clusterPosition(e.lines[line], col, forward)
	return e.snapElementBoundary(line, col, forward)
}

// snapElementBoundary applies only the atomic-element half of snapElement when its
// caller already owns a grapheme boundary. Cursor movement and column mapping have
// that stronger fact and should not rescan the line merely to prove it again.
func (e *Editor) snapElementBoundary(line, col int, forward bool) int {
	for {
		el, inside := e.insideElement(line, col)
		if !inside {
			return col
		}
		if forward {
			col = clusterPosition(e.lines[line], el.End, true)
		} else {
			col = clusterPosition(e.lines[line], el.Start, false)
		}
	}
}

// edited moves every element over a change to the text, dropping the ones the change
// destroyed.
//
// This is the whole of it. There used to be two of these — one for text going in and
// one for text coming out — each doing the same arithmetic in line and column space,
// each with its own edge cases and its own way of being wrong. An insertion, a
// deletion and a replacement are one thing said three ways, and [text.Edit] is that
// thing.
//
// It must be called with offsets into the text as it was before the change, which is
// why every caller works them out first.
func (e *Editor) edited(edit text.Edit) {
	e.marks = edit.Shift(e.marks, e.byteLength())
}

// settleMarks drops every element whose ends are no longer places a caret may sit —
// see [Element] for when that happens and what it means to a caller.
//
// Shifting the marks over a change says where they went, which is a question about
// offsets and belongs to [text.Edit]. Whether what is there is still an element is a
// question about the text, and only the editor can answer it — so it is answered
// here, once, for every change rather than at the one operation that was thought to
// need it.
func (e *Editor) settleMarks() {
	e.marks = slices.DeleteFunc(e.marks, func(m text.Mark) bool {
		return !e.spansWholeClusters(m)
	})
}

// spansWholeClusters reports whether a mark's ends are both places a caret may sit on
// one line.
func (e *Editor) spansWholeClusters(m text.Mark) bool {
	start, end := e.caretAt(m.Start), e.caretAt(m.End)
	if start.Line != end.Line {
		return false
	}
	line := e.lines[start.Line]
	return clusterPosition(line, start.Col, true) == start.Col &&
		clusterPosition(line, end.Col, true) == end.Col
}

// byteLength is the length of the whole text without assembling it. Edits and
// marks speak in whole-document byte offsets even though the editor owns lines.
func (e *Editor) byteLength() int {
	e.ensure()
	n := len(e.lines) - 1 // the newlines between lines
	for _, line := range e.lines {
		n += len(line)
	}
	return n
}

// removed moves every element over a range of the text being replaced by s, which
// covers a plain deletion as the case where s is empty.
//
// It has to be called before the lines change, because the carets it is given are
// carets into the text as it was.
func (e *Editor) removed(start, end Caret, s string) {
	e.edited(text.Edit{Start: e.offsetOf(start), End: e.offsetOf(end), Text: s})
}

// offsetOf is a caret as a byte offset into the whole text.
//
// The editor keeps its content as lines because that is what wrapping, vertical
// movement and the cursor are all expressed in. Marks are kept as offsets because
// that is what a change to text is expressed in, and translating between the two is
// this function and [Editor.caretAt]. Neither idea has to know about the other, which
// is the only reason the shifting rule could move out of this package at all.
func (e *Editor) offsetOf(c Caret) int {
	e.ensure()
	return offsetInLines(e.lines, c)
}

func (e *Editor) caretAt(at int) Caret {
	e.ensure()
	for i, line := range e.lines {
		if at <= len(line) {
			return Caret{Line: i, Col: max(at, 0)}
		}
		at -= len(line) + 1
	}
	last := len(e.lines) - 1
	return Caret{Line: last, Col: len(e.lines[last])}
}

func (e *Editor) elementOf(m text.Mark) Element {
	start := e.caretAt(m.Start)
	end := e.caretAt(m.End)
	if end.Line != start.Line {
		// An element never spans a line break, so a mark that reads as though it does
		// is reported as far as the end of the line it began on. Nothing produces one:
		// an edit that put a break inside a mark destroyed it.
		end = Caret{Line: start.Line, Col: len(e.lines[start.Line])}
	}
	return Element{
		ID:    m.ID,
		Kind:  kindOf(m.Kind),
		Line:  start.Line,
		Start: start.Col,
		End:   end.Col,
	}
}

// RetainedElementIDs returns identities reachable from the document or undo/redo
// history. Applications may release associated payloads only after they disappear
// from this set. The returned slice is owned by the caller.
func (e *Editor) RetainedElementIDs() []uint64 {
	ids := make(map[uint64]struct{})
	for _, mark := range e.marks {
		ids[mark.ID] = struct{}{}
	}
	for _, stack := range [][]editorState{e.history.undo, e.history.redo} {
		for _, state := range stack {
			for _, mark := range state.marks {
				ids[mark.ID] = struct{}{}
			}
		}
	}
	out := make([]uint64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// ForgetHistory ends the lifetime of edits no longer available to Undo or Redo.
// Current document elements remain live.
func (e *Editor) ForgetHistory() { e.endTyping(); e.history.clear() }

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
	if !ok {
		return false
	}
	return e.handleKey(key, e.Do, e.typed)
}

// handleKey leaves action and insertion ownership with the controller. Deferred
// keymap resolution must re-enter that same controller's acceptance boundary.
func (e *Editor) handleKey(key input.Key, do func(keymap.Action) bool, typed func(input.Key) bool) bool {
	if !key.Down() {
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

	matched, handled := e.matcher.Handle(e.keys(), key, do)
	if !matched {
		return typed(key)
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
	do, ok := editorActions[action]
	if !ok {
		return false
	}
	do(e)
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

func (e *Editor) keys() *keymap.Map {
	if e.Keys != nil {
		return e.Keys
	}
	return editorKeys()
}

func (e *Editor) breakContinuation() {
	e.continuation = editorContinuationNone
	e.yank = editorYank{}
}

func (e *Editor) rememberKill(text string, prepend, join bool) {
	e.kills.add(text, prepend, join)
	e.continuation = editorContinuationKill
	e.yank = editorYank{}
}

// rowAt is the visual row the cursor is on at a width, and the column within it.
//
// The one position with two answers is where the width broke a line: the offset after
// its last character and the offset before the next row's first are one offset with two
// places on screen. A cursor that arrived by moving through the text belongs to the
// second, and one that arrived by being clicked past the end of a row belongs to the
// first. Nothing in the offset says which, so the field remembers — see
// [Editor.prefersRowEnd].
func (e *Editor) rowAt(width int) (row, column int) {
	rows := e.rows(width)
	atEnd := e.prefersRowEnd()
	for i, r := range rows {
		if r.line != e.line {
			continue
		}
		// The end of a row is the start of the next, so a cursor there belongs to the
		// next row — except on the last row of a line, where there is no next and the
		// cursor sits after the final character, and except when it was put there by a
		// click on this row.
		if e.col < r.end || (e.col == r.end && (atEnd || e.layout.lastOfLine(i))) {
			return i, text.ColumnOf(e.lines[e.line][r.start:r.end], e.col-r.start)
		}
	}
	if len(rows) == 0 {
		return 0, 0
	}
	return len(rows) - 1, 0
}

func (e *Editor) offsetIn(width, row, column int) (line, col int) {
	rows := e.rows(width)
	if row < 0 || row >= len(rows) {
		return 0, 0
	}
	r := rows[row]
	segment := e.lines[r.line][r.start:r.end]
	return r.line, r.start + text.OffsetAt(segment, column)
}

// MoveUp moves the cursor up one visual row, keeping the column it started from.
func (e *Editor) MoveUp() { e.moveRow(-1) }

// MoveDown moves the cursor down one visual row.
func (e *Editor) MoveDown() { e.moveRow(1) }

// moveRow moves the cursor by visual rows.
//
// The column the cursor was in is remembered across the whole run of movement, so
// travelling down through a short line and out the other side comes back to where it
// went in. Recomputing it each step would drag the cursor left and leave it there.
func (e *Editor) moveRow(delta int) {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	width := e.presentation.Value().width
	if width <= 0 {
		// Nothing has been drawn yet, so there are no visual rows to move through.
		// Logical lines are the best available answer, but the horizontal coordinate
		// is still a terminal column rather than a byte offset. Copying the byte offset
		// to a UTF-8 line can put the cursor in the middle of a rune.
		column := text.ColumnOf(e.lines[e.line], e.col)
		if e.wantColumn >= 0 {
			column = e.wantColumn
		}
		target := min(max(e.line+delta, 0), len(e.lines)-1)
		if target == e.line {
			return
		}
		e.line = target
		e.col = e.snapElementBoundary(target, text.OffsetAt(e.lines[target], column), true)
		e.wantColumn = column
		return
	}
	row, column := e.rowAt(width)
	if e.wantColumn >= 0 {
		column = e.wantColumn
	}
	target := row + delta
	if target < 0 || target >= len(e.rows(width)) {
		return
	}
	e.line, e.col = e.offsetIn(width, target, column)
	e.col = e.snapElementBoundary(e.line, e.col, true)
	e.wantColumn = column
}

// HeightForWidth is how many rows the field needs at a width, within its cap.
func (e *Editor) HeightForWidth(width int) int {
	e.ensure()
	width = e.textWidth(width)
	if e.oneLine() {
		return 1
	}
	rows := len(e.rows(width))
	if e.MaxRows > 0 {
		return min(rows, e.MaxRows)
	}
	return rows
}

// Draw paints the field and places the cursor.
func (e *Editor) Draw(frame Frame) {
	e.DrawWith(frame, e.Look)
}

// DrawWith paints one projection with look without changing the editor's configured
// appearance.
//
// Appearance components use this when an editor participates in a larger theme. The
// editor remains the single owner of its text, cursor and input configuration; drawing
// it through another look does not make that look its configuration.
func (e *Editor) DrawWith(frame Frame, look Look) {
	presented := &e.presentation
	if e.oneLine() {
		e.lineView(e.Text()).draw(frame, look, presented)
		return
	}
	v := frame.View
	total, height := v.Size()
	if total <= 0 || height <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	e.ensure()
	gutter := min(e.gutterWidth(), total)
	width := layout.Remaining(total, gutter)
	gutterView := v.Sub(grid.Area(0, 0, gutter, height))
	v = v.Sub(grid.Area(gutter, 0, width, height))
	if width <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
	presentation := e.drawMultiline(frame, v, gutterView, look, width, height, gutter)
	presented.Stage(frame, presentation)
}

func (e *Editor) drawMultiline(
	frame Frame,
	view, gutterView grid.View,
	look Look,
	width, height, gutter int,
) editorPresentation {
	rows := e.rows(width)
	cursorRow, cursorColumn := e.rowAt(width)

	scroll := e.scroll.Stage(frame, len(rows), height)
	if e.cursorReveal != nil && e.scroll.reveal == e.cursorReveal {
		// Wrapping may have changed since navigation. Resolve the logical cursor
		// using this frame, but only while its request has not been superseded.
		scroll.Reveal(cursorRow, cursorRow)
	}
	first := scroll.Offset()
	presentation := editorPresentation{width: width, gutter: gutter, first: first}
	last := min(layout.Sum(first, height), len(rows))
	e.drawGutter(gutterView, rows[first:last])

	if e.Empty() && e.Placeholder != "" {
		view.Text(0, 0, text.Truncate(e.Placeholder, width, look.Ellipsis), look.Subtle)
		e.placeCursor(view, 0, 0)
		return presentation
	}

	for y := range height {
		index := layout.Sum(first, y)
		if index >= len(rows) {
			break
		}
		r := rows[index]
		text.Of(e.lines[r.line][r.start:r.end], look.Text).Draw(view, 0, y)
	}
	// The selection is laid over the text rather than drawn into it, so a run that
	// crosses a style boundary keeps whatever was underneath — and so that the rows
	// above did not have to be told which of them was selected.
	for _, span := range e.spansOfSelection(width) {
		y := span.Row - first
		if y < 0 || y >= height {
			continue
		}
		end := min(layout.Sum(span.Col, span.Width), width)
		for x := max(span.Col, 0); x < end; x++ {
			view.MergeStyle(x, y, look.Selection)
		}
	}
	if y := cursorRow - first; y >= 0 && y < height {
		e.placeCursor(view, cursorColumn, y)
	}
	return presentation
}

func (e *Editor) gutterWidth() int {
	if e.Gutter == nil {
		return 0
	}
	return max(e.Gutter.Width(len(e.lines)), 0)
}

func (e *Editor) textWidth(total int) int {
	return layout.Remaining(total, e.gutterWidth())
}

func (e *Editor) drawGutter(view grid.View, rows []editorRow) {
	if e.Gutter == nil {
		return
	}
	out := make([]text.Row, len(rows))
	for i, row := range rows {
		shown := e.lines[row.line][row.start:row.end]
		if e.mask != "" {
			// A masked field must not disclose its value to an appearance callback.
			shown = e.shown()
		}
		out[i] = text.Row{
			Text: shown,
			Line: row.line + 1, Joined: row.joined,
		}
	}
	e.Gutter.Draw(view, out)
}

// Focus takes the keyboard, or gives it up. A field without it draws no cursor.
//
// A frame has one cursor and the terminal draws it, so two fields both asking for it
// is not two cursors: it is one, wherever the last of them happened to draw. This is
// how the question is settled — see [Focusable], and note that a field nobody has
// told anything believes it has the keyboard, which is what makes a lone field work.
func (e *Editor) Focus(has bool) {
	if !has {
		e.matcher.Clear()
		e.endTyping()
	}
	e.blurred = !has
}

// placeCursor asks for the terminal's cursor only when this field has the keyboard.
func (e *Editor) placeCursor(v grid.View, x, y int) {
	if e.blurred {
		return
	}
	v.PlaceCursor(x, y, e.CursorStyle)
}

// revealCursor is the semantic navigation boundary. A draw may refine the visual
// row but never creates a new request; manual scrolling can therefore cancel it.
func (e *Editor) revealCursor() {
	if e.oneLine() {
		return
	}
	e.ensure()
	width := e.presentation.Value().width
	row, _ := e.rowAt(width)
	e.scroll.layout(len(e.rows(width)), e.scroll.current.window)
	e.scroll.Reveal(row, row)
	e.cursorReveal = e.scroll.reveal
}

// Scroll exposes the field's position, for a scrollbar beside a tall field.
// Editing and cursor navigation reveal the cursor once. Manual scrolling remains
// in effect through redraws and resizes until the next navigation or edit.
func (e *Editor) Scroll() *Scroll { return &e.scroll }

// rows is the field's text laid out at a width.
//
// A field holding one line is laid out at no width at all, which is how the wrap is
// told not to break anything: the line is one row however long it is, and what is off
// the side of the box is off the side of the box. Everything that reads rows — moving
// the cursor, finding a click, drawing a selection — then agrees, because there is one
// layout and they all ask it.
func (e *Editor) rows(width int) []editorRow {
	if e.oneLine() {
		width = 0
	}
	return e.layout.rowsFor(e.lines, width)
}

func (e *Editor) lineView(value string) editorLineView {
	view := editorLineView{
		value: value, placeholder: e.Placeholder, mask: e.mask,
		cursor: e.col, anchor: e.anchor.Col, selecting: e.selecting,
		blurred: e.blurred, left: e.presentation.Value().left,
		gutter: e.Gutter, cursorStyle: e.CursorStyle,
	}
	if value != e.Text() {
		view.cursor, view.selecting = len(value), false
	}
	return view
}

// shown is the text as it is drawn: the line itself, or the mask once per cluster for
// a field holding something the screen should not show.
func (e *Editor) shown() string {
	e.ensure()
	return e.lineView(e.lines[0]).shown()
}

func (e *Editor) lineAt(at int) int {
	if e.mask == "" {
		return at
	}
	line := e.lines[0]
	if at <= 0 {
		return 0
	}
	want := at / len(e.mask)
	seen := 0
	for offset := range text.Clusters(line) {
		if seen == want {
			return offset
		}
		seen++
	}
	return len(line)
}

func (e *Editor) atLine(x int) Caret {
	col := e.lineAt(text.OffsetAt(e.shown(), layout.Translate(x, e.presentation.Value().left)))
	return Caret{Col: e.snapElement(0, col, true)}
}

// Anchor begins or continues a selection at the cursor.
//
// A selection is not a separate mode with commands of its own. It is what movement
// means while the shift key is held, so every way of moving a cursor selects with
// shift and none of them had to be taught to — see [Editor.Handle].
func (e *Editor) Anchor() {
	e.breakContinuation()
	if !e.selecting {
		e.anchor, e.selecting = Caret{Line: e.line, Col: e.col}, true
	}
}

// SelectNone drops the selection, leaving the cursor where it is.
func (e *Editor) SelectNone() {
	e.dragging = false
	e.breakContinuation()
	e.selecting = false
}

// SelectAll selects the whole text.
func (e *Editor) SelectAll() {
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.anchor, e.selecting = Caret{}, true
	e.line = len(e.lines) - 1
	e.col = len(e.lines[e.line])
	e.wantColumn = -1
}

// Selection is the selected range in reading order, and whether there is one.
//
// It reports false for a selection of nothing, which is what a shift-arrow pressed
// and then taken back leaves: an anchor at the cursor is not a selection, and
// treating it as one would make a copy put an empty string on the clipboard.
func (e *Editor) Selection() (start, end Caret, ok bool) {
	if !e.selecting {
		return Caret{}, Caret{}, false
	}
	cursor := Caret{Line: e.line, Col: e.col}
	if cursor == e.anchor {
		return Caret{}, Caret{}, false
	}
	if cursor.Before(e.anchor) {
		return cursor, e.anchor, true
	}
	return e.anchor, cursor, true
}

// Selected is the selected text, or empty when nothing is selected.
func (e *Editor) Selected() string {
	start, end, ok := e.Selection()
	if !ok {
		return ""
	}
	return e.textBetween(start, end)
}

func (e *Editor) textBetween(start, end Caret) string {
	e.ensure()
	if start.Line == end.Line {
		return e.lines[start.Line][start.Col:end.Col]
	}
	var b strings.Builder
	b.WriteString(e.lines[start.Line][start.Col:])
	for i := start.Line + 1; i < end.Line; i++ {
		b.WriteByte('\n')
		b.WriteString(e.lines[i])
	}
	b.WriteByte('\n')
	b.WriteString(e.lines[end.Line][:end.Col])
	return b.String()
}

// DeleteSelection removes the selected text and reports whether there was any.
func (e *Editor) DeleteSelection() bool {
	start, end, ok := e.Selection()
	if !ok {
		return false
	}
	e.snapshot()
	e.endTyping()
	e.replaceRange(start, end, "")
	return true
}

// prepareReplacement applies the editor's one-line rule and reports whether replacing
// a range changes text or atomic elements. Its returned text is the canonical value
// replaceRange consumes, so normalization occurs at this one boundary.
//
// Comparing text alone is not enough: replacing a chip with the same visible word
// removes the element identity and is therefore a semantic change too. Conversely,
// an empty edit inside a chip is still empty and must not destroy it.
func (e *Editor) prepareReplacement(start, end Caret, s string) (string, bool) {
	s = e.canonicalText(s)
	if e.textBetween(start, end) != s {
		return s, true
	}
	if start == end && s == "" {
		return s, false
	}
	from, to := e.offsetOf(start), e.offsetOf(end)
	for _, mark := range e.marks {
		if from < mark.End && to > mark.Start {
			return s, true
		}
	}
	return s, false
}

// finishReplacement settles what a replacement leaves behind, whether or not its
// bytes changed: which elements are still elements, where the cursor belongs, and
// that the old selection no longer describes an active range. Revision, history and
// layout remain separate because an identity replacement changes none of them.
//
// The elements are settled before the cursor, because a cursor that snapped over an
// element the replacement destroyed would have stepped over nothing.
func (e *Editor) finishReplacement(at Caret) {
	defer e.revealCursor()
	e.settleMarks()
	e.selecting = false
	e.line = min(max(at.Line, 0), len(e.lines)-1)
	e.col = e.snapElement(e.line, at.Col, true)
	e.wantColumn = -1
}

// replaceRange is the one operation that changes editor text. Insertion is an empty
// range, deletion has empty replacement text, and replacement is both. The caller
// has established that the range changes semantic content and taken any undo snapshot
// it needs.
func (e *Editor) replaceRange(start, end Caret, s string) {
	e.requireContentRevision()
	e.layout.stale = true
	e.removed(start, end, s)
	head := e.lines[start.Line][:start.Col]
	tail := e.lines[end.Line][end.Col:]
	if !strings.Contains(s, "\n") {
		e.lines = slices.Replace(e.lines, start.Line, end.Line+1, ownedEditorLine(head, s, tail))
		e.finishReplacement(Caret{Line: start.Line, Col: start.Col + len(s)})
		e.contentChanged()
		return
	}
	parts := strings.Split(s, "\n")
	inserted := make([]string, len(parts))
	inserted[0] = ownedEditorLine(head, parts[0])
	for i := 1; i < len(parts); i++ {
		inserted[i] = strings.Clone(parts[i])
	}
	last := len(inserted) - 1
	cursor := Caret{Line: start.Line + last, Col: len(inserted[last])}
	inserted[last] = ownedEditorLine(inserted[last], tail)
	e.lines = slices.Replace(e.lines, start.Line, end.Line+1, inserted...)
	e.finishReplacement(cursor)
	e.contentChanged()
}

// Copy puts the selection where a paste would find it, and reports whether anything
// was sent. Nothing selected sends nothing, which is not a failure.
func (e *Editor) Copy() bool {
	if e.Clipboard == nil {
		return false
	}
	selected := e.Selected()
	if selected == "" {
		return false
	}
	return e.Clipboard.Copy(selected)
}

// Cut copies the selection and removes it.
//
// The text is removed only if the clipboard took it. A cut that emptied the field
// into a clipboard that refused it would lose the text with nothing to paste back,
// and a terminal is free to refuse.
func (e *Editor) Cut() bool {
	e.endTyping()
	if !e.Copy() {
		return false
	}
	return e.DeleteSelection()
}

// Paste asks the clipboard for its contents and reports whether the request was
// accepted. What comes back arrives later as an ordinary paste event, which this
// editor already inserts.
func (e *Editor) Paste() bool {
	e.endTyping()
	return e.Clipboard != nil && e.Clipboard.Paste()
}

// move runs an action if it is a way of moving, reporting whether it was one.
//
// It is separate from the rest of [Editor.Do] because it is asked twice: once for what
// the keystroke named, and once for what it named with the shift taken off it, which is
// what makes every movement a way of selecting. Selecting is this without the selection
// being let go of first.
func (e *Editor) move(action keymap.Action) bool {
	switch action {
	case MoveLeft:
		e.MoveLeft()
	case MoveRight:
		e.MoveRight()
	case MoveUp:
		e.MoveUp()
	case MoveDown:
		e.MoveDown()
	case MoveWordLeft:
		e.MoveWordLeft()
	case MoveWordRight:
		e.MoveWordRight()
	case MoveLineStart:
		e.MoveLineStart()
	case MoveLineEnd:
		e.MoveLineEnd()
	default:
		return false
	}
	return true
}

// Spans is the runs of columns that the text between two carets covers, one per
// visual row it crosses.
//
// A range in a wrapped field is not a rectangle and is rarely one run. It starts part
// way along a row, covers whole rows, and ends part way along another — and where the
// rows begin and end is decided by the wrap, which is decided by the width. So this
// is a question only the field can answer, and only at a width. Width is the whole
// field, including any [Editor.Gutter], and returned columns use that same coordinate
// space.
//
// It reads the same rows the cursor is placed with. That is the whole point of it
// being here rather than being worked out by whatever draws: a selection painted from
// one wrap and a cursor placed from another disagree about where the text is, and the
// disagreement shows up exactly when the text is interesting — a long word, a wide
// character, a line that just fits.
func (e *Editor) Spans(from, to Caret, width int) []RowSpan {
	gutter := e.gutterWidth()
	out := e.spans(from, to, layout.Remaining(width, gutter))
	for i := range out {
		out[i].Col = layout.Sum(out[i].Col, gutter)
	}
	return out
}

func (e *Editor) spans(from, to Caret, width int) []RowSpan {
	if width <= 0 {
		return nil
	}
	e.ensure()
	if to.Before(from) {
		from, to = to, from
	}
	if e.mask != "" {
		view := e.lineView(e.Text())
		shown := view.shown()
		if from.Line != 0 || to.Line != 0 {
			return nil
		}
		start := text.ColumnOf(shown, view.shownAt(from.Col))
		end := text.ColumnOf(shown, view.shownAt(to.Col))
		if end <= start {
			return nil
		}
		return []RowSpan{{Row: 0, Col: start, Width: end - start}}
	}
	rows := e.rows(width)

	var out []RowSpan
	for i, r := range rows {
		if r.line < from.Line || r.line > to.Line {
			continue
		}
		// The part of this row the range covers, in offsets into the line.
		lo, hi := r.start, r.end
		if r.line == from.Line {
			lo = max(lo, from.Col)
		}
		if r.line == to.Line {
			hi = min(hi, to.Col)
		}
		if lo >= hi {
			continue
		}
		line := e.lines[r.line]
		col := text.ColumnOf(line[r.start:r.end], lo-r.start)
		end := text.ColumnOf(line[r.start:r.end], hi-r.start)
		out = append(out, RowSpan{Row: i, Col: col, Width: end - col})
	}
	return out
}

// SelectionSpans is where the selection is, or nothing when there is none.
func (e *Editor) SelectionSpans(width int) []RowSpan {
	start, end, ok := e.Selection()
	if !ok {
		return nil
	}
	return e.Spans(start, end, width)
}

func (e *Editor) spansOfSelection(width int) []RowSpan {
	start, end, ok := e.Selection()
	if !ok {
		return nil
	}
	return e.spans(start, end, width)
}

// At is the position in the text under a point in the field's box, and whether the
// point is in the text at all.
//
// The point is in the field's own coordinates, which is what a widget is handed. The
// answer accounts for the field having scrolled, because the field is what knows it.
//
// It reads the same rows the cursor is placed from and the selection is painted from,
// which is the only way a click can land where the reader thinks they clicked: three
// walks over three wraps agree until the text is interesting, and then they do not.
func (e *Editor) At(x, y, width int) (Caret, bool) {
	gutter := e.gutterWidth()
	if x < gutter {
		return Caret{}, false
	}
	return e.at(x-gutter, y, layout.Remaining(width, gutter))
}

func (e *Editor) at(x, y, width int) (Caret, bool) {
	if width <= 0 || x < 0 || y < 0 {
		return Caret{}, false
	}
	e.ensure()
	if e.oneLine() {
		return e.atLine(x), true
	}
	rows := e.rows(width)
	presented := e.presentation.Value()
	first := 0
	if presented.width == width {
		first = presented.first
	}
	index := layout.Sum(first, y)
	if index >= len(rows) {
		// Below the text. The end is where a click there means, the way it does in
		// every editor: a reader clicking past the last line means the last line.
		//
		// There is always a row to be past, so nothing here checks. A field holds at
		// least one line even when it is empty, and every line gets a row even when
		// it has nothing on it — a blank line in a composer is a blank line on screen.
		last := rows[len(rows)-1]
		return Caret{Line: last.line, Col: last.end}, true
	}
	r := rows[index]
	line := e.lines[r.line]
	col := r.start + text.OffsetAt(line[r.start:r.end], x)
	// Out of any element it lands in, forwards, so a click in the middle of a chip
	// puts the cursor after it rather than inside — the same rule moving does.
	return Caret{Line: r.line, Col: e.snapElement(r.line, col, true)}, true
}

// handleMouse answers a mouse event at a width, reporting whether it consumed it.
//
// A press puts the cursor where it was pressed and starts a selection there; a drag
// with the button held moves the far end; a release ends it. That is what a text field
// does everywhere.
//
// The width is a parameter rather than a field because this is where the arithmetic
// lives, not because a caller gets to choose one. [Editor.Handle] supplies the width
// the editor last drew at, which is the only width a pointer event can be about: a
// press is aimed at what is on the screen. Routing one against a width that was never
// presented would answer a question nobody asked.
func (e *Editor) handleMouse(ev input.Mouse, presented editorPresentation) bool {
	if ev.Action == input.MouseUp {
		wasDragging := e.dragging
		e.dragging = false
		return wasDragging
	}
	if ev.Action == input.MouseDown {
		e.dragging = false
	}
	if presented.width <= 0 || ev.Pos.X < presented.gutter {
		return false
	}
	ev.Pos.X = layout.Relative(ev.Pos.X, presented.gutter)
	switch ev.Action {
	case input.MouseDown:
		if ev.Button != input.ButtonLeft {
			return false
		}
		at, ok := e.at(ev.Pos.X, ev.Pos.Y, presented.width)
		if !ok {
			return false
		}
		e.SelectNone()
		e.endTyping()
		e.line, e.col, e.wantColumn = at.Line, at.Col, -1
		e.anchor, e.selecting = at, true
		// A click that landed past the end of a wrapped row means that row, not the
		// start of the next. It is the only way a cursor comes to be at a soft break
		// and belong to the earlier side.
		e.rowEnd, e.rowEndSet = at, e.pastRowEnd(ev.Pos.X, ev.Pos.Y, presented.width)
		e.dragging = true
		e.revealCursor()
		return true
	case input.MouseDrag:
		if !e.dragging {
			return false
		}
		at, ok := e.at(ev.Pos.X, ev.Pos.Y, presented.width)
		if !ok {
			return false
		}
		e.line, e.col, e.wantColumn = at.Line, at.Col, -1
		e.revealCursor()
		return true
	default:
		return false
	}
}

// pastRowEnd reports whether a point is beyond the text of a row that the width broke,
// which is the only place a cursor can belong to the earlier side of a break.
func (e *Editor) pastRowEnd(x, y, width int) bool {
	rows := e.rows(width)
	presented := e.presentation.Value()
	first := 0
	if presented.width == width {
		first = presented.first
	}
	index := layout.Sum(first, y)
	if index < 0 || index >= len(rows) || e.layout.lastOfLine(index) {
		return false
	}
	r := rows[index]
	return x >= text.Width(e.lines[r.line][r.start:r.end])
}

// prefersRowEnd reports whether the cursor should be drawn at the end of a wrapped row
// rather than at the start of the next. See [Editor.rowEnd].
func (e *Editor) prefersRowEnd() bool {
	return e.rowEndSet && e.rowEnd == Caret{Line: e.line, Col: e.col}
}

// editorActions is what each name the field answers to does. A table rather than a
// switch, so that the set of actions is one list a reader can see the whole of.
var editorActions = map[keymap.Action]func(*Editor){
	// A delete with something selected takes the selection, which is what makes
	// backspace and the delete key mean "get rid of this" when there is a this.
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
	Copy:           func(e *Editor) { e.Copy() },
	Cut:            func(e *Editor) { e.Cut() },
	Paste:          func(e *Editor) { e.Paste() },
}

// RowSpan is a run of columns on one visual row of a field.
//
// The row is counted from the top of the whole wrapped text and not from the top of
// the box, because the field scrolls: a caller that wanted rows on screen would have
// to be told the scroll position to make sense of them, and the field already knows
// it.
type RowSpan struct {
	Row        int
	Col, Width int
}
