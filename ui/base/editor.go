package base

import (
	"image"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Editor is one view of an EditSession, with its own caret, selection and navigation.
// The zero value is ready. An Editor must not be copied after first use.
type Editor struct {
	element  scene.Element
	geometry eventGeometry
	noCopy   noCopy

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

	session          *EditSession
	local            EditSession
	registered       bool
	updates          editorViewUpdates
	line, byteOffset int
	// Short rows and atomic elements must not replace the intended visual column.
	desiredColumn editorColumn

	// The cursor is the other endpoint, so movement cannot leave a stale selection end.
	anchor    Caret
	selecting bool
	dragging  bool

	// Soft breaks share a byte offset with two visual rows. Clicks may prefer the earlier
	// row; endTyping clears this affinity for subsequent movement and edits.
	rowEnd    Caret
	rowEndSet bool

	kills        editorKillRing
	continuation editorContinuation
	yank         editorYank

	matcher  keymap.Matcher
	pasteErr error

	scroll Scroll
	// Scroll owns request consumption and cancellation; drawing only resolves its visual row.
	cursorReveal *scrollRange
	layout       editorLayout
	presentation scene.Snapshot[editorPresentation]
}

// EditorConfig chooses the document owner. Initial is used only when Session is
// absent; supplying both is a construction error rather than an arbitration rule.
type EditorConfig struct {
	Session     *EditSession
	Initial     string
	Placeholder string
	MaxRows     int
	SingleLine  bool
}

// NewEditor constructs one view. It panics if Session is combined with Initial or SingleLine, which would supply competing document owners.
func NewEditor(config EditorConfig) *Editor {
	if config.Session != nil && (config.Initial != "" || config.SingleLine) {
		panic("base: editor Session cannot be combined with Initial or SingleLine")
	}
	e := &Editor{session: config.Session, Placeholder: strings.Clone(config.Placeholder), MaxRows: config.MaxRows}
	if e.session == nil {
		e.local.initialize(EditSessionConfig{Initial: config.Initial, SingleLine: config.SingleLine})
		e.finishReplacement(e.local.end(), true)
	}
	e.connect()
	return e
}

func (e *Editor) document() *EditSession {
	if e.session != nil {
		return e.session
	}
	return &e.local
}

// Session returns the document owner used by this view. Its subscriptions observe completed document transactions.
func (e *Editor) Session() *EditSession { e.connect(); return e.document() }

func (e *Editor) connect() {
	if e.registered {
		return
	}
	e.registered = true
	e.document().attach(e)
}

type editorPresentation struct {
	content                    editorProjectionKey
	width, gutter, first, left int
	visible                    image.Rectangle
}

type editorColumn struct {
	cells int
	valid bool
}

func (c editorColumn) resolve(current int) int {
	if c.valid {
		return c.cells
	}
	return current
}

func (p editorPresentation) textPoint(point image.Point) (image.Point, bool) {
	if p.width <= 0 || point.X < p.gutter {
		return image.Point{}, false
	}
	point.X = layout.Relative(point.X, p.gutter)
	return point, true
}

func (e *Editor) Text() string { return e.document().Text() }

// Revision advances once per semantic text or element change, including undo and redo.
// Cursor, selection, scrolling and no-op edits leave it unchanged.
// Compare tokens only within this editor; they have no persisted or numeric meaning.
func (e *Editor) Revision() uint64 { return e.document().revision }

// SingleLine reports whether this field was explicitly configured to hold one line.
// A non-empty [Editor.Mask] also makes the effective field one-line.
func (e *Editor) SingleLine() bool { return e.document().singleLine }

// SetSingleLine changes the explicit one-line setting. Enabling flattens line breaks,
// retains element identities and translates the cursor as one semantic change.
// It clears history so undo cannot restore invalid multi-line content. A non-empty
// mask still requires one-line storage when the explicit setting is disabled.
func (e *Editor) SetSingleLine(enabled bool) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.connect()
	e.document().setPolicy(e, enabled, e.Mask())
}

// Mask reports the shared session's display mask. Empty means text is shown.
func (e *Editor) Mask() string { return e.document().mask }

// SetMask changes how every view of this document draws and redacts its text.
// A non-empty mask also requires single-line storage for every writer, including
// Reset. It panics for invisible or joining masks, or during scene projection.
func (e *Editor) SetMask(mask string) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.connect()
	e.document().setPolicy(e, e.SingleLine(), mask)
}

func (e *Editor) SetText(value string) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	e.document().replace(e, editReplacement{text: value, whole: true}, e.finishReplacement)
}

func (e *Editor) Empty() bool {
	lines := e.document().lines
	return len(lines) == 0 || len(lines) == 1 && lines[0] == ""
}

func (e *Editor) Clear() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	e.document().replace(e, editReplacement{end: e.document().end()}, e.finishReplacement)
}

// Cursor returns the logical line and byte offset.
func (e *Editor) Cursor() (line, byteOffset int) { return e.line, e.byteOffset }

// SetCursor clamps the logical line and byte offset, snapping backward to a grapheme boundary.
func (e *Editor) SetCursor(line, byteOffset int) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.line = min(max(line, 0), len(e.document().lines)-1)
	current := e.document().lines[e.line]
	byteOffset = min(max(byteOffset, 0), len(current))
	e.byteOffset = e.document().snapElement(e.line, byteOffset, false)
	e.desiredColumn = editorColumn{}
}

// Insert inserts text at the cursor as one undo step, splitting lines at newlines.
func (e *Editor) Insert(s string) { e.insert(s, false) }

func (e *Editor) insert(value string, typed bool) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if value == "" {
		if typed {
			e.document().endTyping(e)
		} else {
			e.endTyping()
		}
		return
	}
	e.ensure()
	start, end := e.replacementRange()
	history := recordEdit
	if typed {
		history = coalesceEdit
	}
	e.document().replace(e, editReplacement{start: start, end: end, text: value, history: history}, e.finishReplacement)
}

// Replace swaps the byte range [start, end) of the line the cursor is on for s, and
// leaves the cursor after what was put in. The range is clamped to the line and
// expanded to whole grapheme clusters; a terminal cursor cannot address half of one.
//
// It is one edit rather than a delete and an insert so that it is one step to undo:
// accepting a completion is one thing the user did, and taking it back should not
// take two. A token never spans lines, which is why the range does not either.
func (e *Editor) Replace(start, end int, value string) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	e.document().replace(e, editReplacement{
		start: Caret{Line: e.line, Byte: start}, end: Caret{Line: e.line, Byte: end}, text: value, expandClusters: true,
	}, e.finishReplacement)
}

func (e *Editor) oneLine() bool { return e.document().oneLine() }

func (e *Editor) InsertRune(r rune) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	// InsertRune cannot create logical lines; Printable preserves tabs but not line breaks.
	e.insert(text.Printable(string(r)), true)
}

// Newline splits the line at the cursor, and does nothing at all in a field that holds
// one line.
func (e *Editor) Newline() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if e.oneLine() {
		e.endTyping()
		return
	}
	e.Insert("\n")
}

// DeleteBack removes the cluster before the cursor, or joins this line to the one
// above when the cursor is at the start of a line.
func (e *Editor) DeleteBack() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	if e.byteOffset > 0 {
		at := text.PrevCluster(e.document().lines[e.line], e.byteOffset)
		if element, inside := e.document().insideElement(e.line, at); inside {
			at = element.Start
		}
		e.document().replace(e, editReplacement{
			start: Caret{Line: e.line, Byte: at}, end: Caret{Line: e.line, Byte: e.byteOffset}, history: coalesceEdit,
		}, e.finishReplacement)
		return
	}
	if e.line == 0 {
		e.breakContinuation()
		return
	}
	above := e.document().lines[e.line-1]
	e.document().replace(e, editReplacement{
		start: Caret{Line: e.line - 1, Byte: len(above)}, end: Caret{Line: e.line},
	}, e.finishReplacement)
}

// DeleteForward removes the cluster after the cursor, or joins the line below.
func (e *Editor) DeleteForward() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	current := e.document().lines[e.line]
	if e.byteOffset < len(current) {
		at := nextClusterBoundary(current, e.byteOffset)
		if element, inside := e.ElementAt(e.line, e.byteOffset); inside {
			at = element.End
		}
		e.document().replace(e, editReplacement{
			start: Caret{Line: e.line, Byte: e.byteOffset}, end: Caret{Line: e.line, Byte: at},
		}, e.finishReplacement)
		return
	}
	if e.line == len(e.document().lines)-1 {
		e.endTyping()
		return
	}
	e.document().replace(e, editReplacement{
		start: Caret{Line: e.line, Byte: len(current)}, end: Caret{Line: e.line + 1},
	}, e.finishReplacement)
}

func (e *Editor) DeleteWordBack() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	end := Caret{Line: e.line, Byte: e.byteOffset}
	start := end
	if start.Byte > 0 {
		at := wordStart(e.document().lines[e.line], e.byteOffset)
		start.Byte = e.document().snapElementBoundary(e.line, at, false)
	} else if start.Line > 0 {
		start.Line--
		start.Byte = len(e.document().lines[start.Line])
	}
	e.killRange(start, end, true)
}

// KillToEnd cuts from the cursor to the end of the line, keeping what it cut.
// At the end of a line, the line break itself belongs to the kill.
func (e *Editor) KillToEnd() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	start := Caret{Line: e.line, Byte: e.byteOffset}
	end := Caret{Line: e.line, Byte: len(e.document().lines[e.line])}
	if start == end && end.Line+1 < len(e.document().lines) {
		end = Caret{Line: end.Line + 1}
	}
	e.killRange(start, end, false)
}

// KillToStart cuts from the start of the line to the cursor.
func (e *Editor) KillToStart() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	e.killRange(Caret{Line: e.line}, Caret{Line: e.line, Byte: e.byteOffset}, true)
}

func (e *Editor) killRange(start, end Caret, prepend bool) {
	join := e.continuation == editorContinuationKill
	if start == end {
		e.endTyping()
		return
	}
	killed := e.document().textBetween(start, end)
	e.document().replace(e, editReplacement{start: start, end: end}, func(at Caret, changed bool) {
		e.rememberKill(killed, prepend, join)
		e.finishReplacement(at, changed)
	})
}

// Yank puts back the most recently killed text.
func (e *Editor) Yank() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	killed, ok := e.kills.newest()
	if !ok {
		e.endTyping()
		return
	}
	e.ensure()
	start, end := Caret{Line: e.line, Byte: e.byteOffset}, Caret{Line: e.line, Byte: e.byteOffset}
	if first, last, selected := e.Selection(); selected {
		start, end = first, last
	}
	e.applyYank(start, end, killed, 0)
}

// YankPop replaces the immediately preceding yank with the next older kill, cycling
// through the bounded ring. Any intervening edit, movement, or selection ends the
// sequence and makes this a no-op.
func (e *Editor) YankPop() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	if e.continuation != editorContinuationYank {
		e.endTyping()
		return
	}
	killed, next, ok := e.kills.older(e.yank.ring)
	if !ok {
		e.endTyping()
		return
	}
	e.applyYank(e.yank.start, e.yank.end, killed, next)
}

func (e *Editor) applyYank(start, end Caret, killed string, ring int) {
	e.document().replace(e, editReplacement{start: start, end: end, text: killed}, func(at Caret, changed bool) {
		e.finishReplacement(at, changed)
		e.yank = editorYank{start: start, end: at, ring: ring}
		e.continuation = editorContinuationYank
	})
}

func (e *Editor) MoveLeft() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset > 0 {
		e.byteOffset = e.document().snapElementBoundary(e.line, text.PrevCluster(e.document().lines[e.line], e.byteOffset), false)
		return
	}
	if e.line > 0 {
		e.line--
		e.byteOffset = len(e.document().lines[e.line])
	}
}

func (e *Editor) MoveRight() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset < len(e.document().lines[e.line]) {
		e.byteOffset = e.document().snapElementBoundary(e.line, nextClusterBoundary(e.document().lines[e.line], e.byteOffset), true)
		return
	}
	if e.line < len(e.document().lines)-1 {
		e.line++
		e.byteOffset = 0
	}
}

func (e *Editor) MoveWordLeft() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset == 0 {
		e.MoveLeft()
		return
	}
	e.byteOffset = e.document().snapElementBoundary(e.line, wordStart(e.document().lines[e.line], e.byteOffset), false)
}

func (e *Editor) MoveWordRight() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	if e.byteOffset == len(e.document().lines[e.line]) {
		e.MoveRight()
		return
	}
	e.byteOffset = e.document().snapElementBoundary(e.line, wordEnd(e.document().lines[e.line], e.byteOffset), true)
}

func (e *Editor) MoveLineStart() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	e.byteOffset = 0
}

func (e *Editor) MoveLineEnd() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.desiredColumn = editorColumn{}
	e.byteOffset = len(e.document().lines[e.line])
}

func (e *Editor) Undo() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.document().undo(e)
}

func (e *Editor) Redo() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.document().redo(e)
}

func (e *Editor) ensure() {
	e.connect()
	e.document().ensure()
	e.line = min(max(e.line, 0), len(e.document().lines)-1)
	e.byteOffset = min(max(e.byteOffset, 0), len(e.document().lines[e.line]))
}

// Movement and editing share this boundary so typing, drag and soft-break affinity cannot drift.
func (e *Editor) endTyping() {
	e.document().endTyping(e)
	e.prepareEdit(false)
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
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	start, end := e.replacementRange()
	return e.document().insertElement(e, start, end, kind, body, e.finishReplacement)
}

// Elements is every element in the text, in the order they appear. The slice is a
// copy: a caller cannot move an element by writing to it.
func (e *Editor) Elements() []Element {
	out := make([]Element, 0, len(e.document().marks))
	for _, m := range e.document().marks {
		out = append(out, e.document().elementOf(m))
	}
	return out
}

// ElementAt is the element covering a position, and whether there is one. The end is
// exclusive, so the position just after an element is outside it. Out-of-range
// positions return false.
func (e *Editor) ElementAt(line, byteOffset int) (Element, bool) {
	lines := e.document().lines
	if line < 0 || line >= len(lines) || byteOffset < 0 || byteOffset >= len(lines[line]) {
		return Element{}, false
	}
	at := offsetInLines(lines, Caret{Line: line, Byte: byteOffset})
	for _, m := range e.document().marks {
		if m.Covers(at) {
			return e.document().elementOf(m), true
		}
	}
	return Element{}, false
}

// RemoveElement deletes an element's text and forgets it, reporting whether it was
// there to remove.
func (e *Editor) RemoveElement(id ElementID) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	start, end, found := e.document().elementRemoval(id)
	if !found {
		return false
	}
	e.document().replace(e, editReplacement{start: start, end: end}, e.finishReplacement)
	return true
}

// RetainedElementIDs returns identities reachable from the document or undo/redo
// history. Applications may release associated payloads only after they disappear
// from this set. The returned slice is owned by the caller.
func (e *Editor) RetainedElementIDs() []ElementID {
	return e.document().retainedElementIDs()
}

// ForgetHistory ends the lifetime of edits no longer available to Undo or Redo.
// Current document elements remain live.
func (e *Editor) ForgetHistory() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.endTyping()
	e.document().forgetHistory()
}

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

	matched, handled := e.matcher.Handle(e.keys(), key, keyboardReceipt(&e.element), do)
	if !matched {
		return typed(key)
	}
	return handled
}

// Do runs one of the field's actions by name, reporting whether it was one this field
// knows. See [Doer] for why a widget answers to a name at all.
func (e *Editor) Do(action keymap.Action) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.ensure()
	if movement := selectionMovement(action); movement != "" {
		e.Anchor()
		return e.move(movement)
	}
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

func selectionMovement(action keymap.Action) keymap.Action {
	switch action {
	case ExtendLeft:
		return MoveLeft
	case ExtendRight:
		return MoveRight
	case ExtendUp:
		return MoveUp
	case ExtendDown:
		return MoveDown
	case ExtendWordLeft:
		return MoveWordLeft
	case ExtendWordRight:
		return MoveWordRight
	case ExtendLineStart:
		return MoveLineStart
	case ExtendLineEnd:
		return MoveLineEnd
	default:
		return ""
	}
}

func (e *Editor) typed(key input.Key) bool {
	if key.Mods&^input.Shift != 0 {
		return false
	}
	// What the terminal says the key produced wins over the key's own code. The code is
	// the unshifted key on the physical keyboard: on a layout where the key beside "1"
	// produces "@", inserting the code would type "2".
	if key.Text != "" {
		e.insert(key.Text, true)
		return true
	}
	if r := key.FallbackRune(); r != 0 {
		e.InsertRune(r)
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
	e.rows(width)
	affinity := text.Forward
	if e.prefersRowEnd() {
		affinity = text.Backward
	}
	point := e.layout.projection.Caret(text.Position{Line: e.line, Byte: e.byteOffset, Affinity: affinity})
	return point.Y, point.X
}

func (e *Editor) offsetIn(width, row, column int) (line, byteOffset int) {
	e.rows(width)
	position := e.layout.projection.Hit(image.Pt(column, row))
	return position.Line, position.Byte
}

func (e *Editor) MoveUp() { e.moveRow(-1) }

func (e *Editor) MoveDown() { e.moveRow(1) }

// Preserve the desired column across short rows so repeated vertical movement cannot drift left.
func (e *Editor) moveRow(delta int) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	width := e.geometry.read(&e.presentation).width
	// Snapping toward the movement avoids returning to the same wrapped atomic row.
	forward := delta > 0
	if width <= 0 {
		// Nothing has been drawn yet, so there are no visual rows to move through.
		// Logical lines are the best available answer, but the horizontal coordinate
		// is still a terminal column rather than a byte offset. Copying the byte offset
		// to a UTF-8 line can put the cursor in the middle of a rune.
		column := text.ColumnOf(e.document().lines[e.line], e.byteOffset)
		column = e.desiredColumn.resolve(column)
		target := min(max(e.line+delta, 0), len(e.document().lines)-1)
		if target == e.line {
			return
		}
		e.line = target
		e.byteOffset = e.document().snapElementBoundary(target, text.OffsetAt(e.document().lines[target], column), forward)
		e.desiredColumn = editorColumn{cells: column, valid: true}
		return
	}
	row, column := e.rowAt(width)
	column = e.desiredColumn.resolve(column)
	target := row + delta
	if target < 0 || target >= len(e.rows(width)) {
		return
	}
	e.line, e.byteOffset = e.offsetIn(width, target, column)
	e.byteOffset = e.document().snapElementBoundary(e.line, e.byteOffset, forward)
	e.desiredColumn = editorColumn{cells: column, valid: true}
}

func (e *Editor) HeightForWidth(width int) int {
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

func (e *Editor) Draw(frame scene.Frame) {
	e.DrawWith(frame, e.Look)
}

// DrawWith paints one projection with look without changing the editor's configured
// appearance.
//
// Appearance components use this when an editor participates in a larger theme. The
// editor remains the single owner of its text, cursor and input configuration; drawing
// it through another look does not make that look its configuration.
// It panics unless the editor is mounted in the frame's scene outside read-only projection.
func (e *Editor) DrawWith(frame scene.Frame, look Look) {
	requireProjection(e.element, frame)
	presented := &e.presentation
	if e.oneLine() {
		e.lineView().draw(frame, look, presented)
		return
	}
	v := frame.View
	total, height := v.Size()
	if total <= 0 || height <= 0 {
		presented.Stage(frame, editorPresentation{})
		return
	}
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
	frame scene.Frame,
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
	projection := e.layout.projection
	presentation := editorPresentation{content: e.projectionKey(), width: width, gutter: gutter, first: first, visible: frame.Visible()}
	last := min(layout.Sum(first, height), len(rows))
	e.drawGutter(gutterView, projection, first, last)

	if e.Empty() && e.Placeholder != "" {
		view.Text(0, 0, text.Truncate(e.Placeholder, width, look.Ellipsis), look.Subtle)
		e.placeCursor(view, 0, 0)
		return presentation
	}

	visible := view.Visible()
	for y := visible.Min.Y; y < visible.Max.Y; y++ {
		index := layout.Sum(first, y)
		if index >= len(rows) {
			break
		}
		row, _ := projection.Row(index)
		text.Of(row.Text, look.Text).Draw(view, 0, y)
	}
	// The selection is laid over the text rather than drawn into it, so a run that
	// crosses a style boundary keeps whatever was underneath — and so that the rows
	// above did not have to be told which of them was selected.
	for _, span := range e.spansOfSelection(width) {
		y := span.Row - first
		if y < visible.Min.Y || y >= visible.Max.Y {
			continue
		}
		end := min(layout.Sum(span.Col, span.Width), visible.Max.X)
		for x := max(span.Col, visible.Min.X); x < end; x++ {
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
	return readProjection(e.Element(), func() int { return max(e.Gutter.Width(max(len(e.document().lines), 1)), 0) })
}

func (e *Editor) textWidth(total int) int {
	return layout.Remaining(total, e.gutterWidth())
}

func (e *Editor) drawGutter(view grid.View, projection *text.Layout, first, last int) {
	if e.Gutter == nil {
		return
	}
	rows := make([]text.Row, last-first)
	for i := range rows {
		row, _ := projection.Row(first + i)
		rows[i] = text.Row{Text: row.Text, Line: row.Line + 1, Joined: row.Joined}
	}
	e.Gutter.Draw(view, rows)
}

// Losing the keyboard ends transient input. Cursor painting reads scene focus directly.
func (e *Editor) focusChanged(has bool) {
	if !has {
		e.endTyping()
		e.matcher.Clear()
	}
}

func (e *Editor) placeCursor(v grid.View, x, y int) {
	if !e.element.Focused() {
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
	width := e.geometry.read(&e.presentation).width
	row, _ := e.rowAt(width)
	e.scroll.revealContent(len(e.rows(width)), row, row)
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
	lines := e.document().lines
	if len(lines) == 0 {
		lines = []string{""}
	}
	return e.layout.rowsFor(lines, width)
}

// A pointer may only address the text and display policy used by its arrival frame.
type editorProjectionKey struct {
	revision   uint64
	singleLine bool
	mask       string
}

func (e *Editor) projectionKey() editorProjectionKey {
	return editorProjectionKey{revision: e.Revision(), singleLine: e.oneLine(), mask: e.Mask()}
}

func (e *Editor) lineView() editorLineView {
	return editorLineView{
		editorProjectionKey: e.projectionKey(),
		value:               e.Text(), placeholder: e.Placeholder,
		cursorByte: e.byteOffset, anchorByte: e.anchor.Byte, selecting: e.selecting,
		focused: e.element.Focused(), left: e.geometry.read(&e.presentation).left,
		gutter: e.Gutter, cursorStyle: e.CursorStyle,
	}
}

func (e *Editor) shown() string {
	e.ensure()
	return e.lineView().shown()
}

func (e *Editor) lineAt(at int) int {
	if e.Mask() == "" {
		return at
	}
	line := e.document().lines[0]
	if at <= 0 {
		return 0
	}
	want := at / len(e.Mask())
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
	byteOffset := e.lineAt(text.OffsetAt(e.shown(), layout.Translate(x, e.geometry.read(&e.presentation).left)))
	return Caret{Byte: e.document().snapElement(0, byteOffset, true)}
}

// Anchor preserves an existing selection's origin so repeated extension actions
// can move the caret without moving the start of the gesture.
func (e *Editor) Anchor() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.breakContinuation()
	if !e.selecting {
		e.anchor, e.selecting = Caret{Line: e.line, Byte: e.byteOffset}, true
	}
}

func (e *Editor) SelectNone() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.dragging = false
	e.breakContinuation()
	e.selecting = false
}

func (e *Editor) SelectAll() {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.anchor, e.selecting = Caret{}, true
	e.line = len(e.document().lines) - 1
	e.byteOffset = len(e.document().lines[e.line])
	e.desiredColumn = editorColumn{}
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
	cursor := Caret{Line: e.line, Byte: e.byteOffset}
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
	return e.document().textBetween(start, end)
}

func (e *Editor) DeleteSelection() bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	start, end, ok := e.Selection()
	if !ok {
		return false
	}
	e.document().replace(e, editReplacement{start: start, end: end}, e.finishReplacement)
	return true
}

// Copy sends the selection to the clipboard. Masked sessions never export their
// underlying text. Like Paste and Cut, clipboard access is an owner-side effect.
func (e *Editor) Copy() bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	start, end, ok := e.Selection()
	return ok && e.copyRange(start, end)
}

func (e *Editor) copyRange(start, end Caret) bool {
	return e.Mask() == "" && e.Clipboard != nil && e.Clipboard.Copy(e.document().textBetween(start, end))
}

// Cut copies and removes the selection only after clipboard acceptance. Clipboard
// callbacks that change content or the selected range revoke the removal, so an
// older copy cannot authorize deleting newer text.
func (e *Editor) Cut() bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.endTyping()
	start, end, selected := e.Selection()
	if !selected {
		return false
	}
	revision := e.document().revision
	if !e.copyRange(start, end) {
		return false
	}
	currentStart, currentEnd, current := e.Selection()
	if e.document().revision != revision || !current || currentStart != start || currentEnd != end {
		return false
	}
	return e.DeleteSelection()
}

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
	if e.Mask() != "" {
		view := e.lineView()
		shown := view.shown()
		if from.Line != 0 || to.Line != 0 {
			return nil
		}
		start := text.ColumnOf(shown, view.shownAt(from.Byte))
		end := text.ColumnOf(shown, view.shownAt(to.Byte))
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
			lo = max(lo, from.Byte)
		}
		if r.line == to.Line {
			hi = min(hi, to.Byte)
		}
		if lo >= hi {
			continue
		}
		line := e.document().lines[r.line]
		column := text.ColumnOf(line[r.start:r.end], lo-r.start)
		end := text.ColumnOf(line[r.start:r.end], hi-r.start)
		out = append(out, RowSpan{Row: i, Col: column, Width: end - column})
	}
	return out
}

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
	e.rows(width)
	presented := e.geometry.read(&e.presentation)
	first := 0
	if presented.width == width {
		first = presented.first
	}
	position := e.layout.projection.Hit(image.Pt(x, layout.Sum(first, y)))
	return Caret{Line: position.Line, Byte: e.document().snapElement(position.Line, position.Byte, true)}, true
}

func (e *Editor) handleMouse(ev input.Mouse, presented editorPresentation) bool {
	e.beginViewUpdate()
	defer e.endViewUpdate()
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
	if presented.content != e.projectionKey() {
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
	e.line, e.byteOffset = at.Line, at.Byte
	e.desiredColumn = editorColumn{}
	e.revealCursor()
	return true
}

func (e *Editor) pastRowEnd(x, y, width int) bool {
	rows := e.rows(width)
	presented := e.geometry.read(&e.presentation)
	first := 0
	if presented.width == width {
		first = presented.first
	}
	index := layout.Sum(first, y)
	if index < 0 || index >= len(rows) || e.layout.lastOfLine(index) {
		return false
	}
	r := rows[index]
	return x >= text.Width(e.document().lines[r.line][r.start:r.end])
}

func (e *Editor) prefersRowEnd() bool {
	return e.rowEndSet && e.rowEnd == Caret{Line: e.line, Byte: e.byteOffset}
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
	Paste:          (*Editor).pasteFromInput,
}

// RowSpan is a visual column run. Row is relative to the whole wrapped text so
// scrolling does not change its coordinates.
type RowSpan struct {
	Row        int
	Col, Width int
}
