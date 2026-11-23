package base

import (
	"context"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/setting"
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

	session    *EditSession
	local      EditSession
	registered bool
	updates    editorViewUpdates
	caret      Caret
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

	matcher           keymap.Matcher
	clipboardFeedback requestFeedback

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
	// MaxRows caps text rows while keeping the cursor visible. Zero is unlimited;
	// a paste failure adds its own feedback row outside this content limit.
	MaxRows    int
	SingleLine bool
	// Mask presents every character as itself, for a password, as
	// [EditSessionConfig.Mask] does for the editor's own document.
	Mask string
	Look Look
	// Keys of nil uses [DefaultEditorKeys].
	Keys *keymap.Map
	// Clipboard of nil disables clipboard actions.
	Clipboard Clipboard
	// Gutter input belongs to the containing component; its text is excluded from
	// selection.
	Gutter      RowGutter
	CursorStyle grid.CursorStyle
}

// NewEditor panics for negative MaxRows, an invalid CursorStyle.Shape, an invisible
// or joining Mask, or a Session combined with Initial, SingleLine or Mask, which
// would supply competing document owners.
func NewEditor(config EditorConfig) *Editor {
	requireNonNegative("editor MaxRows", config.MaxRows)
	if err := config.CursorStyle.Shape.Validate(); err != nil {
		panic(err)
	}
	if config.Session != nil && (config.Initial != "" || config.SingleLine || config.Mask != "") {
		panic("base: editor Session cannot be combined with Initial, SingleLine or Mask")
	}
	e := &Editor{session: config.Session, placeholder: strings.Clone(config.Placeholder), maxRows: config.MaxRows, look: config.Look, keys: keyBinding{current: config.Keys}, clipboard: config.Clipboard, gutter: config.Gutter, cursorStyle: config.CursorStyle}
	if e.session == nil {
		e.local.initialize(EditSessionConfig{Initial: config.Initial, SingleLine: config.SingleLine, Mask: config.Mask})
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

// Cursor is where the next edit happens.
func (e *Editor) Cursor() Caret { return e.caret }

// SetCursor moves the cursor to at, clamped into the text and snapped backward to a
// grapheme and element boundary.
func (e *Editor) SetCursor(at Caret) {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	defer e.revealCursor()
	e.ensure()
	e.endTyping()
	e.caret.Line = min(max(at.Line, 0), len(e.document().lines)-1)
	current := e.document().lines[e.caret.Line]
	e.caret.Byte = e.document().snapElement(e.caret.Line, min(max(at.Byte, 0), len(current)), false)
	e.desiredColumn = editorColumn{}
}

func (e *Editor) oneLine() bool { return e.document().oneLine() }

func (e *Editor) ensure() {
	e.connect()
	e.document().ensure()
	e.caret.Line = min(max(e.caret.Line, 0), len(e.document().lines)-1)
	e.caret.Byte = min(max(e.caret.Byte, 0), len(e.document().lines[e.caret.Line]))
}

// Movement and editing share this boundary so typing, drag and soft-break affinity cannot drift.
func (e *Editor) endTyping() {
	e.document().endTyping(e)
	e.prepareEdit(false)
}

func (e *Editor) Placeholder() string { return e.placeholder }

func (e *Editor) SetPlaceholder(value string) {
	setting.Assign(e.element, &e.placeholder, strings.Clone(value))
}

func (e *Editor) Look() Look { return e.look }

func (e *Editor) SetLook(value Look) {
	setting.Assign(e.element, &e.look, value)
}

func (e *Editor) MaxRows() int { return e.maxRows }

// SetMaxRows limits text rows; operation feedback has its own row. It panics for
// a negative limit. Zero leaves text height uncapped.
func (e *Editor) SetMaxRows(value int) {
	requireNonNegative("editor MaxRows", value)
	setting.Assign(e.element, &e.maxRows, value)
}

func (e *Editor) Clipboard() Clipboard { return e.clipboard }

func (e *Editor) SetClipboard(value Clipboard) {
	setting.Replace(e.element, &e.clipboard, value)
}

func (e *Editor) Gutter() RowGutter { return e.gutter }

func (e *Editor) SetGutter(value RowGutter) {
	setting.Replace(e.element, &e.gutter, value)
}

func (e *Editor) CursorStyle() grid.CursorStyle { return e.cursorStyle }

// SetCursorStyle panics for an unknown cursor shape before changing presentation.
func (e *Editor) SetCursorStyle(value grid.CursorStyle) {
	element := e.element
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
	e.keys.set(e.element, &e.matcher, keys)
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

func (e *Editor) Element() scene.Element {
	if e.element.IsZero() {
		e.element = scene.NewElement(scene.ElementConfig{
			Measure: layout.MeasureRows(e.HeightForWidth), Paint: e.Draw,
			Handle: func(event scene.Event) scene.EventResult { return partEvent(event, e.Handle, &e.geometry) }, Focusable: func() bool { return true }, FocusChanged: e.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleTextBox, Name: e.placeholder, Value: e.semanticValue(), Sensitive: e.Mask() != "", ContentRevision: e.Revision()}, e.ClipboardError())
			}, Actions: []scene.ActionBinding{e.fillAction()},
		})
	}
	e.scroll.element = e.element
	e.keys.attach(e.element, &e.matcher)
	return e.element
}

// A synchronous edit subscriber may replace the answer or change its masking policy.
func (e *Editor) fillAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[string, string]{
		ID: scene.ActionFill, Label: "Fill", Run: func(_ context.Context, value string) (scene.ActionResult[string], error) {
			masked := e.Mask() != ""
			before := e.Revision()
			e.SetText(value)
			result := scene.ActionResult[string]{}
			if e.Revision() != before {
				result.State = scene.Applied
			}
			if !masked {
				result.Value = e.semanticValue()
			}
			return result, nil
		},
	}).Binding()
}

func (e *Editor) semanticValue() string {
	if e.Mask() != "" {
		return ""
	}
	return e.Text()
}
