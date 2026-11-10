package base

import (
	"errors"
	"image"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/focus"
)

// ErrValidationSuperseded reports a Validate or Reply whose callback started a
// newer validation, changed its rule, or replaced the answer being checked.
// The interrupted call cannot publish or report success.
var ErrValidationSuperseded = errors.New("base: validation superseded")

// ErrInvalidValue reports an answer outside the field's available values.
var ErrInvalidValue = errors.New("base: invalid field value")

// Field is one thing a [Form] collects.
//
// It draws itself, including what it is asking for and what was wrong with the answer.
// That is not the division this package usually makes, and the reason is that a field
// is generic over what it holds: a renderer that drew every kind of field would have to
// name every kind, and there is no way to write down "a select of something". So the
// look travels the other way, as a [Look] the form hands down.
type Field interface {
	Element() scene.Element
	Draw(frame scene.Frame)

	Prompt() string
	// Validate checks what has been entered, remembers what was wrong with it, and
	// reports that. It is called when the field loses the keyboard and again when the
	// form is submitted. Built-in fields return ErrValidationSuperseded if a callback
	// starts a newer Validate or Reply, invalidates the rule, or replaces the checked
	// answer. An obsolete check cannot publish Error.
	Validate() error
	// Validation returns a read-only receipt for the last completed answer check, or
	// nil when none is current. The receipt reports false when another Validate or
	// Reply has begun, its rule was invalidated, or the checked answer is no longer
	// current. Reading it must be pure; it cannot validate again or advance the
	// answer. Form retains it across later callbacks.
	Validation() func() bool
	Error() error
	// Revision identifies the complete answer snapshot. Owners may advance it
	// conservatively when replacing opaque values whose equality cannot be proved.
	Revision() uint64
}

// Look supplies semantic styles and choice marks to the default field, editor and
// completion renderers. Its zero value uses terminal colors. Custom choice rows and
// completion renderers receive the same roles while owning their own row layout.
type Look struct {
	Text, Label, Subtle grid.Style
	Selection, Accent   grid.Style
	Danger              grid.Style
	// Taken and Free are the marks beside a choice that has been made and one that is
	// still on offer. They are drawn in the same column, so they are the same width or
	// nothing beside them lines up.
	Taken, Free string
	// Ellipsis is the truncation marker. Empty clips without a marker.
	Ellipsis string
}

// mark is the mark for a choice, and how many columns the pair of them takes. The pair
// is measured rather than the one being drawn, so that a row with a mark and a row
// without line up.
func (l Look) mark(taken bool) (string, int) {
	width := max(text.Width(l.Taken), text.Width(l.Free))
	if width == 0 {
		return "", 0
	}
	if taken {
		return l.Taken, layout.Sum(width, 1)
	}
	return l.Free, layout.Sum(width, 1)
}

// Cursor focus and accepted choice are distinct in MultiSelect, so their appearances must remain separate.
func (l Look) choice(v grid.View, label string, under, taken bool) {
	w, _ := v.Size()
	style := l.Text
	if under {
		style = l.Selection
		v.Fill(v.Bounds(), style)
	}
	x := 0
	if mark, width := l.mark(taken); width > 0 {
		marked := style
		if taken {
			marked = style.Merge(l.Accent)
		}
		v.Text(x, 0, mark, marked)
		x = width
	}
	v.Text(x, 0, text.Truncate(label, layout.Remaining(w, x), l.Ellipsis), style)
}

// ThemedField optionally receives a Form look for one frame. Built-in and external
// fields use the same capability; the look must not replace stored configuration.
type ThemedField interface {
	DrawWith(frame scene.Frame, look Look)
}

// field is what every field in this package has in common: whether it has the
// keyboard, what was wrong with the answer, and the committed geometry used by
// pointer input. Appearance is deliberately absent; a Form passes it as a projection
// argument while drawing.
type field struct {
	key        string
	geometry   eventGeometry
	element    scene.Element
	checked    fieldCheck
	validation uint64
	// keyboard receives the parent's focus before answer or resolver callbacks.
	// Answer operations cannot refuse that notification or revoke its lifetime.
	keyboard focus.State
	// presentation is the label offset and visible answer area from the complete root
	// frame. Field drawing stages it; pointer handling never observes a partial tree.
	presentation scene.Snapshot[fieldPresentation]
}

// Error describes the current answer, not a replacement for the one that failed validation.
func (f *field) Error() error {
	c := f.checked
	if c.err == nil || c.validation != f.validation || c.current != nil && !c.current() {
		return nil
	}
	return c.err
}

// Validation returns a receipt whose validity belongs to this field's checked answer.
func (f *field) Validation() func() bool {
	checked := f.checked
	if !checked.validated || checked.validation != f.validation || !checked.current() {
		return nil
	}
	return func() bool { return checked.validation == f.validation && checked.current() }
}

type fieldCheck struct {
	validated  bool
	err        error
	current    func() bool
	validation uint64
}

func (f *field) invalidateValidation() {
	f.validation++
	f.checked = fieldCheck{}
}

// InvalidateValidation revokes results when a check's domain dependencies change
// without replacing the callback or answer. The dependency owner must call it
// before publishing that change; it does not run a check or advance Revision.
func (f *field) InvalidateValidation() {
	requireMutable(f.element)
	f.invalidateValidation()
	f.element.Invalidate()
}

func (f *field) rows(label string) int {
	extra := 0
	if label != "" {
		extra++
	}
	if f.Error() != nil {
		extra++
	}
	return extra
}

func (f *field) frame(v scene.Frame, label string, look Look) scene.Frame {
	requireProjection(f.element, v)
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		f.presentation.Stage(v, fieldPresentation{})
		return v.Sub(grid.Area(0, 0, 0, 0))
	}
	top, bottom := 0, 0
	if label != "" {
		v.Text(0, 0, text.Truncate(label, w, look.Ellipsis), look.Label)
		top = 1
	}
	if f.Error() != nil && h > top {
		bottom = 1
		v.Text(0, h-1, text.Truncate(f.Error().Error(), w, look.Ellipsis), look.Danger)
	}
	inner := v.Sub(grid.Area(0, top, w, max(h-top-bottom, 0)))
	f.presentation.Stage(v, fieldPresentation{top: top, visible: inner.Visible()})
	return inner
}

// The label occupies a row outside the value; translate pointer coordinates before value hit testing.
func (f *field) within(ev input.Mouse) (input.Mouse, bool) {
	ev = f.local(ev)
	return ev, ev.Pos.In(f.geometry.read(&f.presentation).visible)
}

func (f *field) local(ev input.Mouse) input.Mouse {
	ev.Pos.Y -= f.geometry.read(&f.presentation).top
	return ev
}

type fieldPresentation struct {
	top     int
	visible image.Rectangle
}

func (f *field) validate(prepare func() error, revision func() uint64, check func() error) error {
	requireMutable(f.element)
	f.validation++
	turn := f.validation
	// Revocation is observable even when a callback aborts or another request
	// makes the rest of this validation obsolete.
	f.element.Invalidate()
	var err error
	if prepare != nil {
		err = prepare()
	}
	if turn != f.validation || errors.Is(err, ErrValidationSuperseded) {
		return ErrValidationSuperseded
	}
	if errors.Is(err, ErrReplyUnavailable) {
		return err
	}
	version := revision()
	checked := fieldCheck{
		err: err, validated: err == nil, validation: turn,
		current: func() bool { return revision() == version },
	}
	if checked.validated {
		checked.err = check()
	}
	if turn != f.validation || errors.Is(checked.err, ErrValidationSuperseded) || !checked.current() {
		return ErrValidationSuperseded
	}
	f.checked = checked
	f.element.Invalidate()
	return checked.err
}

func (f *field) focus(has bool, forward func(bool), validate func() error) {
	turn, leaving := f.keyboard.Receive(has)
	if forward != nil {
		forward(has)
	}
	if f.keyboard.Superseded(turn) {
		return
	}
	if leaving {
		_ = validate()
	}
}
