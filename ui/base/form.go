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

	Prompt() string
	// Validate checks what has been entered, remembers what was wrong with it, and
	// reports that. It is called when the field loses the keyboard and again when the
	// form is submitted. Built-in fields return ErrValidationSuperseded if a callback
	// starts a newer Validate or Reply or replaces the checked answer. An obsolete
	// check cannot publish Error.
	Validate() error
	// Validation returns a read-only receipt for the last completed answer check, or
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
type SubmissionID struct{ identity *byte }

const (
	ActionCancel         scene.ActionID = "cancel"
	ActionReadSubmission scene.ActionID = "read-submission"
)

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

var (
	// ErrSubmissionAborted reports that the submit callback exited without accepting work.
	ErrSubmissionAborted = errors.New("base: submission callback did not return")
	// ErrSubmissionStale reports a request superseded by a newer submission or field relation.
	ErrSubmissionStale = errors.New("base: submission is no longer current")
	// ErrNotCancellable reports that no active attempt accepts a cancellation request.
	ErrNotCancellable = errors.New("base: submission cannot be cancelled")
)

// FormConfig keeps Submit's accepted effects independent of an accompanying error. Started
// keeps the attempt pending until Settle, even when a later admission step failed.
// PaintField must be a pure appearance projection.
type FormConfig struct {
	Key, Title string
	Fields     []Field
	Keys       *keymap.Map
	Gap        int
	Check      func() error
	Submit     func(SubmissionID) (scene.ActionState, error)
	// Cancel receives a cancellation request for the pending attempt with that ID; it is never called
	// while the form is idle. It does not settle the attempt; the attempt's owner still delivers Settle.
	// Leaving an unsubmitted form is application navigation: the Cancel key is left unhandled when no
	// attempt is pending, so an enclosing handler receives it.
	Cancel     func(SubmissionID) error
	PaintField func(scene.Frame, Field)
}

// Form owns validation and attempt settlement. Scene owns its mounted fields,
// keyboard target and pointer route; a form stores no parallel focus ring.
// Form must not be copied after first use; its collaborators share one live identity.
type Form struct {
	noCopy         noCopy
	key, title     string
	fields         []Field
	Keys           *keymap.Map
	Gap            int
	check          func() error
	submit         func(SubmissionID) (scene.ActionState, error)
	cancel         func(SubmissionID) error
	paintField     func(scene.Frame, Field)
	group          *scene.Group
	status         scene.Item
	problem        func() error
	cancelFeedback requestFeedback
	attempt        Submission
	submitted      []fieldVersion
	matcher        keymap.Matcher
}

// NewForm constructs one form owner. It panics for nil or duplicate fields, or fields already owned by an incompatible scene relation.
func NewForm(config FormConfig) *Form {
	f := &Form{key: strings.Clone(config.Key), title: strings.Clone(config.Title), Keys: config.Keys, Gap: config.Gap, check: config.Check, submit: config.Submit, cancel: config.Cancel, paintField: config.PaintField}
	f.Set(config.Fields...)
	return f
}

func (f *Form) Element() scene.Element { f.ensure(); return f.group.Element() }

func (f *Form) ensure() {
	if f.group != nil {
		return
	}
	submit := scene.NewAction(scene.ActionConfig[struct{}, SubmissionID]{ID: scene.ActionSubmit, Label: "Submit", Run: func(context.Context, struct{}) (scene.ActionResult[SubmissionID], error) { return f.Submit() }})
	f.status = scene.Item{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		if problem := f.Error(); problem != nil {
			w, _ := frame.Size()
			frame.Text(0, 0, text.Truncate(problem.Error(), w, ""), grid.Style{})
		}
	}, Semantics: func() scene.Semantic {
		value := ""
		if problem := f.Error(); problem != nil {
			value = problem.Error()
		}
		return scene.Semantic{Role: scene.RoleStatus, Value: value}
	}})}
	f.group = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{f.status},
		Element: scene.ElementConfig{Key: f.key, Measure: rowsMeasure(f.HeightForWidth), Handle: f.handleEvent, Semantics: func() scene.Semantic {
			state := scene.SemanticState(0)
			if f.attempt.State == SubmissionPending {
				state |= scene.StateBusy
			}
			if f.Error() != nil {
				state |= scene.StateInvalid
			}
			return scene.Semantic{Role: scene.RoleForm, Name: f.title, State: state}
		}, Actions: []scene.ActionBinding{submit.Binding(), f.submissionAction(), f.cancelAction()}},
		Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
			flow, slots, extra := f.fieldLayout(items)
			rects := flow.Rects(image.Pt(space.X, layout.Remaining(space.Y, extra)), slots)
			return append(rects, grid.Area(0, layout.Remaining(space.Y, extra), space.X, extra))
		},
	})
}

// Set preflights the entire field relation before replacing it. Input begun for
// the previous questions cannot submit their replacements. It panics for nil or
// duplicate fields, an invalid attachment relation, or mutation during presentation or observation.
func (f *Form) Set(fields ...Field) {
	f.ensure()
	requireMutable(f.group.Element())
	items := make([]scene.Item, 0, len(fields)+1)
	for _, field := range fields {
		if field == nil {
			panic("base: nil form field")
		}
		item := scene.Item{Element: field.Element(), Size: layout.Measured(1, 0)}
		if f.paintField != nil {
			item.Paint = func(frame scene.Frame) { f.paintField(frame, field) }
		}
		items = append(items, item)
	}
	items = append(items, f.status)
	if err := f.group.CheckSet(items...); err != nil {
		panic(err)
	}
	f.matcher.Reset(func() {
		f.fields = own(f.fields, fields)
		f.attempt = Submission{}
		f.submitted = nil
		f.problem = nil
		f.cancelFeedback.clear()
		if err := f.group.Set(items...); err != nil {
			panic(err)
		}
	})
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
func (f *Form) Error() error {
	if f.attempt.Err != nil {
		return f.attempt.Err
	}
	if f.cancelFeedback.err != nil {
		return f.cancelFeedback.err
	}
	if f.problem == nil {
		return nil
	}
	return readProjection(f.group.Element(), f.problem)
}

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
	return f.SubmitWith(nil)
}

// SubmitWith collects and validates each field through collect before submitting.
// A successful collector must leave a current Validation receipt on that field;
// a collection error stops immediately. A nil collector validates all fields.
// Changing the field relation or starting another attempt supersedes this call.
func (f *Form) SubmitWith(collect func(Field) error) (scene.ActionResult[SubmissionID], error) {
	requireMutable(f.Element())
	id := SubmissionID{identity: new(byte)}
	f.problem = nil
	f.cancelFeedback.clear()
	f.attempt = Submission{ID: id, State: SubmissionIdle}
	f.submitted = nil
	// Validation may exit without returning after the previous attempt was revoked.
	f.Element().Invalidate()
	versions, validationErr := f.validateSubmission(id, collect)
	if validationErr != nil {
		if f.attempt.ID == id {
			f.Element().Invalidate()
		}
		return scene.ActionResult[SubmissionID]{}, validationErr
	}
	f.submitted = versions
	state, err := f.start(id)
	if state != scene.Started && state != scene.Applied && state != scene.Unchanged {
		err = errors.Join(err, errors.New("base: submission callback returned an invalid action state"))
		if f.attempt.ID == id {
			f.complete(SubmissionOutcome{Err: err})
		}
		return scene.ActionResult[SubmissionID]{}, err
	}
	result := scene.ActionResult[SubmissionID]{State: state, Value: id}
	if f.attempt.ID != id {
		return result, errors.Join(err, ErrSubmissionStale)
	}
	if state == scene.Started {
		f.attempt.State, f.attempt.Err = SubmissionPending, err
		f.Element().Invalidate()
	} else {
		f.complete(SubmissionOutcome{Err: err})
	}
	return result, err
}

// The callback decides admission. Only work it actually accepted may be settled
// later; an abnormal exit propagates after recording failure on its own attempt.
func (f *Form) start(id SubmissionID) (scene.ActionState, error) {
	if f.submit == nil {
		return scene.Applied, nil
	}
	returned := false
	defer func() {
		if !returned && f.attempt.ID == id {
			f.complete(SubmissionOutcome{Err: ErrSubmissionAborted})
		}
	}()
	state, err := f.submit(id)
	returned = true
	return state, err
}

func (f *Form) currentValidations(validations []func() bool) bool {
	return readProjection(f.group.Element(), func() bool {
		for _, current := range validations {
			if current == nil || !current() {
				return false
			}
		}
		return true
	})
}

// Settle accepts only the active attempt. Success describes the submitted snapshot;
// Current reports whether it still describes the draft now visible to the user.
func (f *Form) Settle(id SubmissionID, outcome SubmissionOutcome) bool {
	requireMutable(f.Element())
	if f.attempt.ID != id || f.attempt.State != SubmissionPending {
		return false
	}
	f.complete(outcome)
	return true
}

func (f *Form) complete(outcome SubmissionOutcome) {
	state := SubmissionSucceeded
	if outcome.Cancelled {
		state = SubmissionCancelled
	} else if outcome.Err != nil {
		state = SubmissionFailed
	}
	f.attempt.State, f.attempt.Err = state, outcome.Err
	f.problem = nil
	f.cancelFeedback.clear()
	f.Element().Invalidate()
}

// Submission returns the current attempt, with Current computed from the exact submitted field revisions.
func (f *Form) Submission() Submission {
	result := f.attempt
	result.Current = result.State != SubmissionIdle
	if len(f.submitted) > 0 {
		f.group.Element().ReadOnly(func() {
			for _, field := range f.submitted {
				if field.field.Revision() != field.revision {
					result.Current = false
					break
				}
			}
		})
	}
	return result
}

// Cancel requests cancellation of the pending attempt. It does not settle the attempt; the worker or application must deliver Settle.
func (f *Form) Cancel() error {
	requireMutable(f.Element())
	if !f.canCancel() {
		return ErrNotCancellable
	}
	return f.cancel(f.attempt.ID)
}

func (f *Form) canCancel() bool { return f.cancel != nil && f.attempt.State == SubmissionPending }

func (f *Form) submissionAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[struct{}, Submission]{
		ID: ActionReadSubmission, Label: "Read submission",
		Run: func(context.Context, struct{}) (scene.ActionResult[Submission], error) {
			return scene.ActionResult[Submission]{Value: f.Submission()}, nil
		},
	}).Binding()
}

func (f *Form) cancelAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[SubmissionID, Submission]{
		ID: ActionCancel, Label: "Cancel submission",
		Availability: func(id SubmissionID) scene.Availability {
			if !f.canCancel() || id != f.attempt.ID {
				return scene.Availability{Disabled: true, Reason: "submission is not current or cancellable"}
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, id SubmissionID) (scene.ActionResult[Submission], error) {
			err := f.Cancel()
			state := scene.Unchanged
			if f.attempt.ID != id || f.attempt.State != SubmissionPending {
				state = scene.Applied
			} else if err == nil {
				state = scene.Started
			}
			return scene.ActionResult[Submission]{State: state, Value: f.Submission()}, err
		},
	}).Binding()
}

func (f *Form) HeightForWidth(width int) int {
	if f.group == nil {
		return 0
	}
	flow, slots, extra := f.fieldLayout(f.group.Items())
	return layout.Sum(flow.Wanted(width, slots), extra)
}

// The status is the final child and occupies its own row without an inter-field gap.
func (f *Form) fieldLayout(items []scene.Item) (layout.Flow, []layout.Slot, int) {
	fields := items[:len(items)-1]
	slots := make([]layout.Slot, len(fields))
	for i, field := range fields {
		slots[i] = layout.Slot{Size: field.Size, Of: field.Element}
	}
	extra := 0
	if f.Error() != nil {
		extra = 1
	}
	return layout.Flow{Axis: layout.Down, Gap: f.Gap}, slots, extra
}

func (f *Form) handleEvent(event scene.Event) scene.EventResult {
	key, ok := event.Input().(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	keyboard := f.Element().KeyboardLease()
	_, handled := f.matcher.Handle(f.keys(), key, keyboard.Valid, func(action keymap.Action) bool {
		switch action {
		case FocusNext:
			return keyboard.FocusNext(false)
		case FocusPrev:
			return keyboard.FocusNext(true)
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
		if !f.canCancel() {
			return false
		}
		id := f.attempt.ID
		turn := f.cancelFeedback.begin()
		err := f.Cancel()
		if f.attempt.ID == id && f.attempt.State == SubmissionPending && f.cancelFeedback.finish(turn, err) {
			f.Element().Invalidate()
		}
	default:
		return false
	}
	return true
}

func (f *Form) keys() *keymap.Map {
	if f.Keys != nil {
		return f.Keys
	}
	return formKeys()
}

func (f *Form) checkSubmissionField(field Field, collect func(Field) error) error {
	if collect == nil {
		return field.Validate()
	}
	if err := collect(field); err != nil {
		return err
	}
	return readProjection(f.group.Element(), field.Error)
}

func (f *Form) fieldProblem() error {
	for _, field := range f.fields {
		if err := field.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (f *Form) validateSubmission(id SubmissionID, collect func(Field) error) ([]fieldVersion, error) {
	var validations []func() bool
	var versions []fieldVersion
	var firstInvalid Field
	var problem error
	for _, field := range f.fields {
		err := f.checkSubmissionField(field, collect)
		if f.attempt.ID != id {
			return nil, ErrSubmissionStale
		}
		if err != nil && (collect != nil || errors.Is(err, ErrValidationSuperseded)) {
			return nil, err
		}
		if err != nil && firstInvalid == nil {
			firstInvalid, problem = field, err
		}
		f.group.Element().ReadOnly(func() {
			validations = append(validations, field.Validation())
			versions = append(versions, fieldVersion{field: field, revision: field.Revision()})
		})
	}
	if !f.currentValidations(validations) {
		return nil, ErrValidationSuperseded
	}
	if firstInvalid != nil {
		f.problem = f.fieldProblem
		_ = firstInvalid.Element().RequestFocusFirst()
		return nil, problem
	}
	if err := f.checkDraft(id, validations); err != nil {
		return nil, err
	}
	return versions, nil
}

func (f *Form) checkDraft(id SubmissionID, validations []func() bool) error {
	if f.check == nil {
		return nil
	}
	err := f.check()
	if f.attempt.ID != id {
		return ErrSubmissionStale
	}
	if !f.currentValidations(validations) {
		return ErrValidationSuperseded
	}
	if err != nil {
		f.problem = func() error {
			if f.currentValidations(validations) {
				return err
			}
			return nil
		}
	}
	return err
}

func (f *Form) Title() string { return f.title }

// SetTitle changes the semantic name and invalidates the same form identity.
// It panics if the scene is being presented or observed.
func (f *Form) SetTitle(title string) {
	requireMutable(f.Element())
	f.title = strings.Clone(title)
	f.Element().Invalidate()
}
