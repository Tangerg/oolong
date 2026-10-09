package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type routedPointerControl struct {
	pointer headless.Pointer
	onLeave func()
}

type valuePointerControl struct {
	*routedPointerControl
	payload []byte
}

func TestRootRoutingDoesNotRequireComparableWidgets(t *testing.T) {
	control := &routedPointerControl{}
	root := headless.NewRoot(valuePointerControl{routedPointerControl: control, payload: []byte("value")})
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	for _, action := range []input.MouseAction{input.MouseMove, input.MouseDown} {
		if !root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: action, Button: input.ButtonLeft}) {
			t.Fatal("root declined a valid non-comparable widget")
		}
	}
	root.Draw(view)
	if !root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseUp, Button: input.ButtonLeft}) || !control.pointer.Clicked(input.ButtonLeft) {
		t.Fatal("redrawing a value widget lost its accepted capture")
	}
}

func TestLeaveCallbackCannotDeliverToARemovedTarget(t *testing.T) {
	for _, kind := range []string{"container", "stack", "stack base", "region", "root"} {
		t.Run(kind, func(t *testing.T) {
			a, b := &routedPointerControl{}, &routedPointerControl{}
			view := grid.NewSurface(8, 4).View()
			var root *headless.Root
			var prepare func()
			next := image.Pt(1, 1)
			switch kind {
			case "container":
				container := headless.NewContainer(layout.Across,
					headless.Item{Of: a, Size: layout.Fixed(4)}, headless.Item{Of: b, Size: layout.Fixed(4)})
				root = headless.NewRoot(container)
				prepare = func() {}
				a.onLeave = func() { container.Set(headless.Item{Of: a, Size: layout.Fixed(4)}) }
				next = image.Pt(5, 1)
			case "stack":
				stack := headless.NewStack(nil)
				stack.Push(a)
				root = headless.NewRoot(stack)
				prepare = func() {
					id := stack.Push(b)
					a.onLeave = func() { stack.Remove(id) }
					root.Draw(view)
				}
			case "stack base":
				stack := headless.NewStack(a)
				root = headless.NewRoot(stack)
				prepare = func() { stack.SetBase(b); root.Draw(view) }
				a.onLeave = func() { stack.SetBase(a); root.Draw(view) }
			case "region":
				region := &routedPointerRegion{child: a}
				root = headless.NewRoot(region)
				prepare = func() { region.child = b; root.Draw(view) }
				a.onLeave = func() { region.child = nil; root.Draw(view) }
			case "root":
				root = headless.NewRoot(a)
				prepare = func() { root.SetContent(b); root.Draw(view) }
				a.onLeave = func() { root.SetContent(a); root.Draw(view) }
			}
			root.Draw(view)
			root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseMove})
			prepare()
			root.Handle(input.Mouse{Pos: next, Action: input.MouseDown, Button: input.ButtonLeft})
			if b.pointer.Pressing() || b.pointer.Over() {
				t.Fatal("leave callback removed the new target but the old route still activated it")
			}
		})
	}
}

func TestRoutesInvalidateOldHoverWithoutEndingCapture(t *testing.T) {
	for _, kind := range []string{"container", "stack", "region", "root"} {
		t.Run(kind, func(t *testing.T) {
			a, b := &routedPointerControl{}, &routedPointerControl{}
			var tree headless.Widget
			switch kind {
			case "container":
				tree = headless.NewContainer(layout.Across,
					headless.Item{Of: a, Size: layout.Fixed(4)}, headless.Item{Of: b, Size: layout.Fixed(4)})
			case "stack":
				stack := headless.NewStack(b)
				stack.Push(a)
				tree = stack
			case "region":
				tree = &routedPointerRegion{child: a}
			case "root":
				tree = a
			}
			root := headless.NewRoot(tree)
			view := grid.NewSurface(8, 4).View()
			root.Draw(view)
			root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseMove})
			if !a.pointer.Over() {
				t.Fatal("first hover missing")
			}
			if kind == "root" {
				root.SetContent(b)
				root.Draw(view)
			}
			root.Handle(input.Mouse{Pos: image.Pt(5, 1), Action: input.MouseMove})
			if a.pointer.Over() {
				t.Fatal("old hover survived route transition")
			}
			if kind != "region" && !b.pointer.Over() {
				t.Fatal("new hover missing")
			}
		})
	}
	a := &routedPointerControl{}
	root := headless.NewRoot(&routedPointerRegion{child: a})
	root.Draw(grid.NewSurface(8, 4).View())
	root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseDown, Button: input.ButtonLeft})
	root.Handle(input.Mouse{Pos: image.Pt(5, 1), Action: input.MouseMove})
	if a.pointer.Over() || !a.pointer.Pressing() {
		t.Fatal("leave lost capture or retained hover")
	}
	root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseUp, Button: input.ButtonLeft})
	if !a.pointer.Clicked(input.ButtonLeft) {
		t.Fatal("leave canceled a legal gesture")
	}
}

func (c *routedPointerControl) Draw(frame headless.Frame) {
	w, h := frame.Size()
	c.pointer.Stage(frame, grid.Area(0, 0, w, h))
}

func (c *routedPointerControl) Handle(event input.Event) bool {
	handled := c.pointer.Handle(event)
	if mouse, ok := event.(input.Mouse); ok && mouse.Action == input.MouseLeave && c.onLeave != nil {
		notify := c.onLeave
		c.onLeave = nil
		notify()
	}
	return handled
}

func (*routedPointerControl) Place(image.Point) layout.Placement {
	return layout.Placement{Anchor: layout.TopLeft, Width: 4, Height: 2}
}

type routedPointerRegion struct {
	region headless.PointerRegion
	child  *routedPointerControl
}

func (r *routedPointerRegion) Draw(frame headless.Frame) {
	area := grid.Area(0, 0, 4, 2)
	if r.child != nil {
		r.child.Draw(frame.Sub(area))
	}
	r.region.Stage(frame, area, r.child)
}

func (r *routedPointerRegion) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	handled, _ := r.region.Handle(mouse)
	return handled
}

func TestPointerRoutingKeepsCaptureUntilItsButtonsRelease(t *testing.T) {
	for _, router := range []struct {
		name  string
		build func(*routedPointerControl) headless.Widget
	}{
		{
			name: "region",
			build: func(control *routedPointerControl) headless.Widget {
				return &routedPointerRegion{child: control}
			},
		},
		{
			name: "container",
			build: func(control *routedPointerControl) headless.Widget {
				return headless.NewContainer(layout.Down, headless.Item{Size: layout.Fixed(2), Of: control})
			},
		},
		{
			name: "stack layer",
			build: func(control *routedPointerControl) headless.Widget {
				stack := headless.NewStack(nil)
				stack.Push(control)
				return stack
			},
		},
		{
			name: "stack base",
			build: func(control *routedPointerControl) headless.Widget {
				return headless.NewStack(control)
			},
		},
	} {
		t.Run(router.name, func(t *testing.T) {
			for _, gesture := range []struct {
				name          string
				presses       []input.Button
				wrong         input.Button
				wrongPosition image.Point
				release       input.Button
			}{
				{"different release inside", []input.Button{input.ButtonLeft}, input.ButtonRight, image.Pt(1, 1), input.ButtonLeft},
				{"superseding press", []input.Button{input.ButtonLeft, input.ButtonRight}, input.ButtonLeft, image.Pt(-1, 1), input.ButtonRight},
				{"unspecified release", []input.Button{input.ButtonLeft}, input.ButtonRight, image.Pt(-1, 1), input.ButtonNone},
			} {
				t.Run(gesture.name, func(t *testing.T) {
					control := &routedPointerControl{}
					root := headless.NewRoot(router.build(control))
					root.Draw(grid.NewSurface(8, 4).View())
					for _, pressed := range gesture.presses {
						if !root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseDown, Button: pressed}) {
							t.Fatal("the control declined its press")
						}
					}
					button := gesture.presses[len(gesture.presses)-1]
					root.Handle(input.Mouse{Pos: gesture.wrongPosition, Action: input.MouseUp, Button: gesture.wrong})
					if !control.pointer.Pressing() || control.pointer.Clicked(button) {
						t.Fatal("another button's release completed the control's press")
					}
					outside := image.Pt(-2, 1)
					if !root.Handle(input.Mouse{Pos: outside, Action: input.MouseDrag, Button: button}) {
						t.Fatal("the control lost its captured drag outside the route")
					}
					if at, _ := control.pointer.Position(); at != outside {
						t.Fatalf("the captured drag reached %v, want %v", at, outside)
					}
					if !root.Handle(input.Mouse{Pos: outside, Action: input.MouseUp, Button: gesture.release}) {
						t.Fatal("the control lost its captured release outside the route")
					}
					if control.pointer.Pressing() || control.pointer.Clicked(button) {
						t.Fatal("an outside release left capture or committed a click")
					}
					root.Handle(input.Mouse{Pos: image.Pt(-3, 1), Action: input.MouseDrag, Button: button})
					if at, _ := control.pointer.Position(); at != outside {
						t.Fatal("routing capture outlived the matching release")
					}
				})
			}
		})
	}
}

func TestRootKeepsThePressedTreeAcrossAnotherButtonsRelease(t *testing.T) {
	for _, release := range []struct {
		name   string
		button input.Button
	}{
		{"same button", input.ButtonLeft},
		{"unspecified button", input.ButtonNone},
	} {
		t.Run(release.name, func(t *testing.T) {
			original, replacement := &routedPointerControl{}, &routedPointerControl{}
			root := headless.NewRoot(original)
			surface := grid.NewSurface(8, 4)
			root.Draw(surface.View())
			root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseDown, Button: input.ButtonLeft})
			root.SetContent(replacement)
			root.Draw(surface.View())

			if root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseUp, Button: input.ButtonRight}) {
				t.Fatal("the pressed tree consumed another button's release")
			}
			if !original.pointer.Pressing() || original.pointer.Clicked(input.ButtonLeft) {
				t.Fatal("another button's release completed the original tree's press")
			}
			if !root.Handle(input.Mouse{Pos: image.Pt(-1, 1), Action: input.MouseUp, Button: release.button}) {
				t.Fatal("the original tree lost its captured release")
			}
			if original.pointer.Pressing() {
				t.Fatal("the original tree kept capture after its release")
			}
			if _, inside := replacement.pointer.Position(); inside {
				t.Fatal("the replacement received the original tree's gesture")
			}
			root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseMove})
			if at, inside := replacement.pointer.Position(); !inside || at != image.Pt(1, 1) {
				t.Fatal("routing did not return to the replacement after release")
			}
		})
	}
}

func TestPointerRegionKeepsDeliveryAndConsumptionSeparateAcrossRelease(t *testing.T) {
	original, replacement := &routedPointerControl{}, &routedPointerControl{}
	region := &routedPointerRegion{child: original}
	root := headless.NewRoot(region)
	surface := grid.NewSurface(8, 4)
	root.Draw(surface.View())
	region.region.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseDown, Button: input.ButtonLeft})
	wrongRelease := input.Mouse{Pos: image.Pt(-1, 1), Action: input.MouseUp, Button: input.ButtonRight}
	if handled, delivered := region.region.Handle(wrongRelease); handled || !delivered {
		t.Fatalf("wrong-button release: handled=%v, delivered=%v; want false, true", handled, delivered)
	}

	region.child = replacement
	root.Draw(surface.View())
	for _, event := range []input.Mouse{
		{Pos: image.Pt(1, 1), Action: input.MouseUp, Button: input.ButtonRight},
		{Pos: image.Pt(1, 1), Action: input.MouseDrag, Button: input.ButtonLeft},
		{Pos: image.Pt(1, 1), Action: input.MouseUp, Button: input.ButtonLeft},
	} {
		if handled, delivered := region.region.Handle(event); handled || delivered {
			t.Fatalf("a removed child's gesture reached its replacement: event=%v, handled=%v, delivered=%v", event, handled, delivered)
		}
	}
	if _, inside := replacement.pointer.Position(); inside {
		t.Fatal("the replacement received the removed child's gesture")
	}
	if handled, delivered := region.region.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseMove}); !handled || !delivered {
		t.Fatal("normal routing did not resume after the matching release")
	}
}
