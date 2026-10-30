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
