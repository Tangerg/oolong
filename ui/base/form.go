package base

import (
	"context"
	"errors"
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/focus"
)

// ErrValidationSuperseded reports a Validate or Reply whose callback started a
// newer validation or replaced the answer being checked. The interrupted call cannot
// publish or report success.
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
	HeightForWidth(width int) int

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
	Error() error
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

func (f *field) frame(v scene.Frame, label string, look Look) scene.Frame {
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

func (f *field) validate(prepare func() error, check func() fieldCheck) error {
	requireMutable(f.element)
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

// SubmissionID identifies one validated attempt, independently of the worker
// carrying it out. A later attempt revokes settlement of an earlier one.
type SubmissionID uint64

// SubmissionState distinguishes a pending attempt from its terminal outcome.
type SubmissionState uint8

// Submission states distinguish validation, active work and terminal outcomes.
const (
	SubmissionIdle SubmissionState = iota
	SubmissionPending
	SubmissionSucceeded
	SubmissionFailed
	SubmissionCancelled
)

// SubmissionOutcome settles one asynchronous attempt. Cancellation and failure do not imply successful submission.
type SubmissionOutcome struct {
	Err       error
	Cancelled bool
}

// Submission returns the current attempt, with Current computed from the exact submitted field revisions.
type Submission struct {
	ID      SubmissionID
	State   SubmissionState
	Err     error
	Current bool
}
type fieldVersion struct {
	field    Field
	revision uint64
}

// ErrSubmissionStale reports a request superseded by a newer submission or field relation.
var (
	ErrSubmissionStale = errors.New("base: submission is no longer current")
	// ErrNotCancellable reports that no active attempt accepts a cancellation request.
	ErrNotCancellable = errors.New("base: submission cannot be cancelled")
)

// FormConfig supplies one field relation and event-side submission policy. Submit must return its supplied ID; Started remains pending until Settle. PaintField is a pure optional appearance slot.
type FormConfig struct {
	Key, Title string
	Fields     []Field
	Keys       *keymap.Map
	Gap        int
	Check      func() error
	Submit     func(SubmissionID) (scene.ActionResult[SubmissionID], error)
	Cancel     func(SubmissionID) error
	PaintField func(scene.Frame, Field)
}

// Form owns validation and attempt settlement. Scene owns its mounted fields,
// keyboard target and pointer route; a form stores no parallel focus ring.
// Form must not be copied after first use; its collaborators share one live identity.
type Form struct {
	noCopy     noCopy
	key, title string
	fields     []Field
	Keys       *keymap.Map
	Gap        int
	check      func() error
	submit     func(SubmissionID) (scene.ActionResult[SubmissionID], error)
	cancel     func(SubmissionID) error
	paintField func(scene.Frame, Field)
	group      *scene.Group
	status     scene.Element
	problem    error
	sequence   SubmissionID
	attempt    Submission
	submitted  []fieldVersion
	matcher    keymap.Matcher
}

// NewForm constructs one form owner. It panics for nil or duplicate fields, or fields already owned by an incompatible scene relation.
func NewForm(config FormConfig) *Form {
	f := &Form{key: strings.Clone(config.Key), title: strings.Clone(config.Title), Keys: config.Keys, Gap: config.Gap, check: config.Check, submit: config.Submit, cancel: config.Cancel, paintField: config.PaintField}
	f.Set(config.Fields...)
	return f
}

// Element returns the form's single field group and semantic submission target.
func (f *Form) Element() scene.Element { f.ensure(); return f.group.Element() }

func (f *Form) ensure() {
	if f.group != nil {
		return
	}
	submit := scene.NewAction(scene.ActionConfig[struct{}, SubmissionID]{ID: scene.ActionSubmit, Label: "Submit", Run: func(context.Context, struct{}) (scene.ActionResult[SubmissionID], error) { return f.Submit() }})
	f.status = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		if f.problem != nil {
			w, _ := frame.Size()
			frame.Text(0, 0, text.Truncate(f.problem.Error(), w, ""), grid.Style{})
		}
	}, Semantics: func() scene.Semantic {
		value := ""
		if f.problem != nil {
			value = f.problem.Error()
		}
		return scene.Semantic{Role: scene.RoleStatus, Value: value}
	}})
	f.group = scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Key: f.key, Measure: rowsMeasure(f.HeightForWidth), Handle: f.handleEvent, Semantics: func() scene.Semantic {
			state := scene.SemanticState(0)
			if f.attempt.State == SubmissionPending {
				state |= scene.StateBusy
			}
			if f.problem != nil {
				state |= scene.StateInvalid
			}
			return scene.Semantic{Role: scene.RoleForm, Name: f.title, State: state}
		}, Actions: []scene.ActionBinding{submit.Binding()}},
		Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			extra := 0
			if f.problem != nil {
				extra = 1
			}
			slots := make([]layout.Slot, len(f.fields))
			for i, field := range f.fields {
				slots[i] = layout.Slot{Size: layout.Measured(1, 0), Of: field.Element()}
			}
			rects := (layout.Flow{Axis: layout.Down, Gap: f.Gap}).Rects(image.Pt(space.X, layout.Remaining(space.Y, extra)), slots)
			return append(rects, grid.Area(0, layout.Remaining(space.Y, extra), space.X, extra))
		},
	})
}

// Set preflights the entire field relation before replacing it. It panics for nil or duplicate fields, an invalid attachment relation, or mutation during presentation or observation.
func (f *Form) Set(fields ...Field) {
	checkFields(fields)
	f.ensure()
	requireMutable(f.group.Element())
	items := make([]scene.Item, 0, len(fields)+1)
	for _, field := range fields {
		items = append(items, scene.Item{Element: field.Element(), Size: layout.Measured(1, 0)})
	}
	items = append(items, scene.Item{Element: f.status, Size: layout.Measured(0, 0)})
	if err := f.group.CheckSet(items...); err != nil {
		panic(err)
	}
	for _, field := range fields {
		if f.paintField != nil {
			if err := field.Element().SetPaint(func(frame scene.Frame) { f.paintField(frame, field) }); err != nil && !errors.Is(err, scene.ErrMounted) {
				panic(err)
			}
		}
	}
	f.fields = own(f.fields, fields)
	f.sequence++
	f.attempt = Submission{}
	f.submitted = nil
	f.problem = nil
	if err := f.group.Set(items...); err != nil {
		panic(err)
	}
}

// Add appends fields using the same validation and ownership rules as Set.
func (f *Form) Add(fields ...Field) *Form {
	all := append(f.Fields(), fields...)
	f.Set(all...)
	return f
}

// Fields returns a detached ordered slice of the current fields.
func (f *Form) Fields() []Field { return slices.Clone(f.fields) }

// Error is the current field, form or submission problem exposed by the form.
func (f *Form) Error() error { return f.problem }

// Focused projects the scene's actual keyboard target onto the current field relation.
func (f *Form) Focused() Field {
	for _, field := range f.fields {
		if field.Element().FocusedWithin() {
			return field
		}
	}
	return nil
}

// Submit validates every field, captures their revisions and starts one identified attempt. Started is not completion. It panics during presentation or observation.
func (f *Form) Submit() (scene.ActionResult[SubmissionID], error) {
	requireMutable(f.Element())
	f.sequence++
	id := f.sequence
	f.problem = nil
	f.attempt = Submission{ID: id, State: SubmissionIdle}
	f.submitted = nil
	versions, validationErr := f.validateSubmission(id)
	if validationErr != nil {
		return scene.ActionResult[SubmissionID]{}, validationErr
	}
	f.submitted = versions
	f.attempt = Submission{ID: id, State: SubmissionPending}
	result := scene.ActionResult[SubmissionID]{State: scene.Applied, Value: id}
	var err error
	if f.submit != nil {
		result, err = f.submit(id)
	}
	if f.sequence != id {
		return scene.ActionResult[SubmissionID]{}, ErrSubmissionStale
	}
	if err != nil {
		f.Settle(id, SubmissionOutcome{Err: err})
		return scene.ActionResult[SubmissionID]{}, err
	}
	if result.Value != id || result.State != scene.Applied && result.State != scene.Started && result.State != scene.Unchanged {
		err = errors.New("base: submission callback returned an invalid attempt result")
		f.Settle(id, SubmissionOutcome{Err: err})
		return scene.ActionResult[SubmissionID]{}, err
	}
	if result.State != scene.Started {
		f.Settle(id, SubmissionOutcome{})
	}
	f.Element().Invalidate()
	return result, nil
}

func currentValidations(validations []func() bool) bool {
	for _, current := range validations {
		if current == nil || !current() {
			return false
		}
	}
	return true
}

// Settle accepts only the active attempt. Success describes the submitted snapshot;
// Current reports whether it still describes the draft now visible to the user.
func (f *Form) Settle(id SubmissionID, outcome SubmissionOutcome) bool {
	requireMutable(f.Element())
	if id == 0 || f.attempt.ID != id || f.attempt.State != SubmissionPending {
		return false
	}
	state := SubmissionSucceeded
	if outcome.Cancelled {
		state = SubmissionCancelled
	} else if outcome.Err != nil {
		state = SubmissionFailed
	}
	f.attempt.State, f.attempt.Err = state, outcome.Err
	f.problem = outcome.Err
	f.Element().Invalidate()
	return true
}

// Submission returns the current attempt, with Current computed from the exact submitted field revisions.
func (f *Form) Submission() Submission {
	result := f.attempt
	result.Current = result.ID != 0
	for _, field := range f.submitted {
		if field.field.Revision() != field.revision {
			result.Current = false
			break
		}
	}
	return result
}

// Cancel requests cancellation of the pending attempt. It does not settle the attempt; the worker or application must deliver Settle.
func (f *Form) Cancel() error {
	requireMutable(f.Element())
	if f.cancel == nil || f.attempt.State != SubmissionPending {
		return ErrNotCancellable
	}
	return f.cancel(f.attempt.ID)
}

// HeightForWidth measures fields, inter-field gaps and the current form problem without validating or changing submission state.
func (f *Form) HeightForWidth(width int) int {
	rows := 0
	for i, field := range f.fields {
		if i > 0 {
			rows = layout.Sum(rows, max(0, f.Gap))
		}
		rows = layout.Sum(rows, field.HeightForWidth(width))
	}
	if f.problem != nil {
		rows = layout.Sum(rows, 1)
	}
	return rows
}

func (f *Form) handleEvent(event scene.Event) scene.EventResult {
	key, ok := event.Input.(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	_, handled := f.matcher.Handle(f.keys(), key, func(action keymap.Action) bool {
		switch action {
		case FocusNext:
			return event.FocusNext(false)
		case FocusPrev:
			return event.FocusNext(true)
		default:
			return f.Do(action)
		}
	})
	return scene.EventResult{Consumed: handled}
}

// Do executes a named submit or cancel request through the same attempt owner.
func (f *Form) Do(action keymap.Action) bool {
	switch action {
	case Submit:
		_, _ = f.Submit()
	case Cancel:
		if err := f.Cancel(); err != nil {
			f.problem = err
			f.Element().Invalidate()
		}
	default:
		return false
	}
	return true
}

func checkFields(fields []Field) {
	seen := map[scene.Element]bool{}
	for _, field := range fields {
		if field == nil {
			panic("base: nil form field")
		}
		if seen[field.Element()] {
			panic("base: duplicate form field")
		}
		seen[field.Element()] = true
	}
}

func (f *Form) keys() *keymap.Map {
	if f.Keys != nil {
		return f.Keys
	}
	return formKeys()
}

func (f *Form) validateSubmission(id SubmissionID) ([]fieldVersion, error) {
	var validations []func() bool
	var versions []fieldVersion
	var firstInvalid Field
	for _, field := range f.fields {
		err := field.Validate()
		if f.sequence != id {
			return nil, ErrSubmissionStale
		}
		if err != nil && firstInvalid == nil {
			firstInvalid = field
			f.problem = err
		}
		validations = append(validations, field.Validation())
		versions = append(versions, fieldVersion{field: field, revision: field.Revision()})
	}
	if firstInvalid != nil {
		_ = firstInvalid.Element().RequestFocusFirst()
		f.Element().Invalidate()
		return nil, f.problem
	}
	if !currentValidations(validations) {
		return nil, ErrValidationSuperseded
	}
	if f.check != nil {
		err := f.check()
		if f.sequence != id || !currentValidations(validations) {
			return nil, ErrValidationSuperseded
		}
		if err != nil {
			f.problem = err
			f.Element().Invalidate()
			return nil, err
		}
	}

	return versions, nil
}

// Title is the semantic form name also read by kit appearance.
func (f *Form) Title() string { return f.title }

// SetTitle changes the semantic name and invalidates the same form identity.
// It panics if the scene is being presented or observed.
func (f *Form) SetTitle(title string) {
	requireMutable(f.Element())
	f.title = strings.Clone(title)
	f.Element().Invalidate()
}
