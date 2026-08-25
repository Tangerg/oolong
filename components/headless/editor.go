package headless

import (
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
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
	layout editorLayout
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
// A mask must be valid, visible terminal text without tabs or control characters;
// invalid configuration panics. A non-empty mask makes the field one-line, applying
// the same semantic transition as [Editor.SetSingleLine].
func (e *Editor) SetMask(mask string) {
	if mask != "" && (text.Printable(mask) != mask || strings.Contains(mask, "\t") || text.Width(mask) == 0) {
		panic("headless: editor mask must be visible terminal text without controls")
	}
	if e.mask == mask {
		return
	}
	wasOneLine := e.oneLine()
	e.mask = strings.Clone(mask)
	e.oneLineChanged(wasOneLine)
}

// oneLineChanged maintains the storage invariant after a mode transition.
func (e *Editor) oneLineChanged(wasOneLine bool) {
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

// oneLine reports whether the field holds a single line.
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
	e.Insert(killed)
	e.yank = editorYank{
		start: start,
		end:   Caret{Line: e.line, Col: e.col},
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
	killed, changed := e.prepareReplacement(yank.start, yank.end, killed)
	if changed {
		e.snapshot()
		e.replaceRange(yank.start, yank.end, killed)
	} else {
		e.finishReplacement(yank.end)
	}
	e.yank = editorYank{
		start: yank.start,
		end:   Caret{Line: e.line, Col: e.col},
		ring:  next,
	}
	e.continuation = editorContinuationYank
}

// MoveLeft moves one cluster left, over a line break when there is nowhere else.
func (e *Editor) MoveLeft() {
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

// wordEnd is the offset past the end of the word after i.
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
