package headless_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestSelectSettlementCannotOverwriteANewerLocalOwner(t *testing.T) {
	value := &publishingValue[string]{value: "a"}
	field := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Value: value})
	field.SetOptions(headless.Options("a", "b", "c"))
	value.set = func(string) {
		field.SetBinding(nil)
		if err := field.Reply("c"); err != nil {
			t.Fatal(err)
		}
	}
	field.Do(headless.SelectNext)
	chosen, ok := field.Chosen()
	if !ok || chosen.Value != "c" || value.writes != 1 {
		t.Fatalf("choice=%v exists=%t writes=%d, want the newer local c", chosen, ok, value.writes)
	}
}

func TestMultiSelectSettlementCannotOverwriteANewerLocalOwner(t *testing.T) {
	value := &publishingValue[[]string]{}
	field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: value})
	field.SetOptions(headless.Options("a", "b", "c"))
	value.set = func([]string) {
		field.SetBinding(nil)
		if err := field.Reply("c"); err != nil {
			t.Fatal(err)
		}
	}
	field.Toggle()
	if got := field.Taken(); !slices.Equal(got, []string{"c"}) || value.writes != 1 {
		t.Fatalf("choices=%v writes=%d, want the newer local c", got, value.writes)
	}
}

func TestMultiSelectCannotContinueAfterSyncStartedANewerAnswer(t *testing.T) {
	for _, command := range []string{"Toggle", "Do", "Handle", "Reply"} {
		t.Run(command, func(t *testing.T) {
			value := &publishingValue[[]string]{value: []string{"a", "unavailable"}}
			field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: value})
			var keys keymap.Map
			keys.Bind(headless.Toggle, input.Chord{Rune: 't'})
			field.Keys = &keys
			field.SetOptions(headless.Options("a", "b"))
			value.set = func([]string) {
				value.set = nil
				if err := field.Reply("b"); err != nil {
					t.Fatal(err)
				}
			}
			switch command {
			case "Toggle":
				if field.Toggle() {
					t.Fatal("superseded toggle reported changing the newer answer")
				}
			case "Do":
				if field.Do(headless.Toggle) {
					t.Fatal("superseded action was dispatched")
				}
			case "Handle":
				if !field.Handle(input.Key{Rune: 't'}) {
					t.Fatal("interrupted key was not consumed")
				}
			case "Reply":
				if err := field.Reply("a"); !errors.Is(err, headless.ErrValidationSuperseded) {
					t.Fatalf("interrupted reply returned %v", err)
				}
			}
			if got := field.Taken(); !slices.Equal(got, []string{"b"}) || value.writes != 2 {
				t.Fatalf("choices=%v writes=%d, want b after normalization and the newer answer", got, value.writes)
			}
		})
	}
}

func TestMultiSelectDoesNotResubmitAnUnfinishedNormalization(t *testing.T) {
	value := &transformingAccessor[[]string]{value: []string{"b", "unavailable", "b"}}
	field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: value})
	field.SetOptions(headless.Options("a", "b"))
	value.transform = func(proposed []string) []string {
		if value.writes != 1 {
			t.Fatalf("normalization resubmitted its unfinished proposal: %d writes", value.writes)
		}
		field.Sync()
		return proposed
	}
	field.Sync()
	if got := field.Taken(); !slices.Equal(got, []string{"b"}) || value.writes != 1 {
		t.Fatalf("choices=%v writes=%d, want one normalization to b", got, value.writes)
	}
}

func TestMultiSelectProjectionSyncCannotRevokeTheNormalizingCommand(t *testing.T) {
	value := &transformingAccessor[[]string]{value: []string{"a", "unavailable"}}
	field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: value})
	field.SetOptions(headless.Options("a", "b"))
	value.transform = func(proposed []string) []string { field.Sync(); return proposed }
	if !field.Toggle() || len(field.Taken()) != 0 || value.writes != 2 {
		t.Fatalf("answer=%v writes=%d, want normalization then removal of a", field.Taken(), value.writes)
	}
}

func TestMultiSelectCanNormalizeANewerOwnerDuringSettlement(t *testing.T) {
	old := &publishingValue[[]string]{value: []string{"a", "unavailable"}}
	latest := &publishingValue[[]string]{value: []string{"b", "unavailable"}}
	field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: old})
	field.SetOptions(headless.Options("a", "b"))
	old.set = func([]string) {
		field.SetBinding(latest)
		field.Sync()
	}
	field.Sync()
	if !slices.Equal(latest.value, []string{"b"}) || latest.writes != 1 || old.writes != 1 {
		t.Fatalf("new answer=%v new writes=%d old writes=%d", latest.value, latest.writes, old.writes)
	}
}

func TestMultiSelectCannotApplyAnOldCommandToANewerOwner(t *testing.T) {
	old := &publishingValue[[]string]{value: []string{"a", "unavailable"}}
	latest := &publishingValue[[]string]{value: []string{"b"}}
	field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: old})
	field.SetOptions(headless.Options("a", "b"))
	old.set = func([]string) { field.SetBinding(latest); field.Sync() }
	if field.Toggle() || !slices.Equal(latest.value, []string{"b"}) || latest.writes != 0 {
		t.Fatalf("new answer=%v writes=%d", latest.value, latest.writes)
	}
}

type functionalChoiceAccessor struct {
	read  func() []string
	write func([]string)
}

func (v functionalChoiceAccessor) Value() []string     { return v.read() }
func (v functionalChoiceAccessor) Set(values []string) { v.write(values) }

func TestMultiSelectBindingBoundarySupportsValueAndPointerAccessors(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		t.Run(map[bool]string{false: "value", true: "pointer"}[pointer], func(t *testing.T) {
			value := []string{"a", "unavailable"}
			var field *headless.MultiSelect[string]
			accessor := functionalChoiceAccessor{
				read:  func() []string { return value },
				write: func(values []string) { field.Sync(); value = values },
			}
			var binding headless.Accessor[[]string] = accessor
			if pointer {
				binding = &accessor
			}
			field = headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: headless.Equal[string], Value: binding})
			field.SetOptions(headless.Options("a", "b"))
			if !field.Sync() || !slices.Equal(value, []string{"a"}) || !field.Toggle() || len(field.Taken()) != 0 {
				t.Fatal("functional accessor lost normalization or ordinary selection")
			}
		})
	}
}
