package headless_test

import (
	"errors"
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestSelectRejectedUnavailableAnswerCannotAdvanceItsProjection(t *testing.T) {
	for _, navigation := range []string{"action", "key", "reply", "pointer"} {
		t.Run(navigation, func(t *testing.T) {
			value := &transformingAccessor[string]{value: "missing", transform: func(string) string { return "missing" }}
			field := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Value: value})
			field.SetOptions(headless.Options("a", "b"))
			switch navigation {
			case "action":
				field.Do(headless.SelectNext)
			case "key":
				field.Handle(input.Key{Code: input.Down})
			case "reply":
				if err := field.Reply("b"); !errors.Is(err, headless.ErrInvalidBinding) {
					t.Fatalf("rejected reply = %v, want invalid binding", err)
				}
			case "pointer":
				headless.NewRoot(field).Draw(grid.NewSurface(8, 2).View())
				field.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 1)})
			}
			chosen, ok := field.Chosen()
			if !ok || chosen.Value != "a" || value.value != "missing" {
				t.Fatalf("choice=%v exists=%t owner=%q, want the unchanged initial projection", chosen, ok, value.value)
			}
			if err := field.Validate(); !errors.Is(err, headless.ErrInvalidBinding) {
				t.Fatalf("rejected normalization = %v, want invalid binding", err)
			}
		})
	}
}

func TestSelectLocalChoicePreservesItsPositionAfterRemoval(t *testing.T) {
	field := &headless.Select[string]{Same: headless.Equal[string]}
	field.SetOptions(headless.Options("a", "b", "c"))
	field.Do(headless.SelectLast)
	field.SetOptions(headless.Options("a", "b"))
	chosen, ok := field.Chosen()
	if !ok || chosen.Value != "b" {
		t.Fatalf("local choice=%v exists=%t, want the clamped local position", chosen, ok)
	}
}

func TestSelectRejectedChoiceReadsTheAcceptedOwner(t *testing.T) {
	value := &transformingAccessor[string]{value: "b", transform: func(string) string { return "b" }}
	field := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Value: value})
	field.SetOptions(headless.Options("a", "b"))
	field.Do(headless.SelectFirst)
	chosen, ok := field.Chosen()
	if !ok || chosen.Value != "b" || value.writes != 1 {
		t.Fatalf("choice=%v exists=%t writes=%d, want the rejected proposal's accepted owner", chosen, ok, value.writes)
	}
}

func TestSelectCallerChoiceRevealsBothEndsWithoutWriting(t *testing.T) {
	value := &countedAccessor[string]{value: "a"}
	field := headless.NewSelect(headless.SelectConfig[string]{Same: headless.Equal[string], Value: value})
	field.SetOptions(headless.Options("a", "b", "c"))
	root := headless.NewRoot(field)
	for _, choice := range []string{"a", "c", "a"} {
		value.value = choice
		surface := grid.NewSurface(8, 1)
		root.Draw(surface.View())
		found := false
		for x := range 8 {
			cell, _ := surface.CellAt(x, 0)
			found = found || cell.Content() == choice
		}
		if !found || value.writes != 0 {
			t.Fatalf("caller choice %q was not revealed without a write; writes=%d", choice, value.writes)
		}
	}
}

func TestSelectBindingTransferRevokesLocalCursorVisibility(t *testing.T) {
	for _, transfer := range []string{"pending navigation", "replacement clamps old cursor", "return to local"} {
		t.Run(transfer, func(t *testing.T) {
			field := &headless.Select[string]{Same: headless.Equal[string]}
			field.SetOptions(headless.Options("a", "b", "c"))
			root := headless.NewRoot(field)
			root.Draw(grid.NewSurface(8, 1).View())
			field.Do(headless.SelectLast)
			value := "a"
			field.SetBinding(headless.Bind(&value))
			switch transfer {
			case "replacement clamps old cursor":
				root.Draw(grid.NewSurface(8, 1).View())
				field.SetOptions(headless.Options("a", "b"))
			case "return to local":
				value = "c"
				root.Draw(grid.NewSurface(8, 1).View())
				value = "a"
				field.SetBinding(nil)
			}
			surface := grid.NewSurface(8, 1)
			root.Draw(surface.View())
			found := false
			for x := range 8 {
				cell, _ := surface.CellAt(x, 0)
				found = found || cell.Content() == "a"
			}
			if !found || value != "a" {
				t.Fatalf("local cursor displaced controlled a; owner=%q", value)
			}
		})
	}
}
