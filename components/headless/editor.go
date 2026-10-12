package headless

import (
	"image"
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

// Editor owns text, grapheme-aligned cursor positions and visual-row navigation.
// The zero value is ready. An Editor must not be copied after first use.
type Editor struct {
	noCopy noCopy

	Placeholder string
	Look        Look
	// Nil uses [DefaultEditorKeys].
	Keys *keymap.Map
	// Nil disables clipboard actions.
	Clipboard Clipboard
	// Zero allows unlimited height; a positive limit scrolls to keep the cursor visible.
	MaxRows int
	// Gutter input belongs to the containing component; its text is excluded from selection.
	Gutter      RowGutter
	CursorStyle grid.CursorStyle

	lines []string
	// line is the cursor's logical line; col is its byte offset within that line.
	line, col int
	// Short rows must not erase the desired column for later vertical moves. Negative means unset.
	wantColumn int

	// The cursor is the other endpoint, so movement cannot leave a stale selection end.
	anchor    Caret
	selecting bool
	dragging  bool

	// Soft breaks share a byte offset with two visual rows. Clicks may prefer the earlier
	// row; endTyping clears this affinity for subsequent movement and edits.
	rowEnd    Caret
	rowEndSet bool

	// Whole-document byte offsets let [text.Edit] shift every mark through one edit policy.
	marks      []text.Mark
	elementIDs identitySequence

	kills        editorKillRing
	continuation editorContinuation
	yank         editorYank

	matcher keymap.Matcher

	// Inverted so a standalone zero-value editor owns the keyboard.
	blurred bool

	history editorHistory
	typing  bool
	// Cursor and presentation changes must not advance the semantic content generation.
	revision uint64
	// Setters enforce one-line storage before a mask or mode change can hide content.
	singleLine bool
	mask       string

	scroll Scroll
	// Scroll owns request consumption and cancellation; drawing only resolves its visual row.
	cursorReveal *scrollRange
	layout       editorLayout
	presentation Snapshot[editorPresentation]
}

type editorPresentation struct {
	width, gutter, first, left int
	visible                    image.Rectangle
}

func (p editorPresentation) textPoint(point image.Point) (image.Point, bool) {
	if p.width <= 0 || point.X < p.gutter {
		return image.Point{}, false
	}
	point.X = layout.Relative(point.X, p.gutter)
	return point, true
}

type editorState struct {
	lines     []string
	line, col int
	// Text cannot reconstruct element identities, so undo must retain the marks.
	marks []text.Mark
}

// Own slice headers so a rejected controlled edit can restore history after reentrant mutation.
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

// Revision advances once per semantic text or element change, including undo and redo.
// Cursor, selection, scrolling and no-op edits leave it unchanged.
// Compare tokens only within this editor; they have no persisted or numeric meaning.
func (e *Editor) Revision() uint64 { return e.revision }

// SingleLine reports whether this field was explicitly configured to hold one line.
// A non-empty [Editor.Mask] also makes the effective field one-line.
func (e *Editor) SingleLine() bool { return e.singleLine }

// SetSingleLine changes the explicit one-line setting. Enabling flattens line breaks,
// retains element identities and translates the cursor as one semantic change.
// It clears history so undo cannot restore invalid multi-line content. A non-empty
// mask still requires one-line storage when the explicit setting is disabled.
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

// SetMask changes the glyph drawn for each text cluster. Invalid masks panic: they
// must be visible, free of tabs and controls, and remain separate when repeated.
// A non-empty mask applies the one-line transition of [Editor.SetSingleLine].
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

// Repeated masks must not join into grapheme clusters, or multiplication would place
// the cursor beyond the rendered text.
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
	// Old undo snapshots may contain lines even when the current value does not.
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

// SetText replaces the content and places the cursor at its end.
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

// Owner normalization belongs to the completed edit and must not add a revision or
// undo step. Translate positions through the changed interval, then restore grapheme
// and element boundaries.
func (e *Editor) reconcileEdit(s string) {
	e.ensure()
	s = e.canonicalText(s)
	requested := e.Text()
	if requested == s {
		return
	}

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

// Rejected edits still end typing so the next insertion cannot coalesce without an undo snapshot.
func (e *Editor) rejectEdit(checkpoint editorCheckpoint) {
	e.lines, e.marks = checkpoint.state.lines, checkpoint.state.marks
	e.line, e.col = checkpoint.state.line, checkpoint.state.col
	e.anchor, e.selecting = checkpoint.anchor, checkpoint.selecting
	e.history, e.revision = checkpoint.history, checkpoint.revision
	e.endTyping()
	e.wantColumn = -1
	e.layout.stale = true
}

// Rune-aligned prefix and suffix comparisons avoid matching unequal UTF-8 runes by
// shared bytes. Interior positions preserve their distance within the replacement,
// then snap to UTF-8 boundaries; consumers own grapheme and element snapping.
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

// Cursor returns the logical line and byte offset.
func (e *Editor) Cursor() (line, col int) {
	e.ensure()
	return e.line, e.col
}

// SetCursor clamps the logical line and byte offset, snapping backward to a grapheme boundary.
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

// Insert inserts text at the cursor, splitting lines at newlines.
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
	// Selection replacement and insertion share one undo step.
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

// Expand non-empty edits to whole graphemes; insertions inside a cluster land before
// it so neither UTF-8 nor terminal cursor positions can be split.
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

// All text mutations share storage normalization so paste, typing and replacement
// cannot introduce different line endings, invalid UTF-8 or terminal controls.
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

// Pure projections share one-line normalization without constructing or mutating an Editor.
func oneLineText(s string) string { return text.Printable(flattenLines(s)) }

func flattenLines(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	return flatLineBreaks.Replace(s)
}

// Immutable replacers are shared safely without allocating one for each edit.
var (
	logicalLineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")
	flatLineBreaks    = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")
)

func (e *Editor) oneLine() bool { return e.singleLine || e.mask != "" }

// InsertRune puts one character in.
func (e *Editor) InsertRune(r rune) {
	// InsertRune cannot create logical lines; Printable preserves tabs but not line breaks.
	e.typeText(text.Printable(string(r)))
}

// A handled no-op closes typing so the next insertion has its own undo snapshot.
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
		if el, inside := e.insideElement(e.line, at); inside {
			at = el.Start
		}
		e.replaceRange(Caret{Line: e.line, Col: at}, Caret{Line: e.line, Col: e.col}, "")
		// Coalesce corrections with the typing burst they amend.
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
	// Word boundaries may fall inside an atomic element; deletion must include the whole element.
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

// Content invalidation must not close typing, or each keystroke becomes an undo step.
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

// Movement and editing share this boundary so typing, drag and soft-break affinity cannot drift.
func (e *Editor) endTyping() {
	e.dragging = false
	e.typing = false
	e.rowEndSet = false
	e.breakContinuation()
}

func (e *Editor) snapshot() {
	e.ensure()
	e.history.record(e.state())
}

// Prevent undo storage from growing with session lifetime.
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

// Known grapheme boundaries allow segmentation to resume here without rescanning the prefix.
func nextClusterBoundary(s string, at int) int {
	if at >= len(s) {
		return len(s)
	}
	for _, cluster := range text.Clusters(s[at:]) {
		return at + len(cluster)
	}
	return at
}

func isWord(cluster string) bool {
	r, _ := utf8.DecodeRuneInString(cluster)
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// InsertElement inserts an atomic text unit and returns it. Line breaks become spaces
// so its line-local range remains valid. An ordinary separator follows; a leading
// separator prevents joining the preceding grapheme. Empty bodies insert nothing.
// Identities are never reused; exhaustion panics before insertion.
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
	where, _ := slices.BinarySearchFunc(e.marks, mark, func(a, b text.Mark) int {
		return a.Start - b.Start
	})
	e.marks = slices.Insert(e.marks, where, mark)
	if !changedText {
		e.contentChanged()
	}
	return e.elementOf(mark)
}

// Joining depends on neighboring text: two valid regional-indicator labels can
// form one flag. An element must start at its own grapheme boundary.
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
		// Remove the trailing separator inserted with the element so removing a chip leaves no gap.
		end := el.End
		if line := e.lines[el.Line]; end < len(line) && line[end] == ' ' {
			end++
		}
		e.replaceRange(Caret{Line: el.Line, Col: el.Start}, Caret{Line: el.Line, Col: end}, "")
		return true
	}
	return false
}

// Element endpoints are valid caret positions; treating the start as interior would
// prevent insertion before the element.
func (e *Editor) insideElement(line, col int) (Element, bool) {
	at := e.offsetOf(Caret{Line: line, Col: col})
	for _, m := range e.marks {
		if m.Within(at) {
			return e.elementOf(m), true
		}
	}
	return Element{}, false
}

func (e *Editor) snapElement(line, col int, forward bool) int {
	// The zero editor has a conceptual empty line; a read projection must not initialize storage.
	if len(e.lines) == 0 {
		return 0
	}
	line = min(max(line, 0), len(e.lines)-1)
	col = clusterPosition(e.lines[line], col, forward)
	return e.snapElementBoundary(line, col, forward)
}

// These callers already own a grapheme boundary, so only element snapping is needed.
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

// Marks use pre-edit byte offsets; shift them before changing line storage.
func (e *Editor) edited(edit text.Edit) {
	e.marks = edit.Shift(e.marks, e.byteLength())
}

// Offset shifts cannot prove that an element still spans whole graphemes. Validate
// against the edited text once for every mutation.
func (e *Editor) settleMarks() {
	e.marks = slices.DeleteFunc(e.marks, func(m text.Mark) bool {
		return !e.spansWholeClusters(m)
	})
}

func (e *Editor) spansWholeClusters(m text.Mark) bool {
	start, end := e.caretAt(m.Start), e.caretAt(m.End)
	if start.Line != end.Line {
		return false
	}
	line := e.lines[start.Line]
	return clusterPosition(line, start.Col, true) == start.Col &&
		clusterPosition(line, end.Col, true) == end.Col
}

func (e *Editor) byteLength() int {
	e.ensure()
	n := len(e.lines) - 1
	for _, line := range e.lines {
		n += len(line)
	}
	return n
}

// Caret coordinates refer to the old lines, so shift marks before replacing storage.
func (e *Editor) removed(start, end Caret, s string) {
	e.edited(text.Edit{Start: e.offsetOf(start), End: e.offsetOf(end), Text: s})
}

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

// Handle answers keys, pastes and pointer selection, reporting whether it consumed
// the event. A left-button press starts a drag. A release ends the drag only when it
// names the left button or leaves the button unspecified, even outside the editor.
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

// A soft break has two visual positions at one byte offset; pointer affinity chooses the earlier row.
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

// Preserve the desired column across short rows so repeated vertical movement cannot drift left.
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
	presentation := editorPresentation{width: width, gutter: gutter, first: first, visible: frame.Visible()}
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
	e.blurred = !has
	if !has {
		e.endTyping()
		e.matcher.Clear()
	}
}

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

// Zero wrap width keeps one-line fields on one visual row for drawing and hit testing.
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

// Equal replacement text can still remove an element identity; an empty edit must not.
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

// Settle elements before the cursor so it cannot snap over an element the edit destroyed.
// An identity replacement leaves revision, history and layout unchanged.
func (e *Editor) finishReplacement(at Caret) {
	defer e.revealCursor()
	e.settleMarks()
	e.selecting = false
	e.line = min(max(at.Line, 0), len(e.lines)-1)
	e.col = e.snapElement(e.line, at.Col, true)
	e.wantColumn = -1
}

// The caller has proved a semantic change and taken any required undo snapshot.
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

// Cut copies and removes the selection only after clipboard acceptance. Clipboard
// callbacks that change content or the selected range revoke the removal, so an
// older copy cannot authorize deleting newer text.
func (e *Editor) Cut() bool {
	e.endTyping()
	start, end, selected := e.Selection()
	if !selected || e.Clipboard == nil {
		return false
	}
	revision := e.revision
	if !e.Clipboard.Copy(e.textBetween(start, end)) {
		return false
	}
	currentStart, currentEnd, current := e.Selection()
	if e.revision != revision || !current || currentStart != start || currentEnd != end {
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

// Shift-selection and ordinary movement share actions; only ordinary movement releases the anchor.
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

// Spans returns visual column runs covered by two carets. Width and columns include
// [Editor.Gutter]. Cursor placement and selection painting share the same wrap so
// graphemes cannot acquire different screen positions.
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

// At maps field-local coordinates to a caret, accounting for scrolling and gutter.
// It shares the cursor and selection wrap so hit testing agrees with rendering.
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
		last := rows[len(rows)-1]
		return Caret{Line: last.line, Col: last.end}, true
	}
	r := rows[index]
	line := e.lines[r.line]
	col := r.start + text.OffsetAt(line[r.start:r.end], x)
	return Caret{Line: r.line, Col: e.snapElement(r.line, col, true)}, true
}

func (e *Editor) handleMouse(ev input.Mouse, presented editorPresentation) bool {
	switch ev.Action {
	case input.MouseCancel:
		e.dragging = false
		return false
	case input.MouseUp:
		if !ev.Releases(input.ButtonLeft) {
			return false
		}
		wasDragging := e.dragging
		e.dragging = false
		return wasDragging
	case input.MouseDown:
		e.dragging = false
		if ev.Button != input.ButtonLeft || !ev.Pos.In(presented.visible) {
			return false
		}
	case input.MouseDrag:
		if !e.dragging || !ev.Drags(input.ButtonLeft) {
			return false
		}
	default:
		return false
	}
	point, ok := presented.textPoint(ev.Pos)
	if !ok {
		return false
	}
	at, ok := e.at(point.X, point.Y, presented.width)
	if !ok {
		return false
	}
	if ev.Action == input.MouseDown {
		e.endTyping()
		e.anchor, e.selecting = at, true
		e.rowEnd, e.rowEndSet = at, e.pastRowEnd(point.X, point.Y, presented.width)
		e.dragging = true
	}
	e.line, e.col, e.wantColumn = at.Line, at.Col, -1
	e.revealCursor()
	return true
}

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

func (e *Editor) prefersRowEnd() bool {
	return e.rowEndSet && e.rowEnd == Caret{Line: e.line, Col: e.col}
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
	Copy:           func(e *Editor) { e.Copy() },
	Cut:            func(e *Editor) { e.Cut() },
	Paste:          func(e *Editor) { e.Paste() },
}

// RowSpan is a visual column run. Row is relative to the whole wrapped text so
// scrolling does not change its coordinates.
type RowSpan struct {
	Row        int
	Col, Width int
}
