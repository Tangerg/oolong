package headless

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Field is one thing a [Form] collects.
//
// It draws itself, including what it is asking for and what was wrong with the answer.
// That is not the division this package usually makes, and the reason is that a field
// is generic over what it holds: a renderer that drew every kind of field would have to
// name every kind, and there is no way to write down "a select of something". So the
// look travels the other way, as a [Look] the form hands down.
type Field interface {
	Focusable
	layout.Measurer

	// Prompt is what the field is asking for.
	Prompt() string
	// Validate checks what has been entered, remembers what was wrong with it, and
	// reports that. It is called when the field loses the keyboard and again when the
	// form is submitted.
	Validate() error
	// Error is what Validate last found, or nil when it found nothing.
	Error() error
}

// Look is how a widget here draws itself, for the few that draw themselves at all.
//
// Most of this package draws nothing: a list calls back to whoever knows what a row
// looks like, and that is what makes the ring above it optional. The exceptions are
// the widgets whose drawing cannot be handed out — a field is generic over what it
// holds, so nothing above could name every kind of one; an editor lays a selection
// over text it alone knows the shape of; a completion picks out the characters a
// query matched. They take this, and there is one of it rather than a style field
// per part, keeping one coherent appearance value for the whole field.
//
// A field is given one by the form it is in, and a form is given one by whatever
// appearance layer dressed it. A single field is a [Form] with one field in it: that is
// a widget like any other, it goes wherever a widget goes, and it is the whole of the
// wiring.
//
// The zero value draws in the terminal's own colours with no marks beside a choice,
// which is legible and plain, and is what a widget nobody dressed gets.
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
	v.Text(x, 0, text.Truncate(label, layout.Remaining(w, x), "…"), style)
}

// dressed is a field of this package's, which takes its look from the form it is in.
// One written elsewhere takes it from wherever its author likes, and a form does not
// reach into it.
type dressed interface{ dress(l Look) }

// field is what every field in this package has in common: what it draws itself with,
// whether it has the keyboard, and what was wrong with the answer.
//
// The dressing is unexported on purpose. A field of this package's takes its look from
// the form it is in; one written elsewhere takes it from wherever its author likes, and
// a form does not reach into it.
type field struct {
	look    Look
	problem error
	// held says the field has had the keyboard, so that losing it means something. A
	// field is told where it stands as soon as there is a form to say so, and checking
	// an answer nobody has given yet is how a form greets somebody with three errors.
	held    bool
	blurred bool
	// presentation is the label offset and inner size from the last complete root
	// frame. Field drawing stages it; pointer handling never observes a partial tree.
	presentation Snapshot[fieldPresentation]
}

// Error is what checking the answer last found.
func (f *field) Error() error { return f.problem }

// dress is how a form hands its look down.
func (f *field) dress(l Look) { f.look = l }

// rows is what the label and the error cost, on top of whatever the field itself needs.
func (f *field) rows(label string) int {
	extra := 0
	if label != "" {
		extra++
	}
	if f.problem != nil {
		extra++
	}
	return extra
}

// frame draws the label and the error, and returns the room left in between for the
// field itself.
func (f *field) frame(v Frame, label string) Frame {
	w, h := v.Size()
	if w <= 0 || h <= 0 {
		f.presentation.Stage(v, fieldPresentation{})
		return v.Sub(grid.Rect(0, 0, 0, 0))
	}
	top, bottom := 0, 0
	if label != "" {
		v.Text(0, 0, text.Truncate(label, w, "…"), f.look.Label)
		top = 1
	}
	if f.problem != nil && h > top {
		bottom = 1
		v.Text(0, h-1, text.Truncate(f.problem.Error(), w, "…"), f.look.Danger)
	}
	inner := v.Sub(grid.Rect(0, top, w, max(h-top-bottom, 0)))
	innerW, innerH := inner.Size()
	f.presentation.Stage(v, fieldPresentation{top: top, inner: image.Pt(innerW, innerH)})
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
	presented := f.presentation.Value()
	if presented.inner.Y <= 0 {
		// Nothing has been drawn, so there is no frame for the press to be about.
		return ev, false
	}
	ev.Pos.Y -= presented.top
	return ev, ev.Pos.Y >= 0 && ev.Pos.Y < presented.inner.Y
}

type fieldPresentation struct {
	top   int
	inner image.Point
}

// check records what is wrong with an answer and reports it.
func (f *field) check(err error) error {
	f.problem = err
	return err
}

// leaving takes note of where the keyboard went, and reports whether the answer is now
// worth checking.
//
// On the way out, and only from a field that had the keyboard to begin with. A form
// that greeted somebody with a column of complaints about answers they have not given
// yet would be a form nobody finishes.
func (f *field) leaving(has bool) bool {
	f.blurred = !has
	if has {
		f.held = true
		return false
	}
	return f.held
}

// Form is a set of fields, one of which has the keyboard.
//
// It is a [Container] of them with two things added: an answer is checked when the
// keyboard leaves the field it was given to, and the whole set is checked when the form
// is submitted. Everything else — which field has the keyboard, what tab does, which
// field a click landed in — is the container's, because it is the same question there
// as anywhere else.
//
// The zero Form has no fields and answers nothing.
type Form struct {
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

	body    Container
	problem error
	matcher keymap.Matcher
	blurred bool
}

// NewForm constructs a form from fields in keyboard order.
func NewForm(fields ...Field) *Form {
	f := &Form{}
	f.Set(fields...)
	return f
}

// Set replaces the fields in keyboard order. The form copies the collection, releases
// a focused field that was removed, and settles focus on its replacement. A field is
// still caller-owned; only the ordered collection belongs to the form. A nil field is
// a programmer error and panics here rather than in a later draw or submission.
func (f *Form) Set(fields ...Field) {
	if f == nil {
		return
	}
	checkFields(fields)
	f.fields = own(f.fields, fields)
	f.rebuild()
}

// Add appends fields and returns the form, so a form can be built in one expression.
func (f *Form) Add(fields ...Field) *Form {
	if f == nil {
		return nil
	}
	checkFields(fields)
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
	f.arrange()
	current, _ := f.body.Focused().(Field)
	return current
}

// Submit checks every answer and reports whether the form is complete.
//
// Every field is checked and not just the first that fails, because a form that
// reported its problems one at a time would make somebody submit four times to find
// out there were four.
func (f *Form) Submit() bool {
	f.arrange()
	f.problem = nil
	ok := true
	for _, field := range f.fields {
		if field.Validate() != nil {
			ok = false
		}
	}
	if !ok {
		return false
	}
	if f.Check != nil {
		if err := f.Check(); err != nil {
			f.problem = err
			return false
		}
	}
	if f.Done != nil {
		f.Done()
	}
	return true
}

// Cancel abandons the form.
func (f *Form) Cancel() {
	if f.GaveUp != nil {
		f.GaveUp()
	}
}

// Measure is how tall the fields are altogether, which is what a form in a measured
// slot asks for.
func (f *Form) Measure(across int) int {
	f.arrange()
	return f.body.Measure(across)
}

// Draw dresses the fields with Look and lays them out down the region.
func (f *Form) Draw(v Frame) {
	f.DrawWith(v, f.Look)
}

// DrawWith draws the form with look for this projection only.
//
// An appearance wrapper uses this instead of changing [Form.Look] before drawing.
// The fields are restored to the form's own look before DrawWith returns, so two
// appearances can project the same controller without the last one silently
// becoming its configuration.
func (f *Form) DrawWith(v Frame, look Look) {
	f.arrange()
	f.dress(look)
	defer func() { f.dress(f.Look) }()
	f.body.Draw(v)
}

// Handle gives the event to the field with the keyboard, then to the form itself.
func (f *Form) Handle(ev input.Event) bool {
	f.arrange()
	if f.body.Handle(ev) {
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
		return false
	}
	return true
}

// Focus takes the keyboard, or gives it up, and passes the news to the field that has
// it.
func (f *Form) Focus(has bool) {
	if !has {
		f.matcher.Clear()
	}
	f.blurred = !has
	f.arrange()
	f.body.Focus(has)
}

// arrange hands current behavior configuration to the already-settled container.
// Collection changes go through Set or Add; Draw never acquires or releases
// semantic ownership as a side effect.
func (f *Form) arrange() {
	f.body.Axis = layout.Down
	f.body.Gap = f.Gap
	f.body.Keys = f.keys()
}

// dress hands a projection's appearance to the built-in fields. An external field
// owns its own drawing and is deliberately left alone.
func (f *Form) dress(look Look) {
	for _, field := range f.fields {
		if takes, ok := field.(dressed); ok {
			takes.dress(look)
		}
	}
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

// keys is the map to read through, standing in the default for a caller who set none.
func (f *Form) keys() *keymap.Map {
	if f.Keys != nil {
		return f.Keys
	}
	return formKeys()
}
