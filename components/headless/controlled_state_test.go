package headless_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
)

// countedAccessor makes assignments observable. Bind deliberately makes writing the
// same value look harmless, while a real accessor may persist, validate or publish
// every Set call.
type countedAccessor[T any] struct {
	value  T
	writes int
}

type rejectingIntAccessor struct {
	value  int
	writes int
}

func (a *rejectingIntAccessor) Value() int { return a.value }

func (a *rejectingIntAccessor) Set(int) { a.writes++ }

func (a *countedAccessor[T]) Value() T { return a.value }

func (a *countedAccessor[T]) Set(value T) {
	a.value = value
	a.writes++
}

// TestEveryAccessorOwnerObservesCallerTransitions is both the ownership contract and
// its coverage gate. The case names are checked against every exported production
// struct that accepts an Accessor, so a new controlled component cannot inherit only
// the convenient write half of the contract.
func TestEveryAccessorOwnerObservesCallerTransitions(t *testing.T) {
	cases := map[string]func(*testing.T){
		"Confirm": func(t *testing.T) {
			t.Helper()
			// The owner starts at the value a private copy would not hold, and then
			// moves to the one it would. A shadow copy answers false throughout, so
			// only reading through the accessor satisfies both.
			value := &countedAccessor[bool]{value: true}
			field := &headless.Confirm{Value: value}
			if !field.Answer() {
				t.Fatal("confirmation did not read its initial owner value")
			}
			value.value = false
			if field.Answer() {
				t.Fatal("confirmation answered from a copy of its owner value")
			}
			value.value = true
			field.Say(false)
			if value.value || value.writes != 1 || field.Answer() {
				t.Fatalf("answer=%t writes=%d, want an owner-written yes followed by one no", value.value, value.writes)
			}
		},
		"Dialog": func(t *testing.T) {
			t.Helper()
			value := &countedAccessor[bool]{}
			stack := &headless.Stack{}
			dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Open: value, Content: &panel{}})
			value.value = true
			if !dialog.Open() || !dialog.Sync() || dialog.Sync() || stack.Depth() == 0 {
				t.Fatal("dialog did not settle one owner-written open transition")
			}
			value.value = false
			if !dialog.Sync() || dialog.Sync() || stack.Depth() != 0 {
				t.Fatal("dialog did not settle one owner-written close transition")
			}
		},
		"MultiSelect": func(t *testing.T) {
			t.Helper()
			value := &countedAccessor[[]string]{value: []string{"a"}}
			field := &headless.MultiSelect[string]{Value: value}
			field.SetOptions(headless.Options("a", "b"))
			_ = field.Taken()
			value.value = []string{"b"}
			if got := field.Taken(); !slices.Equal(got, []string{"b"}) || value.writes != 0 {
				t.Fatalf("taken=%v writes=%d, want caller-owned b without a rewrite", got, value.writes)
			}
			if err := field.Reply("a"); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(value.value, []string{"a"}) || value.writes != 1 {
				t.Fatalf("binding=%v writes=%d, want one transition back to a", value.value, value.writes)
			}
		},
		"Select": func(t *testing.T) {
			t.Helper()
			value := &countedAccessor[string]{value: "a"}
			field := &headless.Select[string]{Value: value}
			field.SetOptions(headless.Options("a", "b"))
			_, _ = field.Chosen()
			value.value = "b"
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			chosen, ok := field.Chosen()
			if !ok || chosen.Value != "b" || value.value != "b" || value.writes != 0 {
				t.Fatalf("choice=%+v ok=%t binding=%q writes=%d, want caller-owned b", chosen, ok, value.value, value.writes)
			}
			if err := field.Reply("a"); err != nil {
				t.Fatal(err)
			}
			if value.value != "a" || value.writes != 1 {
				t.Fatalf("binding=%q writes=%d, want one spoken transition to a", value.value, value.writes)
			}
		},
		"Slider": func(t *testing.T) {
			t.Helper()
			value := &countedAccessor[int]{value: 2}
			slider := headless.NewSlider(headless.SliderConfig{Value: value, Minimum: 0, Maximum: 10})
			value.value = 7
			if slider.Value() != 7 || slider.Sync() {
				t.Fatal("slider did not read a valid owner transition directly")
			}
			value.value = 99
			if !slider.Sync() || slider.Sync() || value.value != 10 || value.writes != 1 {
				t.Fatalf("value=%d writes=%d, want one clamp to 10", value.value, value.writes)
			}
		},
		"Tabs": func(t *testing.T) {
			t.Helper()
			value := &countedAccessor[int]{}
			tabs := headless.NewTabs(headless.TabsConfig{
				Items: []headless.Tab{{Title: "a"}, {Title: "b"}}, Selection: value,
			})
			value.value = 1
			if tabs.Selected() != 1 || !tabs.Sync() || tabs.Sync() {
				t.Fatal("tabs did not settle one owner-written selection transition")
			}
		},
		"Text": func(t *testing.T) {
			t.Helper()
			value := &countedAccessor[string]{value: "a"}
			field := &headless.Text{Value: value}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			value.value = "b"
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := field.Editor().Text(); got != "b" || value.writes != 0 {
				t.Fatalf("text=%q writes=%d, want caller-owned b without a rewrite", got, value.writes)
			}
			if !field.Do(headless.Undo) || field.Editor().Text() != "b" || value.writes != 0 {
				t.Fatal("undo restored state from before an owner-written replacement")
			}
		},
	}

	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, cases[name])
	}
	assertAccessorOwnersCovered(t, cases)
}

func assertAccessorOwnersCovered(t *testing.T, covered map[string]func(*testing.T)) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	set := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typed, ok := specification.(*ast.TypeSpec)
				if !ok || !typed.Name.IsExported() || !structHasAccessor(typed.Type) {
					continue
				}
				found[strings.TrimSuffix(typed.Name.Name, "Config")] = true
			}
		}
	}
	for owner := range found {
		if covered[owner] == nil {
			t.Errorf("controlled owner %s has no caller-transition contract case", owner)
		}
	}
	for owner := range covered {
		if !found[owner] {
			t.Errorf("controlled owner case %s is stale", owner)
		}
	}
}

func structHasAccessor(expression ast.Expr) bool {
	structure, ok := expression.(*ast.StructType)
	if !ok {
		return false
	}
	for _, field := range structure.Fields.List {
		indexed, ok := field.Type.(*ast.IndexExpr)
		if !ok {
			continue
		}
		name, ok := indexed.X.(*ast.Ident)
		if ok && name.Name == "Accessor" {
			return true
		}
	}
	return false
}

func TestControlledScalarsWriteOnlyTransitions(t *testing.T) {
	t.Run("tabs", func(t *testing.T) {
		selection := &countedAccessor[int]{value: 1}
		tabs := headless.NewTabs(headless.TabsConfig{
			Items:     []headless.Tab{{Title: "one"}, {Title: "two"}},
			Selection: selection,
		})
		if selection.writes != 0 {
			t.Fatalf("construction rewrote a valid selection %d times", selection.writes)
		}
		tabs.Select(1)
		if selection.writes != 0 {
			t.Fatalf("selecting the visible tab wrote %d times", selection.writes)
		}
		tabs.Select(0)
		tabs.Select(0)
		if selection.value != 0 || selection.writes != 1 {
			t.Fatalf("selection = %d after %d writes, want 0 after one", selection.value, selection.writes)
		}
	})

	t.Run("tabs clamp invalid caller state", func(t *testing.T) {
		selection := &countedAccessor[int]{value: 99}
		tabs := headless.NewTabs(headless.TabsConfig{
			Items:     []headless.Tab{{Title: "one"}, {Title: "two"}},
			Selection: selection,
		})
		if tabs.Selected() != 1 || selection.value != 1 || selection.writes != 1 {
			t.Fatalf("selected=%d binding=%d writes=%d, want one clamped write to 1",
				tabs.Selected(), selection.value, selection.writes)
		}
	})

	t.Run("slider", func(t *testing.T) {
		value := &countedAccessor[int]{value: 5}
		slider := headless.NewSlider(headless.SliderConfig{Value: value, Minimum: 0, Maximum: 10})
		if value.writes != 0 || slider.Set(5) {
			t.Fatalf("unchanged slider wrote %d times", value.writes)
		}
		if !slider.Set(6) || slider.Set(6) || value.value != 6 || value.writes != 1 {
			t.Fatalf("slider=%d writes=%d, want 6 after one", value.value, value.writes)
		}
	})

	t.Run("slider reports the value its owner accepted", func(t *testing.T) {
		value := &rejectingIntAccessor{value: 5}
		slider := headless.NewSlider(headless.SliderConfig{Value: value, Minimum: 0, Maximum: 10})
		if slider.Set(6) {
			t.Fatal("Set reported a change the state owner rejected")
		}
		if value.value != 5 || value.writes != 1 || slider.Value() != 5 {
			t.Fatalf("value=%d writes=%d slider=%d, want one rejected write and value 5",
				value.value, value.writes, slider.Value())
		}
	})

	t.Run("dialog", func(t *testing.T) {
		open := &countedAccessor[bool]{}
		dialog := headless.NewDialog(headless.DialogConfig{
			Stack: &headless.Stack{}, Open: open, Content: &panel{},
		})
		dialog.Show()
		dialog.Show()
		dialog.Dismiss()
		dialog.Dismiss()
		if open.value || open.writes != 2 {
			t.Fatalf("open=%t after %d writes, want one open and one close", open.value, open.writes)
		}
	})
}

func TestControlledFieldsDoNotTurnHandledNoOpsIntoAssignments(t *testing.T) {
	t.Run("text reply", func(t *testing.T) {
		value := &countedAccessor[string]{value: "same"}
		field := &headless.Text{Value: value}
		if err := field.Reply("same"); err != nil {
			t.Fatal(err)
		}
		if value.writes != 0 {
			t.Fatalf("an unchanged reply wrote %d times", value.writes)
		}
		if err := field.Reply("different"); err != nil {
			t.Fatal(err)
		}
		if value.value != "different" || value.writes != 1 {
			t.Fatalf("value=%q writes=%d, want one changed reply", value.value, value.writes)
		}
	})

	t.Run("single choice", func(t *testing.T) {
		value := &countedAccessor[string]{value: "a"}
		field := &headless.Select[string]{Value: value}
		field.SetOptions(headless.Options("a", "b"))
		if chosen, ok := field.Chosen(); !ok || chosen.Value != "a" {
			t.Fatalf("initial choice = %+v, %t", chosen, ok)
		}
		if !field.Handle(input.Key{Code: input.Up}) || value.writes != 0 {
			t.Fatalf("handled boundary movement wrote %d times", value.writes)
		}
		if !field.Handle(input.Key{Code: input.Down}) || value.value != "b" || value.writes != 1 {
			t.Fatalf("choice=%q writes=%d, want b after one", value.value, value.writes)
		}
		if !field.Do(headless.SelectNext) || value.writes != 1 {
			t.Fatalf("handled action at the boundary wrote %d times", value.writes)
		}
		if err := field.Reply("b"); err != nil {
			t.Fatal(err)
		}
		if value.writes != 1 {
			t.Fatalf("repeating the stored choice wrote %d total times", value.writes)
		}
	})

	t.Run("single choice settles an unmatched initial value once", func(t *testing.T) {
		value := &countedAccessor[string]{value: "missing"}
		field := &headless.Select[string]{Value: value}
		field.SetOptions(headless.Options("a", "b"))
		if chosen, ok := field.Chosen(); !ok || chosen.Value != "a" || value.writes != 0 {
			t.Fatalf("choice=%+v ok=%t writes=%d before validation", chosen, ok, value.writes)
		}
		if err := field.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := field.Validate(); err != nil {
			t.Fatal(err)
		}
		if value.value != "a" || value.writes != 1 {
			t.Fatalf("choice=%q writes=%d, want one settlement to a", value.value, value.writes)
		}
	})

	t.Run("multiple choice reply", func(t *testing.T) {
		value := &countedAccessor[[]string]{value: []string{"a"}}
		field := &headless.MultiSelect[string]{Value: value}
		field.SetOptions(headless.Options("a", "b"))
		if got := field.Taken(); len(got) != 1 || got[0] != "a" {
			t.Fatalf("initial choices = %v", got)
		}
		if err := field.Reply("a"); err != nil {
			t.Fatal(err)
		}
		if value.writes != 0 {
			t.Fatalf("an unchanged reply wrote %d times", value.writes)
		}
		if err := field.Reply("b"); err != nil {
			t.Fatal(err)
		}
		if len(value.value) != 1 || value.value[0] != "b" || value.writes != 1 {
			t.Fatalf("choices=%v writes=%d, want b after one", value.value, value.writes)
		}
	})

	t.Run("multiple choice canonicalizes initial caller state", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			value  []string
			want   []string
			writes int
		}{
			{"already canonical", []string{"a", "b"}, []string{"a", "b"}, 0},
			{"option order", []string{"b", "a"}, []string{"a", "b"}, 1},
			{"duplicate", []string{"a", "a"}, []string{"a"}, 1},
			{"unknown choice", []string{"missing", "b"}, []string{"b"}, 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				value := &countedAccessor[[]string]{value: tc.value}
				field := &headless.MultiSelect[string]{Value: value}
				field.SetOptions(headless.Options("a", "b"))
				if got := field.Taken(); !slices.Equal(got, tc.want) {
					t.Fatalf("taken = %v, want %v", got, tc.want)
				}
				if !slices.Equal(value.value, tc.want) || value.writes != tc.writes {
					t.Fatalf("binding=%v writes=%d, want %v after %d", value.value, value.writes, tc.want, tc.writes)
				}
			})
		}
	})

	t.Run("confirmation", func(t *testing.T) {
		value := &countedAccessor[bool]{}
		field := &headless.Confirm{Value: value}
		field.Say(false)
		field.Do(headless.SelectNext)
		if value.writes != 0 {
			t.Fatalf("repeating no wrote %d times", value.writes)
		}
		field.Say(true)
		field.Say(true)
		if !value.value || value.writes != 1 {
			t.Fatalf("answer=%t writes=%d, want yes after one", value.value, value.writes)
		}
	})
}

func TestControlledMultipleChoiceSettlesOneOperationWithOneWrite(t *testing.T) {
	value := &countedAccessor[[]string]{value: []string{"a", "b"}}
	field := &headless.MultiSelect[string]{Value: value}
	field.SetOptions(headless.Options("a", "b"))
	_ = field.Taken()

	// The owner changed both order and cardinality before the limit transition. The
	// field must settle against the new limit directly, not publish a canonical value
	// for the old limit and then a second intermediate state.
	value.value = []string{"b", "a"}
	field.SetLimit(1)
	if !slices.Equal(value.value, []string{"a"}) || value.writes != 1 {
		t.Fatalf("binding=%v writes=%d, want one settled write to a", value.value, value.writes)
	}
}
