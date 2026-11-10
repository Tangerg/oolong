package base

import (
	"context"
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
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
	// attempt is pending, so an enclosing handler receives it. A pending attempt that a newer
	// submission or a field replacement supersedes is cancelled through it too, since Settle can no
	// longer reach that attempt; the newer attempt owns the form's feedback, so that call's error is
	// not shown.
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
	keys           keyBinding
	gap            int
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

// NewForm panics for negative Gap, nil or duplicate fields, or fields already
// owned by an incompatible scene relation.
func NewForm(config FormConfig) *Form {
	requireNonNegative("form Gap", config.Gap)
	f := &Form{key: strings.Clone(config.Key), title: strings.Clone(config.Title), keys: keyBinding{current: config.Keys}, gap: config.Gap, check: config.Check, submit: config.Submit, cancel: config.Cancel, paintField: config.PaintField}
	f.Set(config.Fields...)
	return f
}

func (f *Form) Element() scene.Element {
	f.ensure()
	f.keys.attach(f.group.Element(), &f.matcher)
	return f.group.Element()
}

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
		Element: scene.ElementConfig{Key: f.key, Measure: scene.MeasureRows(f.HeightForWidth), Handle: f.handleEvent, Semantics: func() scene.Semantic {
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
	superseded := f.attempt
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
	f.revoke(superseded)
}

// revoke tells the owner of a pending attempt that a newer one superseded it.
// Settle can no longer reach that attempt, so the Cancel callback is the only
// notice its work receives. Its error has no attempt left to report into.
func (f *Form) revoke(superseded Submission) {
	if superseded.State != SubmissionPending || f.cancel == nil {
		return
	}
	_ = f.cancel(superseded.ID)
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
	return layout.Flow{Axis: layout.Down, Gap: f.gap}, slots, extra
}

func (f *Form) handleEvent(event scene.Event) scene.EventResult {
	key, ok := event.Input().(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	keyboard := f.Element().KeyboardLease()
	_, handled := f.matcher.Handle(f.resolvedKeys(), key, keyboard.Valid, func(action keymap.Action) bool {
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
	requireMutable(f.Element())
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

func (f *Form) resolvedKeys() *keymap.Map { return f.keys.resolve(formKeys) }

// KeysFor returns the sequences bound to action in the map this form handles keys
// with, including its defaults when no map is set, so help text can never describe
// bindings the form does not use.
func (f *Form) KeysFor(action keymap.Action) []input.Keys { return f.resolvedKeys().Keys(action) }

func (f *Form) Title() string { return f.title }

// SetTitle changes the semantic name and invalidates the same form identity.
// It panics if the scene is being presented or observed.
func (f *Form) SetTitle(title string) {
	requireMutable(f.Element())
	f.title = strings.Clone(title)
	f.Element().Invalidate()
}

func (f *Form) Gap() int { return f.gap }

// SetGap panics for a negative gap before changing layout or publishing a revision.
func (f *Form) SetGap(value int) {
	element := f.Element()
	requireMutable(element)
	requireNonNegative("form Gap", value)
	if f.gap == value {
		return
	}
	f.gap = value
	element.Invalidate()
}

func (f *Form) Keys() *keymap.Map { return f.keys.current }

func (f *Form) SetKeys(keys *keymap.Map) {
	f.keys.set(f.Element(), &f.matcher, keys)
}
