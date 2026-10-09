package headless

import (
	"errors"
	"image"
	"slices"

	"github.com/Tangerg/oolong/components/internal/focus"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// ErrValidationSuperseded reports a Validate or Reply whose callback started a
// newer validation or replaced the answer being checked. The interrupted call cannot
// publish or report success.
var ErrValidationSuperseded = errors.New("headless: validation superseded")

// ErrInvalidBinding reports a state owner that retained a value outside the field's
// domain after normalization. A display projection cannot authorize submitting it.
var ErrInvalidBinding = errors.New("headless: binding did not accept a valid answer")

// Field is one thing a [Form] collects.
//
// It draws itself, including what it is asking for and what was wrong with the answer.
// That is not the division this package usually makes, and the reason is that a field
// is generic over what it holds: a renderer that drew every kind of field would have to
// name every kind, and there is no way to write down "a select of something". So the
// look travels the other way, as a [Look] the form hands down.
type Field interface {
	Focusable
	HeightForWidth(width int) int

	// Prompt is what the field is asking for.
	Prompt() string
	// Validate checks what has been entered, remembers what was wrong with it, and
	// reports that. It is called when the field loses the keyboard and again when the
	// form is submitted. Built-in fields return ErrValidationSuperseded if a callback
	// starts a newer Validate or Reply or replaces the checked answer. An obsolete
	// check cannot publish Error.
	Validate() error
	// Validation returns a read-only receipt for the last successful validation, or
	// nil when none is current. The receipt reports false when another Validate or
	// Reply has begun or the checked answer is no longer current. Reading it must be pure; it cannot
	// validate again or advance the answer. Form retains it across later callbacks.
	Validation() func() bool
	// Error is what Validate last found, or nil when it found nothing.
	Error() error
}

// Look supplies semantic styles and choice marks to the default field, editor and
// completion renderers. Its zero value uses terminal colors. Custom choice rows and
// completion renderers receive the same roles while owning their own row layout.
type Look struct {
	// Text is the answer, Label what the field is asking for, and Subtle a placeholder
	// or a hint.
	Text, Label, Subtle grid.Style
	// Selection is the row under the keyboard, and Accent the answer that has been
	// given.
	Selection, Accent grid.Style
	// Danger is what is wrong with the answer.
	Danger grid.Style
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

// choice draws one option on a row: its mark, and its label in whatever is left.
//
// It is here rather than on the field because all three of the fields that offer a
// choice draw a row the same way, and the look is the only thing the drawing depends
// on. under is the row the keyboard is on, taken the choice that has been made — which
// are the same row in a [Select] and two different ones in a [MultiSelect].
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
type ThemedField interface{ DrawWith(frame Frame, look Look) }

// field is what every field in this package has in common: whether it has the
// keyboard, what was wrong with the answer, and the committed geometry used by
// pointer input. Appearance is deliberately absent; a Form passes it as a projection
// argument while drawing.
type field struct {
	checked    fieldCheck
	validation uint64
	// keyboard receives the parent's focus before answer or resolver callbacks.
	// Answer operations cannot refuse that notification or revoke its lifetime.
	keyboard focus.State
	// presentation is the label offset and visible answer area from the complete root
	// frame. Field drawing stages it; pointer handling never observes a partial tree.
	presentation Snapshot[fieldPresentation]
}

// Error is what checking the answer last found.
func (f *field) Error() error { return f.checked.err }

// Validation returns a receipt whose validity belongs to this field's checked answer.
func (f *field) Validation() func() bool {
	checked := f.checked
	if checked.err != nil || checked.current == nil || checked.validation != f.validation || !checked.current() {
		return nil
	}
	return func() bool { return checked.validation == f.validation && checked.current() }
}

type fieldCheck struct {
	err        error
	current    func() bool
	validation uint64
}

func (f *field) invalidateValidation() {
	f.validation++
	f.checked = fieldCheck{}
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

// frame draws the label and the error, and returns the room left in between for the
// field itself.
func (f *field) frame(v Frame, label string, look Look) Frame {
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

// within translates a pointer event into the field's own box and reports whether it
// landed in it.
//
// The label is a row of the field and is not part of what the field holds, so a press
// under it means a row lower than the position says. It is the same translation a
// container does for its children and for the same reason: a widget reasons in its own
// coordinates, and a position that arrived in anybody else's is one it cannot use.
func (f *field) within(ev input.Mouse) (input.Mouse, bool) {
	ev = f.local(ev)
	return ev, ev.Pos.In(f.presentation.Value().visible)
}

func (f *field) local(ev input.Mouse) input.Mouse {
	ev.Pos.Y -= f.presentation.Value().top
	return ev
}

type fieldPresentation struct {
	top     int
	visible image.Rectangle
}

func (f *field) validate(prepare func() error, check func() fieldCheck) error {
	f.validation++
	turn := f.validation
	var err error
	if prepare != nil {
		err = prepare()
	}
	if turn != f.validation || errors.Is(err, ErrValidationSuperseded) {
		return ErrValidationSuperseded
	}
	checked := fieldCheck{err: err, validation: turn}
	if err == nil {
		checked = check()
		checked.validation = turn
	}
	if turn != f.validation || errors.Is(checked.err, ErrValidationSuperseded) || checked.current != nil && !checked.current() {
		return ErrValidationSuperseded
	}
	f.checked = checked
	return checked.err
}

func (f *field) focus(has bool, forward func(bool), reconcile func(), validate func() error) {
	turn, leaving := f.keyboard.Receive(has)
	if forward != nil {
		forward(has)
	}
	if f.keyboard.Superseded(turn) {
		return
	}
	if reconcile != nil {
		reconcile()
	}
	if leaving && !f.keyboard.Superseded(turn) {
		_ = validate()
	}
}

// Form is a set of fields, one of which has the keyboard.
//
// It is a [Container] of them with two things added: an answer is checked when the
// keyboard leaves the field it was given to, and the whole set is checked when the form
// is submitted. Everything else — which field has the keyboard, what tab does, which
// field a click landed in — is the container's, because it is the same question there
// as anywhere else.
//
// The zero Form has no fields and answers nothing. A Form must not be copied after
// first use: its fields, focus, matcher and validation state are one mutable owner.
type Form struct {
	noCopy noCopy

	// fields are what the form collects, in the order the keyboard walks them. They
	// are private for the same reason Container's children are: replacing a focused
	// field has to release the old owner before the new one can receive input.
	fields []Field
	// Keys say which keystrokes submit and abandon the form, and which walk between
	// its fields. Nil reads through [DefaultFormKeys].
	Keys *keymap.Map
	// Look is how the fields draw themselves. It is handed to each of them as the form
	// draws, so changing it changes them.
	Look Look
	// Gap is how many blank rows go between one field and the next. Zero puts them
	// against each other, which is right for a short form and cramped for a long one.
	Gap int

	// Check is what is wrong with the answers taken together, and is asked only once
	// each of them is acceptable on its own. Nil means nothing is.
	Check func() error
	// Done is called when the form is submitted and everything checks out, and Given
	// up when it is abandoned. Either may be nil.
	Done, GaveUp func()

	body       Container
	problem    error
	submission uint64
	matcher    keymap.Matcher
}

// NewForm constructs a form from fields in keyboard order. A nil field is a programmer
// error and panics here, on the same terms as [Form.Set].
func NewForm(fields ...Field) *Form {
	f := &Form{}
	f.Set(fields...)
	return f
}

// Set replaces the fields in keyboard order. The form copies the collection, releases
// a focused field that was removed, and settles focus on its replacement. A field is
// still caller-owned; only the ordered collection belongs to the form. A nil field is
// a programmer error and panics here rather than in a later draw or submission.
// Replacing the collection revokes an active submission and clears its form error.
func (f *Form) Set(fields ...Field) {
	if f == nil {
		return
	}
	checkFields(fields)
	f.submission++
	f.problem = nil
	f.fields = own(f.fields, fields)
	f.rebuild()
}

// Add appends fields and returns the form, so a form can be built in one expression.
// It revokes an active submission and clears the previous form error.
func (f *Form) Add(fields ...Field) *Form {
	if f == nil || len(fields) == 0 {
		return f
	}
	checkFields(fields)
	f.submission++
	f.problem = nil
	f.fields = append(f.fields, fields...)
	f.fields = trim(f.fields)
	f.rebuild()
	return f
}

// Fields returns a copy of the fields in keyboard order.
func (f *Form) Fields() []Field {
	if f == nil {
		return nil
	}
	return slices.Clone(f.fields)
}

// Error is what checking the answers together last found.
func (f *Form) Error() error { return f.problem }

// Focused is the field with the keyboard, or nil.
func (f *Form) Focused() Field {
	current, _ := f.body.Focused().(Field)
	return current
}

// Submit checks every answer and reports whether the form is complete.
//
// Every field is checked and not just the first that fails, because a form that
// reported its problems one at a time would make somebody submit four times to find
// out there were four.
// Replacing or adding fields, cancelling, or starting a new submission from a check
// revokes the interrupted call, which returns false without publishing or calling Done.
// Every successful field receipt must remain current through later field and form
// checks. The form never retries an invalidated receipt or completes an unchecked answer.
func (f *Form) Submit() bool {
	f.submission++
	turn := f.submission
	f.problem = nil
	ok := true
	var validations []func() bool
	for _, field := range f.fields {
		err := field.Validate()
		if turn != f.submission {
			return false
		}
		if err != nil {
			ok = false
		}
		validations = append(validations, field.Validation())
	}
	if !ok || !currentValidations(validations) {
		return false
	}
	if f.Check != nil {
		err := f.Check()
		if turn != f.submission || !currentValidations(validations) {
			return false
		}
		if err != nil {
			f.problem = err
			return false
		}
	}
	if f.Done != nil {
		f.Done()
	}
	return true
}

func currentValidations(validations []func() bool) bool {
	for _, current := range validations {
		if current == nil || !current() {
			return false
		}
	}
	return true
}

// Cancel abandons the form.
func (f *Form) Cancel() {
	f.submission++
	if f.GaveUp != nil {
		f.GaveUp()
	}
}

// HeightForWidth is how tall the fields are altogether, which is what a form in a measured
// slot asks for.
func (f *Form) HeightForWidth(across int) int {
	extra := 0
	if f.problem != nil {
		extra = 1
	}
	return layout.Sum(extra, f.body.measureWith(across, f.flow()))
}

// Draw dresses the fields with Look and lays them out down the region.
func (f *Form) Draw(v Frame) {
	f.DrawWith(v, f.Look)
}

// DrawWith draws the form with look for this projection only.
//
// An appearance wrapper uses this instead of changing [Form.Look] before drawing.
// Neither the form nor its fields store look, so two appearances can project the same
// controller without the last one silently becoming its configuration.
func (f *Form) DrawWith(v Frame, look Look) {
	if f.problem != nil {
		w, h := v.Size()
		if h > 0 {
			v.Text(0, h-1, text.Truncate(f.problem.Error(), w, look.Ellipsis), look.Danger)
			v = v.Sub(grid.Area(0, 0, w, h-1))
		}
	}
	f.body.drawWith(v, f.flow(), func(frame Frame, child Widget) {
		if field, ok := child.(ThemedField); ok {
			field.DrawWith(frame, look)
			return
		}
		child.Draw(frame)
	})
}

// Handle gives the event to the field with the keyboard, then to the form itself.
func (f *Form) Handle(ev input.Event) bool {
	if f.body.handleChild(ev) {
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := f.matcher.Handle(f.keys(), key, f.Do)
	return handled
}

// Do runs one of the form's actions by name. See [Doer].
func (f *Form) Do(action keymap.Action) bool {
	switch action {
	case Submit:
		f.Submit()
	case Cancel:
		f.Cancel()
	default:
		return f.body.Do(action)
	}
	return true
}

// Focus takes the keyboard, or gives it up, and passes the news to the field that has
// it.
//
// Whether a form has the keyboard is the state of the container holding its fields,
// and this keeps no second copy of it: a repeated report is settled there, where
// changing children is settled too.
func (f *Form) Focus(has bool) {
	change := func() {
		f.body.Focus(has)
	}
	if has {
		change()
	} else {
		f.matcher.Reset(change)
	}
}

func (f *Form) flow() layout.Flow {
	return layout.Flow{Axis: layout.Down, Gap: f.Gap}
}

// rebuild is the only path from the form's field collection into its focus owner.
// It is a semantic operation and deliberately never runs from Draw.
func (f *Form) rebuild() {
	items := make([]Item, len(f.fields))
	for i, field := range f.fields {
		items[i] = Item{Size: layout.Measured(1, 0), Of: field}
	}
	f.body.Set(items...)
}

func checkFields(fields []Field) {
	for _, field := range fields {
		if field == nil {
			panic("headless: nil form field")
		}
	}
}

func (f *Form) keys() *keymap.Map {
	if f.Keys != nil {
		return f.Keys
	}
	return formKeys()
}
