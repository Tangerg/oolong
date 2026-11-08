package base_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type choicePayload struct {
	id      int
	allowed bool
}

func sameChoiceIdentity(a, b choicePayload) bool { return a.id == b.id }

func TestSelectAliasCannotCarryValidationToAnotherPayload(t *testing.T) {
	for _, transition := range []string{"navigate", "replace options", "retain selected option"} {
		t.Run(transition, func(t *testing.T) {
			denied := errors.New("choice payload is not allowed")
			first := base.NewSelect(base.SelectConfig[choicePayload]{
				Same: sameChoiceIdentity,
				Options: []base.Option[choicePayload]{
					{Label: "allowed alias", Value: choicePayload{id: 1, allowed: true}},
					{Label: "denied alias", Value: choicePayload{id: 1}},
				},
				Check: func(value choicePayload) error {
					if !value.allowed {
						return denied
					}
					return nil
				},
			})
			before := first.Revision()
			unchanged := transition == "retain selected option"
			second := base.NewText(base.TextConfig{Check: func(string) error {
				receipt := first.Validation()
				if receipt == nil || !receipt() {
					t.Fatal("first field did not publish its validation")
				}
				switch transition {
				case "navigate":
					first.Do(base.SelectNext)
				case "replace options":
					first.SetOptions([]base.Option[choicePayload]{{Label: "denied", Value: choicePayload{id: 1}}})
				case "retain selected option":
					first.Choose(choicePayload{id: 1})
				}
				if receipt() != unchanged {
					t.Fatal("validation receipt does not describe its selected payload")
				}
				return nil
			}})
			submitted := false
			form := base.NewForm(base.FormConfig{
				Fields: []base.Field{first, second},
				Submit: func(base.SubmissionID) (scene.ActionState, error) {
					submitted = true
					return scene.Applied, nil
				},
			})
			_, err := form.Submit()
			if unchanged {
				if err != nil || !submitted || first.Revision() != before {
					t.Fatalf("unchanged selection: submitted=%v revision=%d error=%v", submitted, first.Revision(), err)
				}
				return
			}
			if submitted || !errors.Is(err, base.ErrValidationSuperseded) || first.Revision() != before+1 {
				t.Fatalf("replaced selection: submitted=%v revision=%d error=%v", submitted, first.Revision(), err)
			}
			if err = first.Validate(); !errors.Is(err, denied) {
				t.Fatalf("replacement payload should fail its own check: %v", err)
			}
		})
	}
}

func TestChoiceReplacementRevokesSubmissionSnapshot(t *testing.T) {
	for _, kind := range []string{"select", "multi-select"} {
		t.Run(kind, func(t *testing.T) {
			initial := []base.Option[choicePayload]{{Label: "initial", Value: choicePayload{id: 1, allowed: true}}}
			replacement := []base.Option[choicePayload]{{Label: "replacement", Value: choicePayload{id: 1}}}
			var field base.Field
			var replace func()
			if kind == "select" {
				choice := base.NewSelect(base.SelectConfig[choicePayload]{Same: sameChoiceIdentity, Options: initial})
				field, replace = choice, func() { choice.SetOptions(replacement) }
			} else {
				choice := base.NewMultiSelect(base.MultiSelectConfig[choicePayload]{
					Same: sameChoiceIdentity, Options: initial, Initial: []choicePayload{initial[0].Value},
				})
				field, replace = choice, func() { choice.SetOptions(replacement) }
			}
			form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
			if _, err := form.Submit(); err != nil || !form.Submission().Current {
				t.Fatalf("initial submission: state=%+v error=%v", form.Submission(), err)
			}
			before := field.Revision()
			replace()
			if form.Submission().Current || field.Revision() != before+1 {
				t.Fatalf("changed payload retained submission: revision=%d submission=%+v", field.Revision(), form.Submission())
			}
			if _, err := form.Submit(); err != nil || !form.Submission().Current {
				t.Fatalf("replacement submission: state=%+v error=%v", form.Submission(), err)
			}
		})
	}
}

func TestMultiSelectAliasesReadTheirCanonicalPayload(t *testing.T) {
	canonical := choicePayload{id: 1, allowed: true}
	alias := choicePayload{id: 1}
	field := base.NewMultiSelect(base.MultiSelectConfig[choicePayload]{
		Same: sameChoiceIdentity,
		Options: []base.Option[choicePayload]{
			{Label: "canonical", Value: canonical},
			{Label: "alias", Value: alias},
		},
		Initial: []choicePayload{canonical},
	})
	before := field.Revision()
	field.Do(base.SelectNext)
	field.SetTaken([]choicePayload{alias, canonical})
	if !slices.Equal(field.Taken(), []choicePayload{canonical}) || field.Revision() != before {
		t.Fatal("cursor movement or identity lookup replaced the canonical answer")
	}
	field.SetOptions([]base.Option[choicePayload]{{Label: "new canonical", Value: alias}})
	if !slices.Equal(field.Taken(), []choicePayload{alias}) || field.Revision() != before+1 {
		t.Fatal("option replacement did not publish the complete canonical answer")
	}
}

func TestEmptyChoiceOwnersDoNotInventAnswerChanges(t *testing.T) {
	var single base.Select[string]
	var multiple base.MultiSelect[string]
	for _, field := range []interface {
		base.Field
		SetOptions(options []base.Option[string])
	}{&single, &multiple} {
		before := field.Revision()
		field.SetOptions(nil)
		if field.Revision() != before {
			t.Fatal("empty options advanced the empty answer")
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("non-empty options were accepted without an identity policy")
				}
			}()
			field.SetOptions(base.Options("unavailable"))
		}()
		if field.Revision() != before {
			t.Fatal("rejected options changed the answer revision")
		}
		if err := field.Validate(); err != nil {
			t.Fatalf("empty zero-value field cannot validate: %v", err)
		}
	}
	if _, exists := single.Chosen(); exists || len(multiple.Taken()) != 0 {
		t.Fatal("rejected options changed a zero-value field")
	}
	configured := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string]})
	configured.SetOptions(base.Options("available"))
	if chosen, exists := configured.Chosen(); !exists || chosen.Value != "available" {
		t.Fatal("constructor identity was lost while the field was empty")
	}
}
