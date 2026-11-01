package base_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestCollectionPreparationCannotMutateTheSelectionItProjects(t *testing.T) {
	denied := func(attempt func()) bool {
		var failure error
		func() { defer func() { failure, _ = recover().(error) }(); attempt() }()
		return errors.Is(failure, scene.ErrBusy)
	}
	t.Run("table", func(t *testing.T) {
		var table base.Table[int]
		table.SetItems([]int{3, 2, 1})
		table.SetLess(func(a, b, _ int) bool {
			if !denied(func() { table.Select(1) }) {
				t.Fatal("comparator moved the cursor before ordering committed")
			}
			return a < b
		})
		table.SortBy(0)
		if value, _ := table.Current(); value != 3 {
			t.Fatal(value)
		}
	})
	t.Run("filter", func(t *testing.T) {
		var filter base.Filter[string]
		filter.SetItems([]string{"alfa", "beta"}, func(s string) string {
			if !denied(func() { filter.Select(1) }) {
				t.Fatal("ranking moved the source cursor")
			}
			return s
		})
		filter.SetPattern("a")
	})
	t.Run("select", func(t *testing.T) {
		var s *base.Select[string]
		s = base.NewSelect(base.SelectConfig[string]{Same: func(a, b string) bool {
			if s != nil && !denied(func() { s.SetOptions(nil) }) {
				t.Fatal("comparison replaced its own option set")
			}
			return a == b
		}, Options: base.Options("one", "two")})
		s.Choose("two")
		if item, _ := s.Chosen(); item.Value != "two" {
			t.Fatal(item)
		}
	})
	t.Run("multi", func(t *testing.T) {
		var m *base.MultiSelect[string]
		m = base.NewMultiSelect(base.MultiSelectConfig[string]{Same: func(a, b string) bool {
			if m != nil && !denied(func() { m.Toggle() }) {
				t.Fatal("comparison changed selection while assembling it")
			}
			return a == b
		}, Options: base.Options("one", "two")})
		m.SetTaken([]string{"two"})
	})
}

func TestDialogResolverCannotCloseANewerOpening(t *testing.T) {
	var dialog *base.Dialog
	var keys keymap.Map
	keys.Bind("prefix", input.Chord{Rune: 'g'})
	keys.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	keys.Resolve = func(time.Duration, func()) func() { return func() { dialog.Dismiss(); dialog.Show() } }
	closes := 0
	dialog = base.NewDialog(base.DialogConfig{Initial: true, Keys: &keys, Content: scene.NewElement(scene.ElementConfig{}), Dismissed: func() { closes++ }})
	r := newTestRoot(dialog)
	r.Handle(input.Key{Rune: 'g'})
	dialog.Dismiss()
	if !dialog.Open() || closes != 1 {
		t.Fatalf("old dismissal replaced its successor: open=%t closes=%d", dialog.Open(), closes)
	}
}

type guardedVirtual struct{ measure func() }

func (guardedVirtual) Revision() uint64       { return 1 }
func (guardedVirtual) Len() int               { return 1 }
func (guardedVirtual) Key(int) base.ItemKey   { return "one" }
func (s guardedVirtual) Measure(int, int) int { s.measure(); return 1 }

func TestStandaloneQueriesUseTheSamePurePhaseAsSceneQueries(t *testing.T) {
	var b *base.Button
	b = base.NewButton(base.ButtonConfig{Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{}, nil
	}, Availability: func() scene.Availability {
		if err := b.Element().CheckMutation(); !errors.Is(err, scene.ErrBusy) {
			t.Errorf("button availability runs in writable phase: %v", err)
		}
		return scene.Availability{}
	}})
	b.Availability()
	var w *base.VirtualWindow
	w, err := base.NewVirtualWindow(base.VirtualConfig{Source: guardedVirtual{measure: func() {
		if e := w.Element().CheckMutation(); !errors.Is(e, scene.ErrBusy) {
			t.Errorf("virtual measurement runs in writable phase: %v", e)
		}
	}}, Build: func(base.ItemKey) scene.Element {
		if e := w.Element().CheckMutation(); e != nil {
			t.Errorf("virtual materialization lost owner phase: %v", e)
		}
		return scene.NewElement(scene.ElementConfig{})
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Resolve(10, 2, 0); err != nil {
		t.Fatal(err)
	}
}

type measuringCompletion struct {
	base.DefaultCompletionRenderer
	measure func()
}

func (r measuringCompletion) Width(candidate base.Candidate) int {
	r.measure()
	return r.DefaultCompletionRenderer.Width(candidate)
}

func TestCompletionCannotDismissItselfWhileComputingItsWidth(t *testing.T) {
	var c base.Completion
	c.Renderer = measuringCompletion{measure: func() { c.Dismiss() }}
	c.Offer(base.Token{}, []base.Candidate{{Text: "abc"}})
	var failure error
	func() { defer func() { failure, _ = recover().(error) }(); c.Width() }()
	if !errors.Is(failure, scene.ErrBusy) || !c.Open() {
		t.Fatalf("measurement changed its own offer: failure=%v open=%v", failure, c.Open())
	}
}
