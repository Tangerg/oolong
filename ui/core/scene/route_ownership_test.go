package scene_test

import (
	"errors"
	"image"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestAncestorCannotClaimAfterTheHitContentIsReplaced(t *testing.T) {
	for _, returned := range []bool{false, true} {
		id := "first"
		armed := true
		var focusErr, captureErr error
		var drags int
		child := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Semantics: func() scene.Semantic { return scene.Semantic{ContentID: id} }, Handle: func(scene.Event) scene.EventResult { return scene.EventResult{} }})
		parent := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Focusable: func() bool { return true }, Preview: func(e scene.Event) scene.EventResult {
			m, ok := e.Input().(input.Mouse)
			if !ok {
				return scene.EventResult{}
			}
			if armed && m.Action == input.MouseDown {
				armed = false
				id = "replacement"
				child.Invalidate()
				if !returned {
					focusErr = e.RequestFocus()
					captureErr = e.Capture()
				}
				return scene.EventResult{Consumed: true, Focus: returned, Capture: returned}
			}
			if m.Action == input.MouseDrag {
				drags++
			}
			return scene.EventResult{}
		}}, Items: []scene.Item{item("leaf", child)}})
		r := root(t, parent.Element())
		if err := child.RequestFocus(); err != nil {
			t.Fatal(err)
		}
		draw(t, r, 3, 1, 0)
		r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
		r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(8, 0)})
		if r.Focus() != child.Target() || drags != 0 || !returned && (!errors.Is(focusErr, scene.ErrStale) || !errors.Is(captureErr, scene.ErrStale)) {
			t.Errorf("returned=%v: focus=%v drags=%d errors=(%v,%v)", returned, r.Focus(), drags, focusErr, captureErr)
		}
	}
}

func TestPointerChecksEachReceiverAndTheArrivalOfCapturedRelease(t *testing.T) {
	id := "old"
	var stalePreviews, staleReleases, cancels int
	c := scene.NewElement(scene.ElementConfig{Handle: func(e scene.Event) scene.EventResult {
		m := e.Input().(input.Mouse)
		if m.Action == input.MouseUp {
			staleReleases++
		}
		if m.Action == input.MouseCancel {
			cancels++
		}
		return scene.EventResult{Consumed: true, Capture: m.Action == input.MouseDown}
	}})
	p := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{ContentID: id} }, Preview: func(scene.Event) scene.EventResult {
		if id == "new" {
			stalePreviews++
		}
		return scene.EventResult{}
	}}, Items: []scene.Item{item("c", c)}})
	r := root(t, p.Element())
	draw(t, r, 4, 1, 0)
	id = "new"
	p.Element().Invalidate()
	r.Handle(input.Mouse{Action: input.MouseMove})
	if stalePreviews != 0 {
		t.Error("parent preview received coordinates from its other content")
	}
	draw(t, r, 4, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.HandleInput(r.CaptureInput(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, At: time.Unix(1, 0)}, func(time.Time) (uint64, bool) { return 0, true }))
	if staleReleases != 0 || cancels != 1 {
		t.Errorf("out-of-history release: releases=%d cancels=%d", staleReleases, cancels)
	}
}

func TestKeyboardFallbackUsesTheActiveModalWithoutInventingFocus(t *testing.T) {
	var inside, outside int
	modal := scene.NewGroup(scene.GroupConfig{Modal: true, Element: scene.ElementConfig{Handle: func(scene.Event) scene.EventResult { inside++; return scene.EventResult{Consumed: true} }}, Items: []scene.Item{item("text", scene.NewElement(scene.ElementConfig{}))}})
	r := root(t, scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Preview: func(scene.Event) scene.EventResult { outside++; return scene.EventResult{Consumed: true} }}, Items: []scene.Item{item("modal", modal.Element())}}).Element())
	if !r.Focus().IsZero() {
		t.Fatal("unexpected focus")
	}
	r.Handle(input.Key{Code: input.Esc})
	if inside != 1 || outside != 0 {
		t.Errorf("modal fallback: inside=%d outside=%d", inside, outside)
	}
}

func TestGestureCannotTransferToAReplacementBodyAfterRepaint(t *testing.T) {
	id := "first"
	ups, cancels := 0, 0
	c := scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{ContentID: id} }, Handle: func(e scene.Event) scene.EventResult {
		m := e.Input().(input.Mouse)
		if m.Action == input.MouseCancel {
			cancels++
		}
		if m.Action == input.MouseUp {
			ups++
		}
		return scene.EventResult{Consumed: true, Capture: m.Action == input.MouseDown}
	}})
	r := root(t, c)
	draw(t, r, 2, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	id = "second"
	c.Invalidate()
	draw(t, r, 2, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
	if ups != 0 || cancels != 1 {
		t.Fatalf("replacement inherited press: up=%d cancel=%d", ups, cancels)
	}
}

func TestInputReadsOnlyItsScenesProjectionIncludingNil(t *testing.T) {
	var nilValue scene.Snapshot[any]
	var elsewhere scene.Snapshot[int]
	other := root(t, scene.NewElement(scene.ElementConfig{Paint: func(f scene.Frame) { elsewhere.Stage(f, 42) }}))
	draw(t, other, 2, 1, 0)
	calls := 0
	e := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Paint: func(f scene.Frame) { nilValue.Stage(f, nil) }, Handle: func(e scene.Event) scene.EventResult {
		if m, ok := e.Input().(input.Mouse); ok && m.Action != input.MouseDown {
			return scene.EventResult{}
		}
		calls++
		if x, ok := e.Read(&nilValue); !ok || x != nil {
			t.Error("staged nil reported absent")
		}
		if x, ok := e.Read(&elsewhere); ok {
			t.Errorf("read foreign projection %d", x)
		}
		return scene.EventResult{Consumed: true}
	}})
	r := root(t, e)
	draw(t, r, 2, 1, 0)
	r.Handle(input.Key{Rune: 'x'})
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	if calls != 2 {
		t.Fatalf("deliveries=%d", calls)
	}
}

func TestOnePressCannotStartTwoIndependentCaptures(t *testing.T) {
	var parentDrags, childPresses int
	c := scene.NewElement(scene.ElementConfig{Handle: func(e scene.Event) scene.EventResult {
		if m := e.Input().(input.Mouse); m.Action == input.MouseDown {
			childPresses++
			return scene.EventResult{Consumed: true, Capture: true}
		}
		return scene.EventResult{}
	}})
	g := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{Preview: func(e scene.Event) scene.EventResult {
		if m := e.Input().(input.Mouse); m.Action == input.MouseDown {
			if err := e.Capture(); err != nil {
				t.Error(err)
			}
		}
		return scene.EventResult{}
	}, Handle: func(e scene.Event) scene.EventResult {
		if m := e.Input().(input.Mouse); m.Action == input.MouseDrag {
			parentDrags++
		}
		return scene.EventResult{Consumed: true}
	}}, Items: []scene.Item{item("c", c)}})
	r := root(t, g.Element())
	draw(t, r, 2, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(5, 1)})
	if parentDrags != 1 || childPresses != 0 {
		t.Fatalf("claimed press reached %d children, its owner saw %d drags", childPresses, parentDrags)
	}
}

func TestOnlyTheLatestFocusGainCanReachTheSameAttachment(t *testing.T) {
	gains := 0
	a := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, FocusChanged: func(v bool) {
		if v {
			gains++
		}
	}})
	b := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a), item("b", b)}}).Element())
	if err := b.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if err := a.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	r.FlushNotifications()
	if gains != 1 {
		t.Fatalf("delivered %d gains for the current focus", gains)
	}
}

func TestInterceptedReleaseSettlesTheCapturedGesture(t *testing.T) {
	for _, mode := range []string{"consume", "panic", "new press"} {
		t.Run(mode, func(t *testing.T) {
			var r *scene.Root
			a := control()
			intercept := true
			group := scene.NewGroup(scene.GroupConfig{
				Items: []scene.Item{item("a", a.element)},
				Element: scene.ElementConfig{Preview: func(e scene.Event) scene.EventResult {
					m, ok := e.Input().(input.Mouse)
					if !ok || m.Action != input.MouseUp || !intercept {
						return scene.EventResult{}
					}
					intercept = false
					switch mode {
					case "panic":
						panic("intercepted")
					case "new press":
						r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
					}
					return scene.EventResult{Consumed: true}
				}},
			})
			r = root(t, group.Element())
			draw(t, r, 4, 1, 0)
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
			release := input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft}
			func() {
				defer func() {
					failure := recover()
					if mode == "panic" && failure != "intercepted" || mode != "panic" && failure != nil {
						t.Fatalf("preview failure = %v", failure)
					}
				}()
				r.Handle(release)
			}()
			r.Handle(release)
			want := []input.MouseAction{input.MouseDown, input.MouseCancel}
			if mode == "new press" {
				want = append(want, input.MouseDown, input.MouseUp)
			}
			if got := mouseActions(a.events); !slices.Equal(got, want) {
				t.Fatalf("capture events = %v, want %v", got, want)
			}
		})
	}
}
