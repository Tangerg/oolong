package base_test

import (
	"errors"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

// bare is a look with the marks a choice needs and no colour, which is what a test can
// read off a surface.
func bare() base.Look { return base.Look{Taken: "x", Free: "-"} }

func selectWith[T comparable](options []base.Option[T]) *base.Select[T] {
	field := &base.Select[T]{Same: base.Equal[T]}
	field.SetOptions(options)
	return field
}

func multiWith[T comparable](options []base.Option[T]) *base.MultiSelect[T] {
	field := &base.MultiSelect[T]{Same: base.Equal[T]}
	field.SetOptions(options)
	return field
}

func typeInto(f interface{ Handle(event input.Event) bool }, s string) {
	for _, r := range s {
		f.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

type observedField struct {
	element scene.Element
	focused bool
	handled int
}

func (*observedField) Draw(scene.Frame) {}

func (f *observedField) Focus(has bool) { f.focused = has }

func (f *observedField) Element() scene.Element {
	if f.element.IsZero() {
		f.element = scene.NewElement(scene.ElementConfig{Paint: f.Draw, Measure: func(layout.Axis, int) int { return 1 }, Focusable: func() bool { return true }, FocusChanged: f.Focus, Handle: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: f.Handle(event.Input)} }})
	}
	return f.element
}
func (*observedField) Revision() uint64          { return 0 }
func (*observedField) HeightForWidth(int) int    { return 1 }
func (*observedField) Prompt() string            { return "" }
func (*observedField) Validate() error           { return nil }
func (*observedField) Validation() func() bool   { return func() bool { return true } }
func (*observedField) Error() error              { return nil }
func (f *observedField) Handle(input.Event) bool { f.handled++; return true }

func TestReplacingFormFieldsTransfersKeyboardOwnership(t *testing.T) {
	old, next := new(observedField), new(observedField)
	fields := []base.Field{old}
	form := base.NewForm(base.FormConfig{Fields: fields})
	root := newTestRoot(form)
	fields[0] = next

	if got := form.Fields()[0]; got != old {
		t.Fatal("form retained the caller's field slice")
	}
	form.Set(next)
	root.root.FlushNotifications()
	if old.focused || !next.focused {
		t.Fatalf("focus after Set: old=%v next=%v", old.focused, next.focused)
	}
	newTestRoot(form).Handle(input.FocusIn{})
	if old.handled != 0 || next.handled != 1 {
		t.Fatalf("event deliveries after Set: old=%d next=%d", old.handled, next.handled)
	}

	snapshot := form.Fields()
	snapshot[0] = old
	if got := form.Fields()[0]; got != next {
		t.Fatal("Fields exposed the form's owned collection")
	}
}

func TestFormRejectsANilFieldAtTheCollectionBoundary(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewForm accepted a nil field")
		}
	}()
	base.NewForm(base.FormConfig{Fields: []base.Field{nil}})
}

func TestAFieldPutsWhatWasTypedWhereTheCallerKeepsIt(t *testing.T) {
	// A field does not own what it collects. Somebody asked for a name because they
	// have somewhere to put a name.
	var name string
	field := base.NewText(base.TextConfig{Label: "Name", Initial: name})
	typeInto(field, "ada")
	if field.Text() != "ada" {
		t.Fatalf("the caller's variable = %q", field.Text())
	}
}

func TestAFieldStartsFromWhatTheCallerAlreadyHas(t *testing.T) {
	name := "ada"
	field := base.NewText(base.TextConfig{Label: "Name", Initial: name})
	rows := paintWidget(12, field.HeightForWidth(12), field)
	if !strings.Contains(strings.Join(rows, "\n"), "ada") {
		t.Fatalf("drawn:\n%s\nwant what the caller already had", strings.Join(rows, "\n"))
	}
}

func TestAnAnswerIsCheckedOnTheWayOutAndNotOnTheWayIn(t *testing.T) {
	missing := errors.New("required")
	var name string
	checks := 0
	field := base.NewText(base.TextConfig{
		Label: "Name", Initial: name, Check: func(s string) error {
			checks++
			if s == "" {
				return missing
			}
			return nil
		},
	})
	if field.Error() != nil {
		t.Fatalf("a field nobody has visited complained: %v", field.Error())
	}
	testFocus(field, true)
	if field.Error() != nil {
		t.Fatalf("a field complained on arrival: %v", field.Error())
	}
	testFocus(field, false)
	if !errors.Is(field.Error(), missing) {
		t.Fatalf("error on the way out = %v, want the check's", field.Error())
	}
	testFocus(field, false)
	if checks != 1 {
		t.Fatalf("one real focus loss ran the check %d times", checks)
	}
	testFocus(field, true)
	testFocus(field, false)
	if checks != 2 {
		t.Fatalf("a second visit and loss ran %d total checks, want 2", checks)
	}
}

func TestAChoiceFollowsTheCursor(t *testing.T) {
	// There is nothing to press. A list that made somebody move to a row and then take
	// it is a list that can be left on a row nobody took.
	var picked string
	field := selectWith(base.Options("fast", "good", "cheap"))
	field.Label = "Model"
	field.Choose(picked)
	newTestRoot(field).Draw(grid.NewSurface(12, 4).View())
	if chosenValue(field) != "fast" {
		t.Fatalf("drawing chose %q for the caller", chosenValue(field))
	}
	field.Handle(input.Key{Code: input.Down})
	if chosenValue(field) != "good" {
		t.Fatalf("value = %q, want the option the cursor moved to", chosenValue(field))
	}
}

func TestDrawingAChoiceDoesNotChooseForTheCaller(t *testing.T) {
	picked := "not an option"
	field := selectWith(base.Options("fast", "good", "cheap"))
	field.Choose(picked)

	newTestRoot(field).Draw(grid.NewSurface(12, 4).View())
	if chosenValue(field) != "fast" {
		t.Fatalf("Draw changed the caller's value to %q", chosenValue(field))
	}
}

func TestAChoiceStartsOnWhatWasAlreadyChosen(t *testing.T) {
	picked := "cheap"
	field := selectWith(base.Options("fast", "good", "cheap"))
	field.Choose(picked)
	chosen, ok := field.Chosen()
	if !ok || chosen.Value != "cheap" {
		t.Fatalf("cursor on %+v (ok=%v), want the choice already made", chosen, ok)
	}
}

func TestReplacingChoicesPreservesTheChosenValueRatherThanItsOldPosition(t *testing.T) {
	picked := "b"
	field := selectWith(base.Options("a", "b", "c"))
	field.Choose(picked)
	if chosen, ok := field.Chosen(); !ok || chosen.Value != "b" {
		t.Fatalf("initial choice = %+v, %v", chosen, ok)
	}

	field.SetOptions(base.Options("c", "a", "b"))
	if chosen, ok := field.Chosen(); !ok || chosen.Value != "b" || chosenValue(field) != "b" {
		t.Fatalf("choice after moving options = %+v, %v; bound value %q", chosen, ok, chosenValue(field))
	}

	field.SetOptions(base.Options("c", "a"))
	if chosen, ok := field.Chosen(); !ok || chosen.Value != "a" || chosenValue(field) != "a" {
		t.Fatalf("choice after removal = %+v, %v; bound value %q", chosen, ok, chosenValue(field))
	}
}

func TestAChoiceOwnsItsOptionsAndReturnsSnapshots(t *testing.T) {
	options := base.Options("a", "b")
	field := &base.Select[string]{Same: base.Equal[string]}
	field.SetOptions(options)
	options[0].Label = "changed input"
	if got := field.Options()[0].Label; got != "a" {
		t.Fatalf("first label after input mutation = %q", got)
	}
	snapshot := field.Options()
	snapshot[0].Label = "changed snapshot"
	if got := field.Options()[0].Label; got != "a" {
		t.Fatalf("first label after snapshot mutation = %q", got)
	}
}

func TestPickingSeveralIsMovingAndThenTaking(t *testing.T) {
	// The whole difference between picking one and picking some: here the cursor and
	// the choice are two things.
	var picked []string
	field := multiWith(base.Options("a", "b", "c"))
	field.Label = "Files"
	field.SetTaken(picked)
	field.Handle(input.Key{Code: input.Down})
	if len(field.Taken()) != 0 {
		t.Fatalf("moving took a choice: %v", field.Taken())
	}
	field.Handle(input.Key{Code: input.Character, Rune: ' '})
	if len(field.Taken()) != 1 || field.Taken()[0] != "b" {
		t.Fatalf("after taking = %v, want the row under the cursor", field.Taken())
	}
	field.Handle(input.Key{Code: input.Character, Rune: ' '})
	if len(field.Taken()) != 0 {
		t.Fatalf("taking it again did not give it back: %v", field.Taken())
	}
}

func TestPickingSeveralStopsAtItsLimit(t *testing.T) {
	var picked []string
	field := multiWith(base.Options("a", "b", "c"))
	field.SetTaken(picked)
	field.SetLimit(1)
	field.Do(base.Toggle)
	field.Do(base.SelectNext)
	field.Do(base.Toggle)
	if len(field.Taken()) != 1 || field.Taken()[0] != "a" {
		t.Fatalf("= %v, want only what fits in the limit", field.Taken())
	}
}

func TestLoweringMultipleChoiceLimitSettlesTheBoundValue(t *testing.T) {
	picked := []string{"a", "b", "c"}
	field := multiWith(base.Options("a", "b", "c"))
	field.SetTaken(picked)
	if got := field.Taken(); len(got) != 3 {
		t.Fatalf("initially taken = %v, want all three", got)
	}

	field.SetLimit(1)
	if field.Limit() != 1 {
		t.Fatalf("limit = %d, want 1", field.Limit())
	}
	if len(field.Taken()) != 1 || field.Taken()[0] != "a" {
		t.Fatalf("settled binding = %v, want the first option", field.Taken())
	}
}

func TestMultipleChoiceLimitClampsTheInitialBoundValue(t *testing.T) {
	picked := []string{"a", "b", "c"}
	field := multiWith(base.Options("a", "b", "c"))
	field.SetTaken(picked)
	field.SetLimit(2)

	if got := field.Taken(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("taken = %v, want the first two options", got)
	}
	if len(field.Taken()) != 2 || field.Taken()[0] != "a" || field.Taken()[1] != "b" {
		t.Fatalf("settled binding = %v, want the first two options", field.Taken())
	}
}

func TestMultipleChoiceLimitRejectsNegativeValues(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("SetLimit accepted a negative limit")
		}
	}()
	new(base.MultiSelect[string]).SetLimit(-1)
}

func TestNilMultipleChoiceRejectsLimitChanges(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil MultiSelect silently ignored mutation")
		}
	}()
	var field *base.MultiSelect[string]
	field.SetLimit(-1)
}

func TestReplacingMultipleChoicesMovesTheTakenSetByValue(t *testing.T) {
	picked := []string{"b"}
	field := multiWith(base.Options("a", "b", "c"))
	field.SetTaken(picked)
	if got := field.Taken(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("initially taken = %v", got)
	}

	field.SetOptions(base.Options("c", "a", "b"))
	if got := field.Taken(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("taken after moving options = %v", got)
	}
	if len(field.Taken()) != 1 || field.Taken()[0] != "b" {
		t.Fatalf("bound values after moving options = %v", field.Taken())
	}

	field.SetOptions(base.Options("c", "a"))
	if got := field.Taken(); len(got) != 0 || len(field.Taken()) != 0 {
		t.Fatalf("taken after removal = %v; bound values %v", got, field.Taken())
	}
}

func TestReplacingChoicesUsesExplicitValueIdentity(t *testing.T) {
	type payload []int
	options := []base.Option[payload]{
		{Label: "one", Value: payload{1}},
		{Label: "two", Value: payload{2}},
	}

	one := &base.Select[payload]{Same: slices.Equal[payload]}
	one.SetOptions(options)
	one.Do(base.SelectNext)
	one.SetOptions([]base.Option[payload]{options[1], options[0]})
	chosen, ok := one.Chosen()
	if !ok || chosen.Label != "two" {
		t.Fatalf("single choice after reorder = %+v, %v; want label two", chosen, ok)
	}

	many := &base.MultiSelect[payload]{Same: slices.Equal[payload]}
	many.SetOptions(options)
	many.Do(base.SelectNext)
	many.Do(base.Toggle)
	many.SetOptions([]base.Option[payload]{options[1], options[0]})
	taken := many.Taken()
	if len(taken) != 1 || len(taken[0]) != 1 || taken[0][0] != 2 {
		t.Fatalf("multiple choices after reorder = %v; want the value labelled two", taken)
	}
}

func TestAChoiceIsMarkedAsTaken(t *testing.T) {
	var picked []string
	field := multiWith(base.Options("a", "b"))
	field.SetTaken(picked)
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	styleFields(form, bare())
	newTestRoot(form).Draw(grid.NewSurface(8, 4).View())
	field.Do(base.Toggle)

	rows := paintWidget(8, 4, form)

	if !strings.HasPrefix(rows[0], "x.a") {
		t.Fatalf("first row = %q, want it marked as taken", rows[0])
	}
	if !strings.HasPrefix(rows[1], "-.b") {
		t.Fatalf("second row = %q, want it marked as still on offer", rows[1])
	}
}

func TestAConfirmAnswersYesOrNo(t *testing.T) {
	var sure bool
	field := base.NewConfirm(base.ConfirmConfig{Label: "Delete it?", Initial: sure})
	field.Handle(input.Key{Code: input.Left})
	if !field.Answer() {
		t.Fatal("left did not say yes")
	}
	field.Handle(input.Key{Code: input.Right})
	if field.Answer() {
		t.Fatal("right did not say no")
	}
	field.Handle(input.Key{Code: input.Character, Rune: ' '})
	if !field.Answer() {
		t.Fatal("space did not turn it over")
	}
}

func TestAFormWalksItsFieldsAndSubmitsWhenEverythingChecksOut(t *testing.T) {
	var name, model string
	missing := errors.New("required")
	nameField := base.NewText(base.TextConfig{
		Label: "Name", Initial: name, Check: func(s string) error {
			if s == "" {
				return missing
			}
			return nil
		},
	})
	modelField := selectWith(base.Options("fast", "good"))
	modelField.Label = "Model"
	modelField.Choose(model)
	done := 0
	form := base.NewForm(base.FormConfig{Fields: []base.Field{nameField, modelField}, Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
		done++
		return scene.ActionResult[base.SubmissionID]{State: scene.Applied, Value: id}, nil
	}})
	newTestRoot(form).Draw(grid.NewSurface(20, 8).View())

	if newTestRoot(form).Handle(input.Key{Code: input.Enter}); done != 0 {
		t.Fatal("an empty form was submitted")
	}
	if !errors.Is(nameField.Error(), missing) {
		t.Fatalf("the field that is not filled in says %v", nameField.Error())
	}

	typeInto(nameField, "ada")
	newTestRoot(form).Handle(input.Key{Code: input.Enter})
	if done != 1 {
		t.Fatalf("submitted %d times, want one", done)
	}
	if nameField.Text() != "ada" || chosenValue(modelField) != "fast" {
		t.Fatalf("collected %q and %q", nameField.Text(), chosenValue(modelField))
	}

	newTestRoot(form).Handle(input.Key{Code: input.Tab})
	if form.Focused() != base.Field(modelField) {
		t.Fatal("tab did not move to the next field")
	}
}

func TestAFormChecksEveryAnswerAndNotJustTheFirstOneThatFails(t *testing.T) {
	empty := func(s string) error {
		if s == "" {
			return errors.New("required")
		}
		return nil
	}
	first := &base.Text{Label: "One", Check: empty}
	second := &base.Text{Label: "Two", Check: empty}
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first, second}})
	if _, err := form.Submit(); err == nil {
		t.Fatal("an empty form said it was complete")
	}
	if first.Error() == nil || second.Error() == nil {
		t.Fatal("only some of the fields were checked")
	}
}

func TestAFormChecksTheAnswersTogetherOnlyOnceEachIsGood(t *testing.T) {
	clash := errors.New("those two cannot both be so")
	one := &base.Text{Label: "One"}
	form := base.NewForm(base.FormConfig{Fields: []base.Field{one}, Check: func() error { return clash }})
	if _, err := form.Submit(); err == nil {
		t.Fatal("the form was submitted with a problem across its fields")
	}
	if !errors.Is(form.Error(), clash) {
		t.Fatalf("the form says %v", form.Error())
	}
}

func TestAFieldShowsWhatWasWrongUnderItself(t *testing.T) {
	field := &base.Text{
		Label: "Name",
		Check: func(string) error { return errors.New("required") },
	}
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	styleFields(form, bare())
	if before := field.HeightForWidth(20); before != 2 {
		t.Fatalf("a field with no problem is %d rows, want the label and the answer", before)
	}
	_, _ = form.Submit()
	if after := field.HeightForWidth(20); after != 3 {
		t.Fatalf("a field with a problem is %d rows, want a row for it", after)
	}
	rows := paintWidget(20, field.HeightForWidth(20), form)
	if !strings.HasPrefix(rows[0], "Name") {
		t.Fatalf("first row = %q, want what the field is asking for", rows[0])
	}
	if !strings.HasPrefix(rows[2], "required") {
		t.Fatalf("last row = %q, want what was wrong with it", rows[2])
	}
}

func TestAChoiceCanBePressed(t *testing.T) {
	// A list could be walked with the arrow keys and not pointed at. The label is a row
	// of the field and not part of what it holds, so a press under it means a row lower
	// than the position says — the same translation a container does for its children.
	var picked string
	field := selectWith(base.Options("fast", "good", "cheap"))
	field.Label = "Model"
	field.Choose(picked)
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	styleFields(form, bare())
	newTestRoot(form).Draw(grid.NewSurface(12, 4).View())

	newTestRoot(form).Handle(input.Mouse{
		Pos: image.Pt(2, 3), Action: input.MouseDown, Button: input.ButtonLeft,
	})
	if chosenValue(field) != "cheap" {
		t.Fatalf("pressing the third row chose %q", chosenValue(field))
	}

	newTestRoot(form).Handle(input.Mouse{Pos: image.Pt(2, 1), Action: input.MouseDrag})
	if chosenValue(field) != "fast" {
		t.Fatalf("dragging back up chose %q", chosenValue(field))
	}
}

func TestAPressAboveTheOptionsIsNotAChoice(t *testing.T) {
	// The label's own row belongs to the label. A field that read it as the first
	// option would answer a press nobody made.
	var picked string
	field := selectWith(base.Options("fast", "good"))
	field.Label = "Model"
	field.Choose(picked)
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	styleFields(form, bare())
	newTestRoot(form).Draw(grid.NewSurface(12, 3).View())
	field.Do(base.SelectNext)

	newTestRoot(form).Handle(input.Mouse{
		Pos: image.Pt(2, 0), Action: input.MouseDown, Button: input.ButtonLeft,
	})
	if chosenValue(field) != "good" {
		t.Fatalf("a press on the label chose %q", chosenValue(field))
	}
}

func TestAConfirmCanBePressed(t *testing.T) {
	var sure bool
	field := base.NewConfirm(base.ConfirmConfig{Label: "Delete it?", Initial: sure, Yes: "yes", No: "no"})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	styleFields(form, bare())
	newTestRoot(form).Draw(grid.NewSurface(20, 2).View())

	press := func(x int) {
		newTestRoot(form).Handle(input.Mouse{
			Pos: image.Pt(x, 1), Action: input.MouseDown, Button: input.ButtonLeft,
		})
	}
	press(1)
	if !field.Answer() {
		t.Fatal("pressing the first answer did not say yes")
	}
	press(10)
	if field.Answer() {
		t.Fatal("pressing the second answer did not say no")
	}
}

func TestAFieldNobodyHasDrawnAnswersNoPress(t *testing.T) {
	field := &base.Text{Label: "Name"}
	if field.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("a field that has never been drawn answered a press")
	}
}

// kind is a value a form collects that is not a string, which is what the comparison
// rules are about.
type kind int

func TestAChoiceOfSomethingThatIsNotAStringFindsWhatWasAlreadyChosen(t *testing.T) {
	picked := kind(1)
	shown := selectWith([]base.Option[kind]{{Label: "file", Value: 0}, {Label: "folder", Value: 1}})
	shown.Choose(picked)
	if chosen, _ := shown.Chosen(); chosen.Value != 1 {
		t.Fatalf("cursor on %+v, want what the value is shown as", chosen)
	}

	byValue := selectWith([]base.Option[kind]{{Label: "one", Value: 0}, {Label: "two", Value: 1}})
	byValue.Choose(picked)
	byValue.Same = func(a, b kind) bool { return a == b }
	if chosen, _ := byValue.Chosen(); chosen.Value != 1 {
		t.Fatalf("cursor on %+v, want the caller's rule to have found it", chosen)
	}
}

func TestEveryFieldAnswersToTheNameOfWhatItDoes(t *testing.T) {
	// The same promise the widgets make, kept by the fields built on them: an action
	// is reachable from somewhere that is not the keyboard.
	var name string
	text := base.NewText(base.TextConfig{Initial: name})
	typeInto(text, "one two")
	if !text.Do(base.DeleteWordBack) || text.Text() != "one " {
		t.Fatalf("text = %q", text.Text())
	}

	var sure bool
	confirm := base.NewConfirm(base.ConfirmConfig{Initial: sure})
	if !confirm.Do(base.Toggle) || !confirm.Answer() {
		t.Fatal("the confirm did not turn over")
	}
	if confirm.Do("fly") {
		t.Fatal("the confirm claimed an action nobody taught it")
	}
}

func TestEveryFieldChecksWhatItHolds(t *testing.T) {
	tooMany := errors.New("too many")
	multi := multiWith(base.Options("a", "b"))
	multi.Check = func(v []string) error {
		if len(v) > 1 {
			return tooMany
		}
		return nil
	}
	multi.Do(base.Toggle)
	multi.Do(base.SelectNext)
	multi.Do(base.Toggle)
	if !errors.Is(multi.Validate(), tooMany) {
		t.Fatalf("the multi-select says %v", multi.Error())
	}

	never := errors.New("never")
	confirm := &base.Confirm{Check: func(bool) error { return never }}
	if !errors.Is(confirm.Validate(), never) {
		t.Fatalf("the confirm says %v", confirm.Error())
	}
}

func TestAFormPassesTheKeyboardToTheFieldThatHasIt(t *testing.T) {
	first := &base.Text{Label: "One"}
	second := &base.Text{Label: "Two"}
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first, second}})
	newTestRoot(form).Draw(grid.NewSurface(20, 6).View())

	testFocus(form, false)
	if first.Handle(input.Key{Code: input.Character, Rune: 'x'}); first.Text() != "x" {
		t.Fatal("the field itself stopped taking text")
	}
	testFocus(form, true)
	if got := form.HeightForWidth(20); got != 4 {
		t.Fatalf("a form of two one-line fields with labels is %d rows", got)
	}
}

func TestAReleaseOutsideAFieldStillEndsTheSelectionItStarted(t *testing.T) {
	field := base.NewText(base.TextConfig{Label: "Name"})
	field.SetText("abcdef")
	newTestRoot(field).Draw(grid.NewSurface(20, 2).View())

	if !field.Handle(input.Mouse{Pos: image.Pt(2, 1), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("the press inside the field was not answered")
	}

	if !field.Handle(input.Mouse{Pos: image.Pt(2, 9), Action: input.MouseUp, Button: input.ButtonLeft}) {
		t.Fatal("the release outside the field was not answered")
	}

	if field.Handle(input.Mouse{Pos: image.Pt(5, 1), Action: input.MouseDrag, Button: input.ButtonLeft}) {
		t.Fatal("a drag from elsewhere went on extending the field's selection")
	}
}

// normalizingValue is a controlled value whose owner reacts to being written.
//
// It is the ordinary shape of a controlled field in an application: Set is the
// change notification, and what an application does there is update its model and
// tell the interface. Doing that synchronously is not exotic — it is what a
// single-goroutine interface does, and the alternative would be to post the change
// back to itself.

func styleFields(form *base.Form, look base.Look) {
	for _, field := range form.Fields() {
		themed := field.(base.ThemedField)
		if err := field.Element().SetPaint(func(frame scene.Frame) { themed.DrawWith(frame, look) }); err != nil {
			panic(err)
		}
	}
}
