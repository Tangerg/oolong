package scene_test

import (
	"errors"
	"image"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestModalBoundsBothPreviewAndBubble(t *testing.T) {
	var outside, delivered int
	modal := scene.NewGroup(scene.GroupConfig{Modal: true, Items: []scene.Item{{Element: scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Handle: func(scene.Event) scene.EventResult { delivered++; return scene.EventResult{Consumed: true} }})}}})
	g := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Preview: func(scene.Event) scene.EventResult { outside++; return scene.EventResult{Consumed: true} }}, Items: []scene.Item{{Element: modal.Element()}}})
	r := root(t, g.Element())
	r.Handle(input.Key{Code: input.Esc})
	if outside != 0 || delivered != 1 {
		t.Fatalf("outer shortcut=%d, modal handler=%d", outside, delivered)
	}
}

func TestPointerRouteRequiresTheOriginalContentAtDelivery(t *testing.T) {
	for _, mutate := range []string{"normal", "replace", "preview", "geometry"} {
		t.Run(mutate, func(t *testing.T) {
			id, rev := "a", uint64(1)
			delivered := 0
			leaf := scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{ContentID: id, ContentRevision: rev} }, Handle: func(scene.Event) scene.EventResult { delivered++; return scene.EventResult{Consumed: true} }})
			var r *scene.Root
			g := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Preview: func(scene.Event) scene.EventResult {
				switch mutate {
				case "preview":
					rev++
					leaf.Invalidate()
				case "geometry":
					r.InvalidateGeometry()
				}
				return scene.EventResult{}
			}}, Items: []scene.Item{item("leaf", leaf)}})
			r = root(t, g.Element())
			draw(t, r, 10, 2, 0)
			click := capture(r, input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
			if mutate == "replace" {
				id = "b"
				leaf.Invalidate()
			}
			r.HandleInput(click)
			want := 0
			if mutate == "normal" {
				want = 1
			}
			if delivered != want {
				t.Fatalf("delivered %d, want %d", delivered, want)
			}
		})
	}
}

func TestLayoutCannotChangeStagedGeometry(t *testing.T) {
	var s scene.Snapshot[int]
	var staged *scene.Staged[int]
	observed := false
	g := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Paint: func(f scene.Frame) { staged = s.Stage(f, 1) }}, Layout: func(image.Point, []scene.Item) []image.Rectangle {
		func() {
			defer func() { err, _ := recover().(error); observed = errors.Is(err, scene.ErrBusy) }()
			staged.Set(2)
		}()
		return nil
	}})
	r := root(t, g.Element())
	draw(t, r, 2, 1, 0)
	if !observed || s.Value() != 1 {
		t.Fatalf("layout changed paint state: denied=%v, value=%d", observed, s.Value())
	}
}

func TestNotificationCannotRouteInputIntoAnUnsettledFrame(t *testing.T) {
	var r *scene.Root
	var candidate *scene.Prepared
	calls := 0
	node := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, FocusChanged: func(focus bool) {
		if focus {
			var err error
			candidate, err = r.Prepare(grid.NewSurface(1, 1).View())
			if err != nil {
				t.Fatal(err)
			}
		}
	}, Handle: func(scene.Event) scene.EventResult { calls++; return scene.EventResult{Consumed: true} }})
	r = root(t, node)
	r.Handle(input.Key{Code: input.Enter})
	candidate.Abort()
	if calls != 0 {
		t.Fatal("input ran in the pending-candidate phase")
	}
}

func TestIntermediateWatermarkPublishesExactlyItsAdmittedLayout(t *testing.T) {
	var row scene.Snapshot[uint64]
	var value, hit uint64
	leaf := scene.NewElement(scene.ElementConfig{Paint: func(f scene.Frame) { row.Stage(f, value) }, Handle: func(e scene.Event) scene.EventResult {
		hit, _ = e.Read(&row)
		return scene.EventResult{Consumed: true}
	}})
	r := root(t, leaf)
	for i := uint64(1); i <= 3; i++ {
		value = i
		draw(t, r, 2, 1, i)
	}
	for _, ack := range []uint64{1, 3} {
		r.Wrote(ack)
		r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
		if hit != ack || r.Progress().WrittenFrame != ack {
			t.Errorf("ack %d: used %d, progress=%+v", ack, hit, r.Progress())
		}
	}
}

func TestInvalidatedBoundsAreUnavailableToAutomation(t *testing.T) {
	r := root(t, scene.NewElement(scene.ElementConfig{Key: "button"}))
	draw(t, r, 9, 3, 0)
	r.InvalidateGeometry()
	o, err := r.Observe(scene.Query{})
	if err != nil || len(o.Nodes) != 1 || o.Nodes[0].HasBounds {
		t.Fatalf("expired bounds: %+v, %v", o, err)
	}
}

func TestClosingRootFinishesEveryRevokedLifetime(t *testing.T) {
	run := func(exit func(), want any) {
		t.Helper()
		count := 0
		first := scene.NewElement(scene.ElementConfig{Detached: exit})
		last := scene.NewElement(scene.ElementConfig{Detached: func() { count++ }})
		g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first), item("last", last)}})
		r := root(t, g.Element())
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if got := recover(); got != want {
					t.Errorf("close panic: %v", got)
				}
			}()
			r.Close()
		}()
		<-done
		if count != 1 {
			t.Errorf("terminal cleanup calls=%d", count)
		}
		if err := g.Set(); err != nil {
			t.Fatal(err)
		}
		if n, err := scene.NewRoot(scene.RootConfig{Element: last}); err != nil {
			t.Errorf("closed owner retained element: %v", err)
		} else {
			n.Close()
		}
	}
	run(func() { panic("cleanup") }, "cleanup")
	run(runtime.Goexit, nil)
}

func TestGeometryRevocationPrecedesCaptureCancellation(t *testing.T) {
	var r *scene.Root
	e := scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
		mouse := event.Input().(input.Mouse)
		if mouse.Action == input.MouseCancel {
			r.Close()
		}
		return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
	}})
	r = root(t, e)
	draw(t, r, 1, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.InvalidateGeometry()
}

func TestPendingFrameBudgetRefusesBeforePainting(t *testing.T) {
	paints := 0
	r, err := scene.NewRoot(scene.RootConfig{MaxPendingFrames: 2, Element: scene.NewElement(scene.ElementConfig{Paint: func(scene.Frame) { paints++ }})})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	draw(t, r, 1, 1, 1)
	draw(t, r, 1, 1, 2)
	if candidate, busy := r.Prepare(grid.NewSurface(1, 1).View()); !errors.Is(busy, scene.ErrBusy) || candidate != nil || paints != 2 {
		t.Fatal("capacity refusal did not precede paint", candidate, busy, paints)
	}
	r.Wrote(1)
	draw(t, r, 1, 1, 3)
	if paints != 3 {
		t.Fatal("acknowledgement did not release capacity")
	}
}

func TestWriterProgressDoesNotSettleAnotherPreparedFrame(t *testing.T) {
	r := root(t, scene.NewElement(scene.ElementConfig{}))
	draw(t, r, 1, 1, 1)
	pending, err := r.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	r.Wrote(1)
	if r.Progress().WrittenFrame != 1 || r.Progress().LayoutRevision != 1 {
		t.Fatalf("write progress settled a foreign obligation: %+v", r.Progress())
	}
	pending.Commit(2)
	r.Wrote(2)
	if r.Progress().WrittenFrame != 2 || r.Progress().LayoutRevision != 2 {
		t.Fatalf("candidate lost its settlement: %+v", r.Progress())
	}
}

func TestCaptureCancellationCannotResumePointerRoutingInAPreparedFrame(t *testing.T) {
	var r *scene.Root
	var pending *scene.Prepared
	presses := 0
	e := scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{} }, Handle: func(e scene.Event) scene.EventResult {
		action := e.Input().(input.Mouse).Action
		if action == input.MouseDown {
			presses++
			return scene.EventResult{Capture: true, Consumed: true}
		}
		if action == input.MouseCancel {
			var err error
			pending, err = r.Prepare(grid.NewSurface(1, 1).View())
			if err != nil {
				t.Fatal(err)
			}
		}
		return scene.EventResult{}
	}})
	r = root(t, e)
	draw(t, r, 1, 1, 0)
	press := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}
	r.Handle(press)
	r.Handle(press)
	if pending == nil || presses != 1 {
		t.Fatalf("prepared=%t, presses=%d", pending != nil, presses)
	}
	pending.Abort()
}

func TestPointerCannotClaimNewContentFromInsideItsHandler(t *testing.T) {
	for _, change := range []string{"content", "geometry"} {
		t.Run(change, func(t *testing.T) {
			var r *scene.Root
			id := "original"
			var focus, capture error
			current := true
			button := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Semantics: func() scene.Semantic { return scene.Semantic{ContentID: id} }, Handle: func(e scene.Event) scene.EventResult {
				if m, ok := e.Input().(input.Mouse); !ok || m.Action != input.MouseDown {
					return scene.EventResult{}
				}
				if change == "content" {
					id = "replacement"
					r.Changed()
				} else {
					r.InvalidateGeometry()
				}
				current = e.CurrentLayout()
				focus = e.RequestFocus()
				capture = e.Capture()
				return scene.EventResult{Consumed: true}
			}})
			r = root(t, button)
			draw(t, r, 2, 1, 0)
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
			if !errors.Is(focus, scene.ErrStale) || !errors.Is(capture, scene.ErrStale) || (change == "geometry" && current) {
				t.Errorf("current=%t, focus=%v, capture=%v", current, focus, capture)
			}
		})
	}
}
