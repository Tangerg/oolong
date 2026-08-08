package headless_test

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// bare is a look with the marks a choice needs and no colour, which is what a test can
// read off a surface.
func bare() headless.Look { return headless.Look{Taken: "x", Free: "-"} }

func selectWith[T any](options []headless.Option[T]) *headless.Select[T] {
	field := new(headless.Select[T])
	field.SetOptions(options)
	return field
}

func multiWith[T any](options []headless.Option[T]) *headless.MultiSelect[T] {
	field := new(headless.MultiSelect[T])
	field.SetOptions(options)
	return field
}

// typeInto types a string into a field, one keystroke at a time.
func typeInto(f headless.Field, s string) {
	for _, r := range s {
		f.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

type observedField struct {
	focused bool
	handled int
}

func (*observedField) Draw(headless.Frame)       {}
func (*observedField) Measure(int) int           { return 1 }
func (*observedField) Prompt() string            { return "" }
func (*observedField) Validate() error           { return nil }
func (*observedField) Error() error              { return nil }
func (f *observedField) Focus(has bool)          { f.focused = has }
func (f *observedField) Handle(input.Event) bool { f.handled++; return true }

func TestReplacingFormFieldsTransfersKeyboardOwnership(t *testing.T) {
	old, next := new(observedField), new(observedField)
	fields := []headless.Field{old}
	form := headless.NewForm(fields...)
	fields[0] = next

	if got := form.Fields()[0]; got != old {
		t.Fatal("form retained the caller's field slice")
	}
	form.Set(next)
	if old.focused || !next.focused {
		t.Fatalf("focus after Set: old=%v next=%v", old.focused, next.focused)
	}
	form.Handle(input.FocusIn{})
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
	headless.NewForm(nil)
}

func TestAFieldPutsWhatWasTypedWhereTheCallerKeepsIt(t *testing.T) {
	// A field does not own what it collects. Somebody asked for a name because they
	// have somewhere to put a name.
	var name string
	field := &headless.Text{Label: "Name", Value: headless.Bind(&name)}
	typeInto(field, "ada")
	if name != "ada" {
		t.Fatalf("the caller's variable = %q", name)
	}
}

func TestBindRejectsANilPointerAtTheBoundary(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Bind accepted a nil pointer")
		}
	}()
	headless.Bind((*string)(nil))
}

func TestAFieldStartsFromWhatTheCallerAlreadyHas(t *testing.T) {
	// Which is what makes a form somebody is coming back to show what they said last
	// time, rather than an empty one they have to fill in again.
	name := "ada"
	field := &headless.Text{Label: "Name", Value: headless.Bind(&name)}
	rows := paintWidget(12, field.Measure(12), field)
	if !strings.Contains(strings.Join(rows, "\n"), "ada") {
		t.Fatalf("drawn:\n%s\nwant what the caller already had", strings.Join(rows, "\n"))
	}
}

func TestAnAnswerIsCheckedOnTheWayOutAndNotOnTheWayIn(t *testing.T) {
	// A form that greeted somebody with a column of complaints about answers they have
	// not given yet would be a form nobody finishes.
	missing := errors.New("required")
	var name string
	field := &headless.Text{
		Label: "Name",
		Value: headless.Bind(&name),
		Check: func(s string) error {
			if s == "" {
				return missing
			}
			return nil
		},
	}
	field.Focus(false) // never had it, so there is nothing to check
	if field.Error() != nil {
		t.Fatalf("a field nobody has visited complained: %v", field.Error())
	}
	field.Focus(true)
	if field.Error() != nil {
		t.Fatalf("a field complained on arrival: %v", field.Error())
	}
	field.Focus(false)
	if !errors.Is(field.Error(), missing) {
		t.Fatalf("error on the way out = %v, want the check's", field.Error())
	}
}

func TestAChoiceFollowsTheCursor(t *testing.T) {
	// There is nothing to press. A list that made somebody move to a row and then take
	// it is a list that can be left on a row nobody took.
	var picked string
	field := selectWith(headless.Options("fast", "good", "cheap"))
	field.Label = "Model"
	field.Value = headless.Bind(&picked)
	headless.NewRoot(field).Draw(grid.NewSurface(12, 4).View())
	if picked != "" {
		t.Fatalf("drawing chose %q for the caller", picked)
	}
	field.Handle(input.Key{Code: input.Down})
	if picked != "good" {
		t.Fatalf("value = %q, want the option the cursor moved to", picked)
	}
}

func TestDrawingAChoiceDoesNotChooseForTheCaller(t *testing.T) {
	picked := "not an option"
	field := selectWith(headless.Options("fast", "good", "cheap"))
	field.Value = headless.Bind(&picked)

	headless.NewRoot(field).Draw(grid.NewSurface(12, 4).View())
	if picked != "not an option" {
		t.Fatalf("Draw changed the caller's value to %q", picked)
	}
}

func TestAChoiceStartsOnWhatWasAlreadyChosen(t *testing.T) {
	picked := "cheap"
	field := selectWith(headless.Options("fast", "good", "cheap"))
	field.Value = headless.Bind(&picked)
	chosen, ok := field.Chosen()
	if !ok || chosen.Value != "cheap" {
		t.Fatalf("cursor on %+v (ok=%v), want the choice already made", chosen, ok)
	}
}

func TestReplacingChoicesPreservesTheChosenValueRatherThanItsOldPosition(t *testing.T) {
	picked := "b"
	field := selectWith(headless.Options("a", "b", "c"))
	field.Value = headless.Bind(&picked)
	if chosen, ok := field.Chosen(); !ok || chosen.Value != "b" {
		t.Fatalf("initial choice = %+v, %v", chosen, ok)
	}

	field.SetOptions(headless.Options("c", "a", "b"))
	if chosen, ok := field.Chosen(); !ok || chosen.Value != "b" || picked != "b" {
		t.Fatalf("choice after moving options = %+v, %v; bound value %q", chosen, ok, picked)
	}

	field.SetOptions(headless.Options("c", "a"))
	if chosen, ok := field.Chosen(); !ok || chosen.Value != "a" || picked != "a" {
		t.Fatalf("choice after removal = %+v, %v; bound value %q", chosen, ok, picked)
	}
}

func TestAChoiceOwnsItsOptionsAndReturnsSnapshots(t *testing.T) {
	options := headless.Options("a", "b")
	field := new(headless.Select[string])
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
	field := multiWith(headless.Options("a", "b", "c"))
	field.Label = "Files"
	field.Value = headless.Bind(&picked)
	field.Handle(input.Key{Code: input.Down})
	if len(picked) != 0 {
		t.Fatalf("moving took a choice: %v", picked)
	}
	field.Handle(input.Key{Code: input.Character, Rune: ' '})
	if len(picked) != 1 || picked[0] != "b" {
		t.Fatalf("after taking = %v, want the row under the cursor", picked)
	}
	field.Handle(input.Key{Code: input.Character, Rune: ' '})
	if len(picked) != 0 {
		t.Fatalf("taking it again did not give it back: %v", picked)
	}
}

func TestPickingSeveralStopsAtItsLimit(t *testing.T) {
	var picked []string
	field := multiWith(headless.Options("a", "b", "c"))
	field.Value = headless.Bind(&picked)
	field.Limit = 1
	field.Do(headless.Toggle)
	field.Do(headless.SelectNext)
	field.Do(headless.Toggle)
	if len(picked) != 1 || picked[0] != "a" {
		t.Fatalf("= %v, want only what fits in the limit", picked)
	}
}

func TestReplacingMultipleChoicesMovesTheTakenSetByValue(t *testing.T) {
	picked := []string{"b"}
	field := multiWith(headless.Options("a", "b", "c"))
	field.Value = headless.Bind(&picked)
	if got := field.Taken(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("initially taken = %v", got)
	}

	field.SetOptions(headless.Options("c", "a", "b"))
	if got := field.Taken(); len(got) != 1 || got[0] != "b" {
		t.Fatalf("taken after moving options = %v", got)
	}
	if len(picked) != 1 || picked[0] != "b" {
		t.Fatalf("bound values after moving options = %v", picked)
	}

	field.SetOptions(headless.Options("c", "a"))
	if got := field.Taken(); len(got) != 0 || len(picked) != 0 {
		t.Fatalf("taken after removal = %v; bound values %v", got, picked)
	}
}

func TestReplacingChoicesUsesLabelsForArbitraryValues(t *testing.T) {
	type payload []int
	options := []headless.Option[payload]{
		{Label: "one", Value: payload{1}},
		{Label: "two", Value: payload{2}},
	}

	one := selectWith(options)
	one.Do(headless.SelectNext)
	one.SetOptions([]headless.Option[payload]{options[1], options[0]})
	chosen, ok := one.Chosen()
	if !ok || chosen.Label != "two" {
		t.Fatalf("single choice after reorder = %+v, %v; want label two", chosen, ok)
	}

	many := multiWith(options)
	many.Do(headless.SelectNext)
	many.Do(headless.Toggle)
	many.SetOptions([]headless.Option[payload]{options[1], options[0]})
	taken := many.Taken()
	if len(taken) != 1 || len(taken[0]) != 1 || taken[0][0] != 2 {
		t.Fatalf("multiple choices after reorder = %v; want the value labelled two", taken)
	}
}

func TestAChoiceIsMarkedAsTaken(t *testing.T) {
	var picked []string
	field := multiWith(headless.Options("a", "b"))
	field.Value = headless.Bind(&picked)
	form := headless.NewForm(field)
	form.Look = bare()
	headless.NewRoot(form).Draw(grid.NewSurface(8, 4).View())
	field.Do(headless.Toggle)

	rows := paintWidget(8, 4, form)
	// A dot is a cell nothing was drawn into: the column between the mark and the label
	// is a gap and not a space.
	if !strings.HasPrefix(rows[0], "x.a") {
		t.Fatalf("first row = %q, want it marked as taken", rows[0])
	}
	if !strings.HasPrefix(rows[1], "-.b") {
		t.Fatalf("second row = %q, want it marked as still on offer", rows[1])
	}
}

func TestAConfirmAnswersYesOrNo(t *testing.T) {
	var sure bool
	field := &headless.Confirm{Label: "Delete it?", Value: headless.Bind(&sure)}
	field.Handle(input.Key{Code: input.Left})
	if !sure {
		t.Fatal("left did not say yes")
	}
	field.Handle(input.Key{Code: input.Right})
	if sure {
		t.Fatal("right did not say no")
	}
	field.Handle(input.Key{Code: input.Character, Rune: ' '})
	if !sure {
		t.Fatal("space did not turn it over")
	}
}

func TestAFormWalksItsFieldsAndSubmitsWhenEverythingChecksOut(t *testing.T) {
	var name, model string
	missing := errors.New("required")
	nameField := &headless.Text{
		Label: "Name", Value: headless.Bind(&name),
		Check: func(s string) error {
			if s == "" {
				return missing
			}
			return nil
		},
	}
	modelField := selectWith(headless.Options("fast", "good"))
	modelField.Label, modelField.Value = "Model", headless.Bind(&model)
	done := 0
	form := headless.NewForm(nameField, modelField)
	form.Done = func() { done++ }
	headless.NewRoot(form).Draw(grid.NewSurface(20, 8).View())

	// Nothing typed, so submitting says so rather than finishing.
	if form.Handle(input.Key{Code: input.Enter}); done != 0 {
		t.Fatal("an empty form was submitted")
	}
	if !errors.Is(nameField.Error(), missing) {
		t.Fatalf("the field that is not filled in says %v", nameField.Error())
	}

	typeInto(nameField, "ada")
	form.Handle(input.Key{Code: input.Enter})
	if done != 1 {
		t.Fatalf("submitted %d times, want one", done)
	}
	if name != "ada" || model != "fast" {
		t.Fatalf("collected %q and %q", name, model)
	}

	// Tab moves the keyboard on, which is the container's doing and not the form's.
	form.Handle(input.Key{Code: input.Tab})
	if form.Focused() != headless.Field(modelField) {
		t.Fatal("tab did not move to the next field")
	}
}

func TestAFormChecksEveryAnswerAndNotJustTheFirstOneThatFails(t *testing.T) {
	// A form that reported its problems one at a time would make somebody submit four
	// times to find out there were four.
	empty := func(s string) error {
		if s == "" {
			return errors.New("required")
		}
		return nil
	}
	first := &headless.Text{Label: "One", Check: empty}
	second := &headless.Text{Label: "Two", Check: empty}
	form := headless.NewForm(first, second)
	if form.Submit() {
		t.Fatal("an empty form said it was complete")
	}
	if first.Error() == nil || second.Error() == nil {
		t.Fatal("only some of the fields were checked")
	}
}

func TestAFormChecksTheAnswersTogetherOnlyOnceEachIsGood(t *testing.T) {
	clash := errors.New("those two cannot both be so")
	one := &headless.Text{Label: "One"}
	form := headless.NewForm(one)
	form.Check = func() error { return clash }
	if form.Submit() {
		t.Fatal("the form was submitted with a problem across its fields")
	}
	if !errors.Is(form.Error(), clash) {
		t.Fatalf("the form says %v", form.Error())
	}
}

func TestAFieldShowsWhatWasWrongUnderItself(t *testing.T) {
	field := &headless.Text{
		Label: "Name",
		Check: func(string) error { return errors.New("required") },
	}
	form := headless.NewForm(field)
	form.Look = bare()
	if before := field.Measure(20); before != 2 {
		t.Fatalf("a field with no problem is %d rows, want the label and the answer", before)
	}
	form.Submit()
	if after := field.Measure(20); after != 3 {
		t.Fatalf("a field with a problem is %d rows, want a row for it", after)
	}
	rows := paintWidget(20, field.Measure(20), form)
	if !strings.HasPrefix(rows[0], "Name") {
		t.Fatalf("first row = %q, want what the field is asking for", rows[0])
	}
	if !strings.HasPrefix(rows[2], "required") {
		t.Fatalf("last row = %q, want what was wrong with it", rows[2])
	}
}

func TestAFormAbandonedSaysSo(t *testing.T) {
	gone := 0
	form := headless.NewForm(&headless.Text{Label: "One"})
	form.GaveUp = func() { gone++ }
	form.Handle(input.Key{Code: input.Esc})
	if gone != 1 {
		t.Fatalf("cancelled %d times", gone)
	}
}

func TestAChoiceCanBePressed(t *testing.T) {
	// A list could be walked with the arrow keys and not pointed at. The label is a row
	// of the field and not part of what it holds, so a press under it means a row lower
	// than the position says — the same translation a container does for its children.
	var picked string
	field := selectWith(headless.Options("fast", "good", "cheap"))
	field.Label, field.Value = "Model", headless.Bind(&picked)
	form := headless.NewForm(field)
	form.Look = bare()
	headless.NewRoot(form).Draw(grid.NewSurface(12, 4).View())

	form.Handle(input.Mouse{
		Pos: image.Pt(2, 3), Action: input.MouseDown, Button: input.ButtonLeft,
	})
	if picked != "cheap" {
		t.Fatalf("pressing the third row chose %q", picked)
	}
	// And a drag carries the choice with it.
	form.Handle(input.Mouse{Pos: image.Pt(2, 1), Action: input.MouseDrag})
	if picked != "fast" {
		t.Fatalf("dragging back up chose %q", picked)
	}
}

func TestAPressAboveTheOptionsIsNotAChoice(t *testing.T) {
	// The label's own row belongs to the label. A field that read it as the first
	// option would answer a press nobody made.
	var picked string
	field := selectWith(headless.Options("fast", "good"))
	field.Label, field.Value = "Model", headless.Bind(&picked)
	form := headless.NewForm(field)
	form.Look = bare()
	headless.NewRoot(form).Draw(grid.NewSurface(12, 3).View())
	field.Do(headless.SelectNext)

	form.Handle(input.Mouse{
		Pos: image.Pt(2, 0), Action: input.MouseDown, Button: input.ButtonLeft,
	})
	if picked != "good" {
		t.Fatalf("a press on the label chose %q", picked)
	}
}

func TestAConfirmCanBePressed(t *testing.T) {
	var sure bool
	field := &headless.Confirm{Label: "Delete it?", Value: headless.Bind(&sure), Yes: "yes", No: "no"}
	form := headless.NewForm(field)
	form.Look = bare()
	headless.NewRoot(form).Draw(grid.NewSurface(20, 2).View())

	press := func(x int) {
		form.Handle(input.Mouse{
			Pos: image.Pt(x, 1), Action: input.MouseDown, Button: input.ButtonLeft,
		})
	}
	press(1)
	if !sure {
		t.Fatal("pressing the first answer did not say yes")
	}
	press(10)
	if sure {
		t.Fatal("pressing the second answer did not say no")
	}
}

func TestAFieldNobodyHasDrawnAnswersNoPress(t *testing.T) {
	// A press arrives between two frames and is about the one on screen. With no frame
	// on screen there is nothing for it to be about.
	field := &headless.Text{Label: "Name"}
	if field.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("a field that has never been drawn answered a press")
	}
}

// kind is a value a form collects that is not a string, which is what the comparison
// rules are about.
type kind int

func (k kind) String() string { return [...]string{"file", "folder"}[k] }

func TestAChoiceOfSomethingThatIsNotAStringFindsWhatWasAlreadyChosen(t *testing.T) {
	// Go will not compare two values of a type parameter, so with no rule the labels
	// are compared — which is right whenever what is shown is what it means.
	picked := kind(1)
	shown := selectWith([]headless.Option[kind]{{Label: "file", Value: 0}, {Label: "folder", Value: 1}})
	shown.Value = headless.Bind(&picked)
	if chosen, _ := shown.Chosen(); chosen.Value != 1 {
		t.Fatalf("cursor on %+v, want what the value is shown as", chosen)
	}

	// And a rule of the caller's wins, which is the only thing that works when two
	// options are shown the same way or when the type is one Go cannot compare.
	byValue := selectWith([]headless.Option[kind]{{Label: "one", Value: 0}, {Label: "two", Value: 1}})
	byValue.Value = headless.Bind(&picked)
	byValue.Same = func(a, b kind) bool { return a == b }
	if chosen, _ := byValue.Chosen(); chosen.Value != 1 {
		t.Fatalf("cursor on %+v, want the caller's rule to have found it", chosen)
	}
}

func TestEveryFieldAnswersToTheNameOfWhatItDoes(t *testing.T) {
	// The same promise the widgets make, kept by the fields built on them: an action
	// is reachable from somewhere that is not the keyboard.
	var name string
	text := &headless.Text{Value: headless.Bind(&name)}
	typeInto(text, "one two")
	if !text.Do(headless.DeleteWordBack) || name != "one " {
		t.Fatalf("text = %q", name)
	}

	var sure bool
	confirm := &headless.Confirm{Value: headless.Bind(&sure)}
	if !confirm.Do(headless.Toggle) || !confirm.Answer() {
		t.Fatal("the confirm did not turn over")
	}
	if confirm.Do("fly") {
		t.Fatal("the confirm claimed an action nobody taught it")
	}
}

func TestEveryFieldChecksWhatItHolds(t *testing.T) {
	tooMany := errors.New("too many")
	multi := multiWith(headless.Options("a", "b"))
	multi.Check = func(v []string) error {
		if len(v) > 1 {
			return tooMany
		}
		return nil
	}
	multi.Do(headless.Toggle)
	multi.Do(headless.SelectNext)
	multi.Do(headless.Toggle)
	if !errors.Is(multi.Validate(), tooMany) {
		t.Fatalf("the multi-select says %v", multi.Error())
	}

	never := errors.New("never")
	confirm := &headless.Confirm{Check: func(bool) error { return never }}
	if !errors.Is(confirm.Validate(), never) {
		t.Fatalf("the confirm says %v", confirm.Error())
	}
}

func TestAFormPassesTheKeyboardToTheFieldThatHasIt(t *testing.T) {
	first := &headless.Text{Label: "One"}
	second := &headless.Text{Label: "Two"}
	form := headless.NewForm(first, second)
	headless.NewRoot(form).Draw(grid.NewSurface(20, 6).View())

	form.Focus(false)
	if first.Editor().Handle(input.Key{Code: input.Character, Rune: 'x'}); first.Editor().Text() != "x" {
		t.Fatal("the field itself stopped taking text")
	}
	form.Focus(true)
	if got := form.Measure(20); got != 4 {
		t.Fatalf("a form of two one-line fields with labels is %d rows", got)
	}
}
