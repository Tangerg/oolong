package scene_test

import (
	"runtime"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestInterruptedInitialFocusRevokesProvisionalMount(t *testing.T) {
	for _, goexit := range []bool{false, true} {
		var element scene.Element
		var lease scene.Attachment
		element = scene.NewElement(scene.ElementConfig{Focusable: func() bool {
			if !lease.Target().IsZero() {
				return true
			}
			lease = element.Attachment()
			if goexit {
				runtime.Goexit()
			}
			panic("initial focus")
		}})
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				got := recover()
				if goexit && got != nil || !goexit && got != "initial focus" {
					t.Errorf("unexpected exit: %v", got)
				}
			}()
			_, _ = scene.NewRoot(scene.RootConfig{Element: element})
			t.Error("constructor returned")
		}()
		<-done
		select {
		case <-lease.Done():
		default:
			t.Fatal("unreturned root still owns the attachment")
		}
		if lease.Target().Valid() || element.Alive() {
			t.Fatal("aborted mount remained routable")
		}
		r := root(t, element)
		if !element.Focused() {
			t.Fatal("retry did not establish focus")
		}
		r.Close()
	}
}

func TestSettlementKeepsUndeliveredCallbacksInOrder(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		var order []string
		var r *scene.Root
		tail := scene.NewElement(scene.ElementConfig{Detached: func() { order = append(order, "new") }})
		first := scene.NewElement(scene.ElementConfig{Detached: func() {
			order = append(order, "first")
			if err := r.SetContent(scene.Element{}); err != nil {
				t.Fatal(err)
			}
			r.FlushNotifications()
			if interrupt {
				panic("interrupted")
			}
		}})
		second := scene.NewElement(scene.ElementConfig{Detached: func() { order = append(order, "second") }})
		g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first), item("second", second)}})
		r = root(t, g.Element())
		if err := r.SetContent(tail); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				got := recover()
				if interrupt && got != "interrupted" || !interrupt && got != nil {
					t.Errorf("bad exit: %v", got)
				}
			}()
			r.FlushNotifications()
		}()
		r.FlushNotifications()
		if !slices.Equal(order, []string{"first", "second", "new"}) {
			t.Errorf("callback delivery: %v", order)
		}
	}
}

func TestClosingCoveredModalKeepsOriginalReturnAddress(t *testing.T) {
	first, opener, lower, upper := control(), control(), control(), control()
	a, b := scene.NewGroup(scene.GroupConfig{Modal: true}), scene.NewGroup(scene.GroupConfig{Modal: true})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first.element), item("opener", opener.element), item("a", a.Element()), item("b", b.Element())}})
	r := root(t, g.Element())
	if err := opener.element.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		group    *scene.Group
		children []scene.Item
	}{{a, []scene.Item{item("low", lower.element)}}, {b, []scene.Item{item("high", upper.element)}}, {a, nil}, {b, nil}} {
		if err := step.group.Set(step.children...); err != nil {
			t.Fatal(err)
		}
	}
	if r.Focus() != opener.element.Target() {
		t.Fatal("closing both scopes lost the original opener")
	}
}

func TestReorderedModalRetainsFocusOfEachSurvivingLayer(t *testing.T) {
	a, b := scene.NewGroup(scene.GroupConfig{Modal: true}), scene.NewGroup(scene.GroupConfig{Modal: true})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.Element()), item("b", b.Element())}})
	r := root(t, g.Element())
	a2, b2 := control(), control()
	for _, opened := range []struct {
		layer *scene.Group
		pick  *routedControl
	}{{a, a2}, {b, b2}} {
		if err := opened.layer.Set(item("first", control().element), item("second", opened.pick.element)); err != nil {
			t.Fatal(err)
		}
		if err := opened.pick.element.RequestFocus(); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.Move(1, 0); err != nil {
		t.Fatal(err)
	}
	if r.Focus() != a2.element.Target() {
		t.Error("uncovered layer lost its focus")
	}
	if err := a.Set(); err != nil {
		t.Fatal(err)
	}
	if r.Focus() != b2.element.Target() {
		t.Error("reordered opener lost its focus")
	}
}
