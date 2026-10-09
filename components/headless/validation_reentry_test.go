package headless_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestFieldValidationRevokesTheOuterPublicationOnReentry(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			stale, latest := errors.New("old validation"), errors.New("new validation")
			var field headless.Field
			calls := 0
			check := func() error {
				calls++
				if calls == 1 {
					if err := field.Validate(); !errors.Is(err, latest) {
						t.Fatalf("nested validation = %v, want %v", err, latest)
					}
					return stale
				}
				return latest
			}
			switch kind {
			case "Text":
				field = &headless.Text{Check: func(string) error { return check() }}
			case "Select":
				field = &headless.Select[string]{Check: func(string) error { return check() }}
			case "MultiSelect":
				field = &headless.MultiSelect[string]{Check: func([]string) error { return check() }}
			case "Confirm":
				field = &headless.Confirm{Check: func(bool) error { return check() }}
			}
			if err := field.Validate(); !errors.Is(err, headless.ErrValidationSuperseded) {
				t.Fatalf("superseded validation = %v", err)
			}
			if !errors.Is(field.Error(), latest) {
				t.Fatalf("published validation = %v, want %v", field.Error(), latest)
			}
		})
	}
}

func TestFieldValidationCannotPublishForAnAnswerReplacedDuringCheck(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			stale := errors.New("problem with the old answer")
			var field headless.Field
			var change func()
			check := func() error { change(); return stale }
			switch kind {
			case "Text":
				f := &headless.Text{Check: func(string) error { return check() }}
				field, change = f, func() { f.SetText("new") }
			case "Select":
				f := &headless.Select[string]{Same: headless.Equal[string], Check: func(string) error { return check() }}
				f.SetOptions(headless.Options("old", "new"))
				field, change = f, func() { f.Do(headless.SelectNext) }
			case "MultiSelect":
				f := &headless.MultiSelect[string]{Same: headless.Equal[string], Check: func([]string) error { return check() }}
				f.SetOptions(headless.Options("new"))
				field, change = f, func() { f.Toggle() }
			case "Confirm":
				f := &headless.Confirm{Check: func(bool) error { return check() }}
				field, change = f, func() { f.Say(true) }
			}
			if err := field.Validate(); !errors.Is(err, headless.ErrValidationSuperseded) {
				t.Fatalf("obsolete check returned %v", err)
			}
			if field.Error() != nil {
				t.Fatalf("obsolete check published %v for the newer answer", field.Error())
			}
		})
	}
}

func TestFieldValidationObservesOwnerWritesDuringCheck(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			stale := errors.New("problem with the old answer")
			var field headless.Field
			switch kind {
			case "Text":
				value := &countedAccessor[string]{value: "old"}
				field = headless.NewText(headless.TextConfig{Value: value, Check: func(string) error { value.value = "new"; return stale }})
			case "Select":
				value := &countedAccessor[string]{value: "old"}
				f := headless.NewSelect(headless.SelectConfig[string]{
					Same: headless.Equal[string], Value: value,
					Check: func(string) error { value.value = "new"; return stale },
				})
				f.SetOptions(headless.Options("old", "new"))
				field = f
			case "MultiSelect":
				value := &countedAccessor[[]string]{value: []string{"old"}}
				f := headless.NewMultiSelect(headless.MultiSelectConfig[string]{
					Same: headless.Equal[string], Value: value,
					Check: func(values []string) error { values[0] = "new"; value.value = values; return stale },
				})
				f.SetOptions(headless.Options("old", "new"))
				field = f
			case "Confirm":
				value := &countedAccessor[bool]{}
				field = headless.NewConfirm(headless.ConfirmConfig{Value: value, Check: func(bool) error { value.value = true; return stale }})
			}
			if err := field.Validate(); !errors.Is(err, headless.ErrValidationSuperseded) {
				t.Fatalf("obsolete check returned %v", err)
			}
			if field.Error() != nil {
				t.Fatalf("obsolete check published %v for the newer owner answer", field.Error())
			}
		})
	}
}

func TestFormCannotSubmitAnAnswerChangedAfterItsFieldWasChecked(t *testing.T) {
	for _, changeAt := range []string{"later field", "form check"} {
		t.Run(changeAt, func(t *testing.T) {
			first := &headless.Text{Check: func(answer string) error {
				if answer == "invalid" {
					return errors.New("invalid answer")
				}
				return nil
			}}
			second := &headless.Text{}
			form := headless.NewForm(first, second)
			change := func() { first.SetText("invalid") }
			if changeAt == "later field" {
				second.Check = func(string) error { change(); return nil }
			} else {
				form.Check = func() error { change(); return nil }
			}
			completed := false
			form.Done = func() { completed = true }
			if form.Submit() || completed {
				t.Fatal("form submitted an answer its successful field check never saw")
			}
		})
	}
}

func TestFieldValidationReceiptsBelongToOneCheckAndAnswer(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			var field headless.Field
			var change func()
			calls := 0
			check := func() { calls++ }
			switch kind {
			case "Text":
				value := &countedAccessor[string]{value: "old"}
				field = headless.NewText(headless.TextConfig{Value: value, Check: func(string) error { check(); return nil }})
				change = func() { value.value = "new" }
			case "Select":
				value := &countedAccessor[string]{value: "old"}
				f := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Value: value, Check: func(string) error { check(); return nil }})
				f.SetOptions(headless.Options("old", "new"))
				field, change = f, func() { value.value = "new" }
			case "MultiSelect":
				value := &countedAccessor[[]string]{value: []string{"old"}}
				f := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: value, Check: func([]string) error { check(); return nil }})
				f.SetOptions(headless.Options("old", "new"))
				field, change = f, func() { value.value = []string{"new"} }
			case "Confirm":
				value := &countedAccessor[bool]{}
				field = headless.NewConfirm(headless.ConfirmConfig{Value: value, Check: func(bool) error { check(); return nil }})
				change = func() { value.value = true }
			}
			if field.Validation() != nil {
				t.Fatal("unchecked answer had a validation receipt")
			}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			previous := field.Validation()
			if previous == nil || !previous() || !previous() || calls != 1 {
				t.Fatalf("receipt was absent, invalid or reran checking: %d calls", calls)
			}
			change()
			if previous() || field.Validation() != nil || calls != 1 {
				t.Fatal("a read renewed validation for a changed answer")
			}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			current := field.Validation()
			if current == nil || !current() || previous() {
				t.Fatal("new checking did not replace the old receipt")
			}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			if current() {
				t.Fatal("another check left the previous receipt current")
			}
		})
	}
}

func TestFormCannotUseAFieldReceiptReplacedByALaterCallback(t *testing.T) {
	first := &headless.Confirm{}
	form := headless.NewForm(first)
	stale := errors.New("obsolete form check")
	form.Check = func() error {
		if err := first.Validate(); err != nil {
			t.Fatal(err)
		}
		return stale
	}
	if form.Submit() || form.Error() != nil {
		t.Fatal("form published a cross-field result after its receipt was replaced")
	}
}

func TestFormCannotSubmitAProjectionItsBindingRefusedToAccept(t *testing.T) {
	text := &transformingAccessor[string]{value: "one\ntwo"}
	text.transform = func(string) string { return text.value }
	choice := &transformingAccessor[string]{value: "unavailable"}
	choice.transform = func(string) string { return choice.value }
	choices := &transformingAccessor[[]string]{value: []string{"a", "unavailable", "a"}}
	choices.transform = func([]string) []string { return choices.value }
	one := headless.NewSelect(headless.SelectConfig[string]{Value: choice, Same: headless.Equal[string]})
	one.SetOptions(headless.Options("a", "b"))
	many := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Value: choices, Same: headless.Equal[string]})
	many.SetOptions(headless.Options("a", "b"))
	for kind, field := range map[string]headless.Field{
		"Text": headless.NewText(headless.TextConfig{Value: text}), "Select": one, "MultiSelect": many,
	} {
		t.Run(kind, func(t *testing.T) {
			if err := field.Validate(); !errors.Is(err, headless.ErrInvalidBinding) || !errors.Is(field.Error(), headless.ErrInvalidBinding) || field.Validation() != nil {
				t.Fatal("a projected answer hid the invalid state its owner retained")
			}
			form := headless.NewForm(field)
			completed := false
			form.Done = func() { completed = true }
			if form.Submit() || completed {
				t.Fatal("form submitted a projection that its state owner never accepted")
			}
			switch kind {
			case "Text":
				text.value = "one two"
			case "Select":
				choice.value = "a"
			case "MultiSelect":
				choices.value = []string{"a"}
			}
			if !form.Submit() || !completed {
				t.Fatal("form could not recover after its owner accepted a valid answer")
			}
		})
	}
}

func TestValidationReceiptChecksTheOwnerEvenWhenItsProjectionIsUnchanged(t *testing.T) {
	text := &countedAccessor[string]{value: "one two"}
	choice := &countedAccessor[string]{value: "a"}
	choices := &countedAccessor[[]string]{value: []string{"a"}}
	one := headless.NewSelect(headless.SelectConfig[string]{Value: choice, Same: headless.Equal[string]})
	one.SetOptions(headless.Options("a", "b"))
	many := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Value: choices, Same: headless.Equal[string]})
	many.SetOptions(headless.Options("a", "b"))
	cases := map[string]struct {
		field  headless.Field
		change func()
	}{
		"Text":        {headless.NewText(headless.TextConfig{Value: text}), func() { text.value = "one\ntwo" }},
		"Select":      {one, func() { choice.value = "unavailable" }},
		"MultiSelect": {many, func() { choices.value = []string{"a", "a", "unavailable"} }},
	}
	for kind, test := range cases {
		t.Run(kind, func(t *testing.T) {
			if err := test.field.Validate(); err != nil {
				t.Fatal(err)
			}
			current := test.field.Validation()
			if current == nil || !current() {
				t.Fatal("valid owner had no current receipt")
			}
			test.change()
			if current() || test.field.Validation() != nil {
				t.Fatal("an unchanged projection concealed its owner's invalid state")
			}
		})
	}
}

func TestSpokenReplyOwnsParsingAndValidationTogether(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			var field interface {
				headless.Field
				Reply(said string) error
			}
			latest := errors.New("new answer")
			calls := 0
			check := func() error {
				calls++
				if calls == 1 {
					if kind == "Text" {
						if err := field.Reply("next"); !errors.Is(err, latest) {
							t.Fatalf("nested reply = %v", err)
						}
					} else if err := field.Reply("invalid"); err == nil {
						t.Fatal("invalid nested reply succeeded")
					}
					return nil
				}
				return latest
			}
			switch kind {
			case "Text":
				field = &headless.Text{Check: func(string) error { return check() }}
			case "Select":
				f := &headless.Select[string]{Same: headless.Equal[string], Check: func(string) error { return check() }}
				f.SetOptions(headless.Options("yes"))
				field = f
			case "MultiSelect":
				f := &headless.MultiSelect[string]{Same: headless.Equal[string], Check: func([]string) error { return check() }}
				f.SetOptions(headless.Options("yes"))
				field = f
			case "Confirm":
				field = &headless.Confirm{Check: func(bool) error { return check() }}
			}
			if err := field.Reply("yes"); !errors.Is(err, headless.ErrValidationSuperseded) {
				t.Fatalf("superseded reply = %v", err)
			}
			if field.Error() == nil {
				t.Fatal("outer reply erased the newer problem")
			}
		})
	}
}

func TestFormSubmissionStopsWhenItsCollectionIsReplaced(t *testing.T) {
	for _, change := range []string{"Set", "Add"} {
		t.Run(change, func(t *testing.T) {
			replacement := &headless.Text{Check: func(string) error { return errors.New("required") }}
			old := &headless.Text{}
			form := headless.NewForm(old)
			old.Check = func(string) error {
				old.Check = nil
				if change == "Set" {
					form.Set(replacement)
				} else {
					form.Add(replacement)
				}
				return nil
			}
			done := 0
			form.Done = func() { done++ }
			if form.Submit() || done != 0 {
				t.Fatalf("changed submission completed: done=%d", done)
			}
			if form.Submit() || replacement.Error() == nil {
				t.Fatal("new field was not validated by the next submission")
			}
		})
	}
}

func TestFormOnlyTheNewestSubmissionCanComplete(t *testing.T) {
	for _, site := range []string{"field", "form"} {
		t.Run(site, func(t *testing.T) {
			field := &headless.Text{}
			form := headless.NewForm(field)
			calls, done := 0, 0
			check := func() {
				calls++
				if calls == 1 && !form.Submit() {
					t.Fatal("newer valid submission failed")
				}
			}
			if site == "field" {
				field.Check = func(string) error { check(); return nil }
			} else {
				form.Check = func() error { check(); return nil }
			}
			form.Done = func() { done++ }
			if form.Submit() || done != 1 {
				t.Fatalf("superseded submission completed: done=%d, want one nested completion", done)
			}
		})
	}
}

func TestSpokenReplyCannotApplyAnAnswerAfterSyncStartedANewerReply(t *testing.T) {
	value := &publishingValue[[]string]{value: []string{"a", "a"}}
	field := &headless.MultiSelect[string]{Same: headless.Equal[string]}
	field.SetOptions(headless.Options("a", "b"))
	field.SetBinding(value)
	value.set = func([]string) {
		value.set = nil
		if err := field.Reply("b"); err != nil {
			t.Fatal(err)
		}
	}
	if err := field.Reply("a"); !errors.Is(err, headless.ErrValidationSuperseded) {
		t.Fatalf("superseded reply = %v", err)
	}
	if got := field.Taken(); len(got) != 1 || got[0] != "b" || value.writes != 2 {
		t.Fatalf("answer=%v writes=%d, want b after two writes", got, value.writes)
	}
}

func TestFormSubmissionRevokesTheOuterCheckOnReentry(t *testing.T) {
	form := headless.NewForm(&headless.Text{})
	stale, latest := errors.New("old form"), errors.New("new form")
	calls := 0
	form.Check = func() error {
		calls++
		if calls == 1 {
			if form.Submit() {
				t.Fatal("nested invalid form submitted")
			}
			return stale
		}
		return latest
	}
	if form.Submit() {
		t.Fatal("superseded form submitted")
	}
	if !errors.Is(form.Error(), latest) {
		t.Fatalf("published form error = %v, want %v", form.Error(), latest)
	}
}

func TestFormSubmissionStopsAfterCancellationFromACheck(t *testing.T) {
	for _, site := range []string{"field", "form"} {
		t.Run(site, func(t *testing.T) {
			field := &headless.Text{}
			form := headless.NewForm(field)
			if site == "field" {
				field.Check = func(string) error { form.Cancel(); return nil }
			} else {
				form.Check = func() error { form.Cancel(); return nil }
			}
			done, cancelled := 0, 0
			form.Done = func() { done++ }
			form.GaveUp = func() { cancelled++ }
			if form.Submit() || done != 0 || cancelled != 1 {
				t.Fatalf("cancelled submission: done=%d cancelled=%d", done, cancelled)
			}
		})
	}
}
