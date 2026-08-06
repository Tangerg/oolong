package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestAnExactSpokenChoiceWinsOverLongerPrefixes(t *testing.T) {
	var chosen string
	field := &headless.Select[string]{
		Options: headless.Options("good", "goose", "go"),
		Value:   headless.Bind(&chosen),
	}
	if err := field.Reply("go"); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if chosen != "go" {
		t.Fatalf("chosen = %q, want the exact label", chosen)
	}
}

func TestDuplicateSpokenChoicesAreRefused(t *testing.T) {
	field := &headless.Select[string]{Options: headless.Options("same", "same")}
	if err := field.Reply("same"); err == nil {
		t.Fatal("two choices with the same spoken label were guessed between")
	}
}

func TestAnAmbiguousSpokenConfirmationIsRefused(t *testing.T) {
	field := &headless.Confirm{Yes: "enable", No: "enter"}
	if err := field.Reply("en"); err == nil {
		t.Fatal("a prefix shared by both answers was guessed")
	}
}
