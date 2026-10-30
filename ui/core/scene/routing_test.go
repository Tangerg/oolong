package scene_test

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type routedControl struct {
	element scene.Element
	takes   bool
	events  []input.Event
	focus   []bool
	onEvent func(scene.Event)
	onFocus func(bool)
	size    image.Point
}

func control() *routedControl {
	c := &routedControl{takes: true}
	c.element = scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		FocusChanged: func(has bool) {
			c.focus = append(c.focus, has)
			if c.onFocus != nil {
				c.onFocus(has)
			}
		},
		Handle: func(event scene.Event) scene.EventResult {
			c.events = append(c.events, event.Input)
			if c.onEvent != nil {
				c.onEvent(event)
			}
			mouse, pointer := event.Input.(input.Mouse)
			return scene.EventResult{Consumed: c.takes, Capture: c.takes && pointer && mouse.Action == input.MouseDown}
		},
		Paint: func(frame scene.Frame) { width, height := frame.Size(); c.size = image.Pt(width, height) },
	})
	return c
}

func mouseActions(events []input.Event) []input.MouseAction {
	var actions []input.MouseAction
	for _, event := range events {
		if mouse, ok := event.(input.Mouse); ok {
			actions = append(actions, mouse.Action)
		}
	}
	return actions
}

func TestKeysUseOneFocusAndOnlyDeclinedKeysReachParentPolicy(t *testing.T) {
	a, b := control(), control()
	g := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
			key, ok := event.Input.(input.Key)
			if ok && key.Is(input.Tab, 0) {
				return scene.EventResult{Consumed: event.FocusNext(false)}
			}
			return scene.EventResult{}
		}},
		Items: []scene.Item{item("a", a.element), item("b", b.element)},
	})
	r := root(t, g.Element())
	r.FlushNotifications()
	r.Handle(input.Key{Code: input.Tab})
	if !a.element.Focused() || len(b.events) != 0 {
		t.Fatal("parent stole a key the focused child consumed")
	}
	a.takes = false
	r.Handle(input.Key{Code: input.Tab})
	if !b.element.Focused() {
		t.Fatal("declined Tab did not reach traversal policy")
	}
	b.takes = false
	if r.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("unhandled key was reported consumed")
	}
}

func TestReorderAndUnrelatedChildrenDoNotReassignFocus(t *testing.T) {
	a, b, unrelated := control(), control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element)}})
	r := root(t, g.Element())
	r.FlushNotifications()
	if err := b.element.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	r.FlushNotifications()
	a.focus, b.focus = nil, nil
	if err := g.Set(item("new", unrelated.element), item("b", b.element), item("a", a.element)); err != nil {
		t.Fatal(err)
	}
	r.FlushNotifications()
	if !b.element.Focused() || len(a.focus) != 0 || len(b.focus) != 0 {
		t.Fatal("reorder manufactured a focus loss and regain")
	}
	items := g.Items()
	items[1].Element = a.element
	if g.Items()[1].Element != b.element {
		t.Fatal("child list snapshot mutated authoritative relations")
	}
}

func TestNestedCoordinatesAndClipsComeFromTheSameFrame(t *testing.T) {
	c := control()
	inner := scene.NewGroup(scene.GroupConfig{Layout: func(image.Point, []scene.Item) []image.Rectangle { return []image.Rectangle{image.Rect(-1, 1, 5, 4)} }, Items: []scene.Item{item("leaf", c.element)}})
	outer := scene.NewGroup(scene.GroupConfig{Layout: func(image.Point, []scene.Item) []image.Rectangle { return []image.Rectangle{image.Rect(3, 2, 7, 6)} }, Items: []scene.Item{item("inner", inner.Element())}})
	r := root(t, outer.Element())
	draw(t, r, 10, 7, 0)
	if r.Handle(input.Mouse{Action: input.MouseDown, Pos: image.Pt(2, 3)}) {
		t.Fatal("pointer escaped ancestor clip")
	}
	r.Handle(input.Mouse{Action: input.MouseDown, Pos: image.Pt(3, 3)})
	if len(c.events) != 1 {
		t.Fatalf("unexpected routes: %v", c.events)
	}
	if got := c.events[0].(input.Mouse).Pos; got != image.Pt(1, 0) {
		t.Fatalf("local coordinates: %v", got)
	}
	if c.size != image.Pt(6, 3) {
		t.Fatalf("clipping changed layout extent: %v", c.size)
	}
}

func TestHoverLeaveCannotActivateAnElementItRemoved(t *testing.T) {
	a, b := control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element)}})
	r := root(t, g.Element())
	draw(t, r, 10, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseMove})
	a.onEvent = func(event scene.Event) {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseLeave {
			a.onEvent = nil
			if err := g.Set(item("a", a.element)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !r.Handle(input.Mouse{Action: input.MouseDown, Pos: image.Pt(0, 1)}) {
		t.Fatal("interrupted route was allowed to propagate")
	}
	if len(b.events) != 0 {
		t.Fatalf("removed target received interrupted input: %v", b.events)
	}
}

func TestReentrantPointerRouteCannotResumeAnOlderPress(t *testing.T) {
	a, b := control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element)}})
	r := root(t, g.Element())
	draw(t, r, 10, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseMove})
	a.onEvent = func(event scene.Event) {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseLeave {
			a.onEvent = nil
			r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
		}
	}
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 1)})
	for _, action := range mouseActions(b.events) {
		if action != input.MouseLeave {
			t.Fatalf("older route continued after a newer input: %v", b.events)
		}
	}
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(0, 9)})
	if got := mouseActions(a.events); !slices.Equal(got, []input.MouseAction{input.MouseMove, input.MouseLeave, input.MouseDown, input.MouseDrag}) {
		t.Fatalf("new gesture did not keep ownership: %v", got)
	}
}

func TestHoverExitPreservesCaptureAndNewPressCancelsIt(t *testing.T) {
	a, b := control(), control()
	b.takes = false
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element)}})
	r := root(t, g.Element())
	draw(t, r, 10, 2, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(0, 1)})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(0, 4)})
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: image.Pt(0, 1)})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(0, 4)})
	if got := mouseActions(a.events); !slices.Equal(got, []input.MouseAction{input.MouseDown, input.MouseLeave, input.MouseDrag, input.MouseCancel}) {
		t.Fatalf("hover and capture were conflated: %v", got)
	}
}

func TestReplacementCannotReceiveTheRemainderOfCancelledGesture(t *testing.T) {
	old, replacement := control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("same", old.element)}})
	r := root(t, g.Element())
	draw(t, r, 10, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	if err := g.Set(item("same", replacement.element)); err != nil {
		t.Fatal(err)
	}
	draw(t, r, 10, 1, 0)
	for range 3 {
		r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft})
	}
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight})
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonNone})
	if len(replacement.events) != 0 {
		t.Fatalf("replacement inherited a retired gesture: %v", replacement.events)
	}
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	if len(replacement.events) != 1 {
		t.Fatal("new press could not create a new gesture")
	}
}

func TestFocusNotificationReentrySeesSettledState(t *testing.T) {
	a, b, c := control(), control(), control()
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a.element), item("b", b.element), item("c", c.element)}})
	r := root(t, g.Element())
	r.FlushNotifications()
	a.onFocus = func(has bool) {
		if !has {
			a.onFocus = nil
			if err := c.element.RequestFocus(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := b.element.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	r.FlushNotifications()
	r.FlushNotifications()
	if !c.element.Focused() || a.element.Focused() || b.element.Focused() {
		t.Fatal("notification reentry created two actual focus owners")
	}
	if !slices.Equal(c.focus, []bool{true}) {
		t.Fatalf("settled target was unnecessarily blurred: %v", c.focus)
	}
}

func TestGroupMeasuresChildrenAndKeepsExplicitZeroSlots(t *testing.T) {
	a, b := control(), control()
	if err := a.element.SetMeasure(func(axis layout.Axis, across int) int {
		if axis != layout.Down || across != 10 {
			t.Fatal("wrong measurement axis")
		}
		return 2
	}); err != nil {
		t.Fatal(err)
	}
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: a.element, Size: layout.Measured(0, 0)}, {Element: b.element, Size: layout.Flex(1)}}})
	r := root(t, g.Element())
	draw(t, r, 10, 5, 0)
	if a.size.Y != 2 || b.size.Y != 3 {
		t.Fatalf("measured allocation: %v %v", a.size, b.size)
	}
	if err := g.Set(scene.Item{Element: a.element, Size: layout.Fixed(0)}, scene.Item{Element: b.element, Size: layout.Flex(1)}); err != nil {
		t.Fatal(err)
	}
	draw(t, r, 10, 5, 0)
	if a.size.Y != 0 || b.size.Y != 5 {
		t.Fatalf("explicit zero allocation: %v %v", a.size, b.size)
	}
}

func TestAbortedReplacementDoesNotReviveRevokedAttachment(t *testing.T) {
	a, b := control(), control()
	r := root(t, a.element)
	draw(t, r, 10, 1, 0)
	if err := r.SetContent(b.element); err != nil {
		t.Fatal(err)
	}
	p, err := r.Prepare(grid.NewSurface(10, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	p.Abort()
	r.Handle(input.Mouse{Action: input.MouseDown})
	if len(a.events) != 0 || len(b.events) != 0 {
		t.Fatal("rejected output revived an old attachment or routed to invisible replacement")
	}
}

func TestPreviewUsesTheSameRouteAndMayConsumeBeforeTheFocusedPrimitive(t *testing.T) {
	var order []string
	leaf := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		Preview:   func(scene.Event) scene.EventResult { order = append(order, "leaf preview"); return scene.EventResult{} },
		Handle: func(scene.Event) scene.EventResult {
			order = append(order, "leaf handle")
			return scene.EventResult{Consumed: true}
		},
	})
	consume := false
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: leaf}}, Element: scene.ElementConfig{
		Preview: func(scene.Event) scene.EventResult {
			order = append(order, "parent preview")
			return scene.EventResult{Consumed: consume}
		},
	}})
	root, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	root.Handle(input.Key{Rune: 'a'})
	if got := strings.Join(order, "; "); got != "parent preview; leaf preview; leaf handle" {
		t.Fatal(got)
	}
	consume, order = true, nil
	root.Handle(input.Key{Rune: 'b'})
	if got := strings.Join(order, "; "); got != "parent preview" {
		t.Fatal(got)
	}
}

func TestPreviewCannotResumeAnAttachmentItRemoved(t *testing.T) {
	calls := 0
	leaf := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Handle: func(scene.Event) scene.EventResult {
		calls++
		return scene.EventResult{Consumed: true}
	}})
	var group *scene.Group
	group = scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: leaf}}, Element: scene.ElementConfig{Preview: func(scene.Event) scene.EventResult {
		if err := group.Set(); err != nil {
			t.Fatal(err)
		}
		return scene.EventResult{}
	}}})
	root, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	root.Handle(input.Key{Rune: 'a'})
	if calls != 0 {
		t.Fatal("removed target handled old input")
	}
}

func TestDecorativeAndInitiallyDisabledOverlaysDoNotStealPointerHits(t *testing.T) {
	backgroundPresses, overlayPresses := 0, 0
	background := scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown {
			backgroundPresses++
		}
		return scene.EventResult{Consumed: true}
	}})
	enabled := false
	overlay := scene.NewElement(scene.ElementConfig{Enabled: func() bool { return enabled }, Handle: func(event scene.Event) scene.EventResult {
		if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown {
			overlayPresses++
		}
		return scene.EventResult{Consumed: true}
	}})
	decoration := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { frame.Text(0, 0, "decoration", grid.Style{}) }})
	group := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: background}, {Element: overlay}, {Element: decoration}}})
	r := root(t, group.Element())
	draw(t, r, 20, 4, 1)
	r.Wrote(1)
	press := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}
	r.Handle(press)
	if backgroundPresses != 1 || overlayPresses != 0 {
		t.Fatal(backgroundPresses, overlayPresses)
	}
	enabled = true
	r.Changed()
	draw(t, r, 20, 4, 2)
	r.Wrote(2)
	enabled = false
	r.Changed()
	r.Handle(press)
	if backgroundPresses != 1 {
		t.Fatal("a newly disabled target retargeted its old visible hit to the background")
	}
}

func TestInputConsumptionDoesNotInventAModelChange(t *testing.T) {
	element := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Handle: func(scene.Event) scene.EventResult {
		return scene.EventResult{Consumed: true}
	}})
	r := root(t, element)
	before := r.Progress().ModelRevision
	if !r.Handle(input.Key{Rune: 'a'}) {
		t.Fatal("input was not consumed")
	}
	if after := r.Progress().ModelRevision; after != before {
		t.Fatalf("consumption advanced model revision %d -> %d", before, after)
	}
}
