package scene_test

import (
	"errors"
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestReleaseDuringPressDoesNotRestoreCapture(t *testing.T) {
	a := control()
	r := root(t, a.element)
	draw(t, r, 10, 1, 0)
	a.onEvent = func(event scene.Event) {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown {
			a.onEvent = nil
			r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
		}
	}
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(0, 9)})
	if got := mouseActions(a.events); !slices.Equal(got, []input.MouseAction{input.MouseDown}) {
		t.Fatalf("older handler revived a released gesture: %v", got)
	}
}

func TestCancellationMayStartANewerCapture(t *testing.T) {
	a, b := control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element)}})
	r := root(t, g.Element())
	draw(t, r, 10, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	a.onEvent = func(event scene.Event) {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseCancel {
			a.onEvent = nil
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: image.Pt(0, 1)})
		}
	}
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonRight, Pos: image.Pt(0, 8)})
	if got := mouseActions(b.events); !slices.Equal(got, []input.MouseAction{input.MouseDown, input.MouseDrag}) {
		t.Fatalf("old cancellation cleared newer capture: %v", got)
	}
}

func TestAbnormalCallbackDoesNotClearNewerCapture(t *testing.T) {
	a, b := control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element)}})
	r := root(t, g.Element())
	draw(t, r, 10, 2, 0)
	a.onEvent = func(event scene.Event) {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown {
			a.onEvent = nil
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: image.Pt(0, 1)})
			panic("interrupted callback")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("callback failure was suppressed")
			}
		}()
		r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	}()
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonRight, Pos: image.Pt(0, 8)})
	if got := mouseActions(b.events); !slices.Equal(got, []input.MouseAction{input.MouseDown, input.MouseDrag}) {
		t.Fatalf("unwind cleared newer capture: %v", got)
	}
}

func TestPurePhasesRejectOwnerMutationBeforeItChangesState(t *testing.T) {
	value := 1
	var e scene.Element
	mutate := func() error {
		if err := e.CheckMutation(); err != nil {
			return err
		}
		value++
		e.Invalidate()
		return nil
	}
	e = scene.NewElement(scene.ElementConfig{
		Paint: func(scene.Frame) {
			if err := mutate(); !errors.Is(err, scene.ErrBusy) {
				t.Fatalf("paint mutation: %v", err)
			}
		},
		Semantics: func() scene.Semantic {
			if err := mutate(); !errors.Is(err, scene.ErrBusy) {
				t.Fatalf("observe mutation: %v", err)
			}
			return scene.Semantic{Role: scene.RoleStatus}
		},
	})
	r := root(t, e)
	p, err := r.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	if err := mutate(); !errors.Is(err, scene.ErrBusy) {
		t.Fatalf("pending frame mutation: %v", err)
	}
	p.Commit(0)
	if _, err := r.Observe(scene.Query{}); err != nil {
		t.Fatal(err)
	}
	if value != 1 {
		t.Fatal("failed phase check corrupted model state")
	}
}

func TestFocusTransferAndRoundTripEndOldKeyPropagation(t *testing.T) {
	for _, roundTrip := range []bool{false, true} {
		t.Run(map[bool]string{false: "transfer", true: "round trip"}[roundTrip], func(t *testing.T) {
			a, b := control(), control()
			a.takes = false
			parentKeys := 0
			g := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Handle: func(scene.Event) scene.EventResult { parentKeys++; return scene.EventResult{Consumed: true} }}, Items: []scene.Item{item("a", a.element), item("b", b.element)}})
			r := root(t, g.Element())
			a.onEvent = func(scene.Event) {
				if err := b.element.RequestFocus(); err != nil {
					t.Fatal(err)
				}
				if roundTrip {
					if err := a.element.RequestFocus(); err != nil {
						t.Fatal(err)
					}
				}
			}
			if !r.Handle(input.Key{Code: input.Tab}) || parentKeys != 0 {
				t.Fatal("old key escaped a changed focus scope")
			}
			if a.element.Focused() != roundTrip {
				t.Fatal("old propagation overwrote the requested focus")
			}
		})
	}
}

func TestContentRevocationCannotClearACaptureStartedByItsCancelCallback(t *testing.T) {
	var r *scene.Root
	var revision uint64
	var released bool
	first := scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{ContentRevision: revision} }, Handle: func(event scene.Event) scene.EventResult {
		mouse, ok := event.Input.(input.Mouse)
		if !ok {
			return scene.EventResult{}
		}
		if mouse.Action == input.MouseCancel {
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(2, 1)})
		}
		return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
	}})
	second := scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
		mouse, ok := event.Input.(input.Mouse)
		if !ok {
			return scene.EventResult{}
		}
		if mouse.Action == input.MouseUp {
			released = true
		}
		return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
	}})
	group := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{{Element: first, Size: layout.Fixed(1)}, {Element: second, Size: layout.Fixed(1)}}})
	r = root(t, group.Element())
	draw(t, r, 10, 2, 1)
	r.Wrote(1)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(2, 0)})
	revision++
	r.Changed()
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(2, 0)})
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(2, 1)})
	if !released {
		t.Fatal("the obsolete release cleared the replacement capture")
	}
}
