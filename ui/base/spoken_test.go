package base_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestAnExactSpokenChoiceWinsOverLongerPrefixes(t *testing.T) {
	var chosen string
	field := selectWith(base.Options("good", "goose", "go"))
	field.Choose(chosen)
	if err := field.Reply("go"); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if chosenValue(field) != "go" {
		t.Fatalf("chosen = %q, want the exact label", chosenValue(field))
	}
}

func TestSpokenReplyRetainsTheAcceptedDraftAfterValidationFails(t *testing.T) {
	problem := errors.New("draft requires correction")
	text := base.NewText(base.TextConfig{Initial: "before", Check: func(string) error { return problem }})
	confirm := base.NewConfirm(base.ConfirmConfig{Check: func(bool) error { return problem }})
	choice := base.NewSelect(base.SelectConfig[string]{Initial: "before", Options: base.Options("before", "after"), Same: base.Equal[string], Check: func(string) error { return problem }})
	many := base.NewMultiSelect(base.MultiSelectConfig[string]{Initial: []string{"before"}, Options: base.Options("before", "after"), Same: base.Equal[string], Check: func([]string) error { return problem }})
	for _, test := range []struct {
		name     string
		field    base.SpokenField
		answer   string
		retained func() bool
	}{
		{"text", text, "after", func() bool { return text.Text() == "after" }},
		{"confirm", confirm, "yes", confirm.Answer},
		{"select", choice, "after", func() bool { selected, _ := choice.Chosen(); return selected.Value == "after" }},
		{"multiselect", many, "after", func() bool { return slices.Equal(many.Taken(), []string{"after"}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.field.Revision()
			err := test.field.Reply(test.answer)
			if !errors.Is(err, problem) || !errors.Is(test.field.Error(), problem) || !test.retained() || test.field.Revision() == before {
				t.Fatalf("validation failure rolled back its accepted draft: error=%v revision=%d", err, test.field.Revision())
			}
			if errors.Is(err, base.ErrReplyUnavailable) || errors.Is(err, base.ErrValidationSuperseded) {
				t.Fatal("correctable validation failure was marked nonretryable")
			}
		})
	}
}

func TestSpokenParseRefusalPreservesThePreviousAnswer(t *testing.T) {
	confirm := base.NewConfirm(base.ConfirmConfig{Initial: true})
	choice := base.NewSelect(base.SelectConfig[string]{Initial: "before", Options: base.Options("before", "after"), Same: base.Equal[string]})
	many := base.NewMultiSelect(base.MultiSelectConfig[string]{Initial: []string{"before"}, Options: base.Options("before", "after"), Same: base.Equal[string]})
	for _, test := range []struct {
		field     base.SpokenField
		unchanged func() bool
	}{
		{confirm, confirm.Answer},
		{choice, func() bool { selected, _ := choice.Chosen(); return selected.Value == "before" }},
		{many, func() bool { return slices.Equal(many.Taken(), []string{"before"}) }},
	} {
		before := test.field.Revision()
		if err := test.field.Reply("missing"); err == nil || test.field.Revision() != before || !test.unchanged() {
			t.Fatalf("%T changed its answer before parsing succeeded: %v", test.field, err)
		}
	}
}

func TestSpokenConfirmFailurePreservesAcceptedDomainEffects(t *testing.T) {
	problem := errors.New("accepted change has a later failure")
	for _, asynchronous := range []bool{false, true} {
		domain := &confirmDomain{asynchronous: asynchronous, problem: problem}
		checks := 0
		field := base.NewConfirm(base.ConfirmConfig{Control: domain, Check: func(bool) error { checks++; return nil }})
		t.Cleanup(field.Close)
		err := field.Reply("yes")
		if !errors.Is(err, base.ErrReplyUnavailable) || !errors.Is(err, problem) || domain.requests != 1 || checks != 0 || field.Error() != nil || field.Validation() != nil {
			t.Fatalf("partial domain failure = %v, requests=%d, checks=%d, validation=%v", err, domain.requests, checks, field.Error())
		}
		if field.Answer() != !asynchronous || (domain.pending != nil) != asynchronous {
			t.Fatalf("partial failure lost accepted effects: answer=%t pending=%v", field.Answer(), domain.pending)
		}
	}
}

func TestDuplicateSpokenChoicesAreRefused(t *testing.T) {
	field := selectWith(base.Options("same", "same"))
	if err := field.Reply("same"); err == nil {
		t.Fatal("two choices with the same spoken label were guessed between")
	}
}

func TestAnAmbiguousSpokenConfirmationIsRefused(t *testing.T) {
	field := base.NewConfirm(base.ConfirmConfig{Yes: "enable", No: "enter"})
	if err := field.Reply("en"); err == nil {
		t.Fatal("a prefix shared by both answers was guessed")
	}
}

func TestExactConfirmationCanBeAPrefixOfTheOtherAnswer(t *testing.T) {
	field := base.NewConfirm(base.ConfirmConfig{Yes: "on", No: "once"})
	if err := field.Reply("ON"); err != nil || !field.Answer() {
		t.Fatalf("exact answer was not accepted: %v", err)
	}
	if err := field.Reply("o"); err == nil {
		t.Fatal("ambiguous abbreviation was accepted")
	}
}

func TestRejectedReplyExpiresWhenTheAnswerChanges(t *testing.T) {
	one := selectWith(base.Options("one", "two"))
	many := multiWith(base.Options("one", "two"))
	confirm := base.NewConfirm(base.ConfirmConfig{})
	for _, tc := range []struct {
		field interface {
			base.Field
			Reply(said string) error
		}
		change func()
	}{
		{one, func() { one.Choose("two") }},
		{many, func() { many.Toggle() }},
		{confirm, func() { confirm.Say(true) }},
	} {
		if err := tc.field.Reply("invalid"); err == nil || tc.field.Error() == nil || tc.field.Validation() != nil {
			t.Fatalf("%T did not retain a rejected reply as a parse error", tc.field)
		}
		tc.change()
		if err := tc.field.Error(); err != nil {
			t.Fatalf("%T displays an obsolete reply on its new answer: %v", tc.field, err)
		}
	}
}

func TestChangingChoiceLimitInvalidatesThePreviousReplyError(t *testing.T) {
	field := multiWith(base.Options("one", "two"))
	field.SetLimit(1)
	if err := field.Reply("one,two"); err == nil {
		t.Fatal("invalid reply was accepted")
	}
	field.SetLimit(2)
	if field.Error() != nil {
		t.Fatal("the old limit is still displayed")
	}
}
