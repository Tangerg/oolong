package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestFieldValidationRevokesTheOuterPublicationOnReentry(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			stale, latest := errors.New("old validation"), errors.New("new validation")
			var field base.Field
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
				field = &base.Text{Check: func(string) error { return check() }}
			case "Select":
				field = &base.Select[string]{Check: func(string) error { return check() }}
			case "MultiSelect":
				field = &base.MultiSelect[string]{Check: func([]string) error { return check() }}
			case "Confirm":
				field = &base.Confirm{Check: func(bool) error { return check() }}
			}
			if err := field.Validate(); !errors.Is(err, base.ErrValidationSuperseded) {
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
			var field base.Field
			var change func()
			check := func() error { change(); return stale }
			switch kind {
			case "Text":
				f := &base.Text{Check: func(string) error { return check() }}
				field, change = f, func() { f.SetText("new") }
			case "Select":
				f := &base.Select[string]{Same: base.Equal[string], Check: func(string) error { return check() }}
				f.SetOptions(base.Options("old", "new"))
				field, change = f, func() { f.Do(base.SelectNext) }
			case "MultiSelect":
				f := &base.MultiSelect[string]{Same: base.Equal[string], Check: func([]string) error { return check() }}
				f.SetOptions(base.Options("new"))
				field, change = f, func() { f.Toggle() }
			case "Confirm":
				f := &base.Confirm{Check: func(bool) error { return check() }}
				field, change = f, func() { f.Say(true) }
			}
			if err := field.Validate(); !errors.Is(err, base.ErrValidationSuperseded) {
				t.Fatalf("obsolete check returned %v", err)
			}
			if field.Error() != nil {
				t.Fatalf("obsolete check published %v for the newer answer", field.Error())
			}
		})
	}
}

func TestSpokenReplyOwnsParsingAndValidationTogether(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			var field interface {
				base.Field
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
				field = &base.Text{Check: func(string) error { return check() }}
			case "Select":
				f := &base.Select[string]{Same: base.Equal[string], Check: func(string) error { return check() }}
				f.SetOptions(base.Options("yes"))
				field = f
			case "MultiSelect":
				f := &base.MultiSelect[string]{Same: base.Equal[string], Check: func([]string) error { return check() }}
				f.SetOptions(base.Options("yes"))
				field = f
			case "Confirm":
				field = &base.Confirm{Check: func(bool) error { return check() }}
			}
			if err := field.Reply("yes"); !errors.Is(err, base.ErrValidationSuperseded) {
				t.Fatalf("superseded reply = %v", err)
			}
			if field.Error() == nil {
				t.Fatal("outer reply erased the newer problem")
			}
		})
	}
}
