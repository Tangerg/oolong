package base_test

import (
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

func TestDuplicateSpokenChoicesAreRefused(t *testing.T) {
	field := selectWith(base.Options("same", "same"))
	if err := field.Reply("same"); err == nil {
		t.Fatal("two choices with the same spoken label were guessed between")
	}
}

func TestAnAmbiguousSpokenConfirmationIsRefused(t *testing.T) {
	field := &base.Confirm{Yes: "enable", No: "enter"}
	if err := field.Reply("en"); err == nil {
		t.Fatal("a prefix shared by both answers was guessed")
	}
}

func TestExactConfirmationCanBeAPrefixOfTheOtherAnswer(t *testing.T) {
	field := &base.Confirm{Yes: "on", No: "once"}
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
