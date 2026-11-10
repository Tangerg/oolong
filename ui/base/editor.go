package base

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

// Editor is one view of an EditSession, with its own caret, selection and navigation.
// The zero value is ready. An Editor must not be copied after first use.
type Editor struct {
	element  scene.Element
	geometry eventGeometry
	noCopy   noCopy

	placeholder string
	look        Look
	keys        keyBinding
	clipboard   Clipboard
	maxRows     int
	gutter      RowGutter
	cursorStyle grid.CursorStyle

	session          *EditSession
	local            EditSession
	registered       bool
	updates          editorViewUpdates
	line, byteOffset int
	// Short rows and atomic elements must not replace the intended visual column.
	desiredColumn editorColumn

	// The cursor is the other endpoint, so movement cannot leave a stale selection end.
	anchor    Caret
	selection editorSelection

	// Soft breaks share a byte offset with two visual rows. A click past the end
	// of the earlier row places the caret there; endTyping clears this affinity
	// for subsequent movement and edits. Nil when no click asked for it.
	rowEnd *Caret

	kills        editorKillRing
	continuation editorContinuation

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
	// MaxRows of zero allows unlimited height; a positive limit scrolls to keep the
	// cursor visible.
	MaxRows    int
	SingleLine bool
	Look       Look
	// Keys of nil uses [DefaultEditorKeys].
	Keys *keymap.Map
	// Clipboard of nil disables clipboard actions.
	Clipboard Clipboard
	// Gutter input belongs to the containing component; its text is excluded from
	// selection.
	Gutter      RowGutter
	CursorStyle grid.CursorStyle
}

// NewEditor panics for negative MaxRows, an invalid CursorStyle.Shape, or a
// Session combined with Initial or SingleLine, which would supply competing
// document owners.
func NewEditor(config EditorConfig) *Editor {
	requireNonNegative("editor MaxRows", config.MaxRows)
	if err := config.CursorStyle.Shape.Validate(); err != nil {
		panic(err)
	}
	if config.Session != nil && (config.Initial != "" || config.SingleLine) {
		panic("base: editor Session cannot be combined with Initial or SingleLine")
	}
	e := &Editor{session: config.Session, placeholder: strings.Clone(config.Placeholder), maxRows: config.MaxRows, look: config.Look, keys: keyBinding{current: config.Keys}, clipboard: config.Clipboard, gutter: config.Gutter, cursorStyle: config.CursorStyle}
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

func (e *Editor) oneLine() bool { return e.document().oneLine() }

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

func (e *Editor) Placeholder() string { return e.placeholder }

func (e *Editor) SetPlaceholder(value string) {
	element := e.Element()
	requireMutable(element)
	if e.placeholder == value {
		return
	}
	e.placeholder = strings.Clone(value)
	element.Invalidate()
}

func (e *Editor) Look() Look { return e.look }

func (e *Editor) SetLook(value Look) {
	element := e.Element()
	requireMutable(element)
	if e.look == value {
		return
	}
	e.look = value
	element.Invalidate()
}

func (e *Editor) MaxRows() int { return e.maxRows }

// SetMaxRows panics for a negative limit. Zero leaves the measured height uncapped.
func (e *Editor) SetMaxRows(value int) {
	element := e.Element()
	requireMutable(element)
	requireNonNegative("editor MaxRows", value)
	if e.maxRows == value {
		return
	}
	e.maxRows = value
	element.Invalidate()
}

func (e *Editor) Clipboard() Clipboard { return e.clipboard }

func (e *Editor) SetClipboard(value Clipboard) {
	element := e.Element()
	requireMutable(element)
	e.clipboard = value
	element.Invalidate()
}

func (e *Editor) Gutter() RowGutter { return e.gutter }

func (e *Editor) SetGutter(value RowGutter) {
	element := e.Element()
	requireMutable(element)
	e.gutter = value
	element.Invalidate()
}

func (e *Editor) CursorStyle() grid.CursorStyle { return e.cursorStyle }

// SetCursorStyle panics for an unknown cursor shape before changing presentation.
func (e *Editor) SetCursorStyle(value grid.CursorStyle) {
	element := e.Element()
	requireMutable(element)
	if err := value.Shape.Validate(); err != nil {
		panic(err)
	}
	if e.cursorStyle == value {
		return
	}
	e.cursorStyle = value
	element.Invalidate()
}

func (e *Editor) Keys() *keymap.Map { return e.keys.current }

func (e *Editor) SetKeys(keys *keymap.Map) {
	e.keys.set(e.Element(), &e.matcher, keys)
}

// Caret is a position in an editor's text: a logical line, and a byte offset into it.
//
// It is not a [Point]. A transcript numbers visual rows, because everything it
// answers is about what is on the screen; an editor's text has lines of its own that
// wrapping turns into rows, and a position in the text has to survive the window
// changing width. The two coordinate spaces are different questions, and one type for
// both would let an answer to one be passed as an answer to the other.
type Caret struct{ Line, Byte int }

func (c Caret) Before(d Caret) bool {
	return c.Line < d.Line || (c.Line == d.Line && c.Byte < d.Byte)
}
