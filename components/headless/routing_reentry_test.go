package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type recursivePointerTarget struct {
	pointer headless.Pointer
	press   func()
	cancel  func()
	leave   func()
	seen    []input.Mouse
}

type recursivePointerRegion struct{ pointerRegionFixture }

type movingPointerTarget struct {
	recursivePointerTarget
	area image.Rectangle
}

func (c *movingPointerTarget) Place(image.Point) layout.Placement {
	anchor := layout.TopLeft
	if c.area.Min.X > 0 {
		anchor = layout.TopRight
	}
	return layout.Placement{Anchor: anchor, Width: c.area.Dx(), Height: c.area.Dy()}
}

func (r *recursivePointerRegion) Draw(frame headless.Frame) {
	if r.child != nil {
		r.child.Draw(frame.Sub(r.area))
	}
	r.pointerRegionFixture.Draw(frame)
}

func (c *recursivePointerTarget) Draw(frame headless.Frame) { c.pointer.Stage(frame, frame.Bounds()) }

func (*recursivePointerTarget) Place(image.Point) layout.Placement {
	return layout.Placement{Anchor: layout.TopLeft, Width: 4, Height: 2}
}

func (c *recursivePointerTarget) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	c.seen = append(c.seen, mouse)
	handled := c.pointer.Handle(mouse)
	if mouse.Action == input.MouseDown && c.press != nil {
		press := c.press
		c.press = nil
		press()
	}
	if mouse.Action == input.MouseCancel && c.cancel != nil {
		cancel := c.cancel
		c.cancel = nil
		cancel()
	}
	if mouse.Action == input.MouseLeave && c.leave != nil {
		leave := c.leave
		c.leave = nil
		leave()
	}
	return handled
}

func TestReleaseDuringPressDoesNotRestoreRoutingCapture(t *testing.T) {
	a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
	root := headless.NewRoot(a)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	a.press = func() {
		root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
		root.SetContent(b)
		root.Draw(view)
	}
	root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	a.seen = nil
	root.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(9, 9)})
	if a.pointer.Pressing() || len(a.seen) != 1 || a.seen[0].Action != input.MouseLeave {
		t.Fatal("completed release was overwritten by the interrupted press")
	}
}

func TestCancellationMayStartANewerRoutingCapture(t *testing.T) {
	a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
	root := headless.NewRoot(a)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	a.cancel = func() {
		root.SetContent(b)
		root.Draw(view)
		root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: image.Pt(1, 1)})
	}
	root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	if a.pointer.Pressing() || !b.pointer.Pressing() {
		t.Fatal("older replacement press overwrote cancellation's gesture")
	}
	a.seen, b.seen = nil, nil
	root.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonRight, Pos: image.Pt(9, 9)})
	root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight, Pos: image.Pt(9, 9)})
	if len(a.seen) != 0 || len(b.seen) != 2 {
		t.Fatalf("cancellation's capture was lost: old=%v new=%v", a.seen, b.seen)
	}
}

func TestCancellationRedrawRoutesTheNewPressThroughTheCommittedFrame(t *testing.T) {
	for _, kind := range []string{"region", "stack"} {
		t.Run(kind, func(t *testing.T) {
			child := &movingPointerTarget{area: image.Rect(0, 0, 4, 2)}
			view := grid.NewSurface(8, 4).View()
			var tree headless.Widget
			var handle func(input.Event) bool
			var move func()
			switch kind {
			case "region":
				region := &recursivePointerRegion{pointerRegionFixture{child: child, area: child.area}}
				tree, handle = region, region.Handle
				move = func() { region.area = image.Rect(4, 0, 8, 2) }
			case "stack":
				stack := headless.NewStack(nil)
				stack.Push(child)
				tree, handle = stack, stack.Handle
				move = func() { child.area = image.Rect(4, 0, 8, 2) }
			}
			root := headless.NewRoot(tree)
			root.Draw(view)
			handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
			child.cancel = func() { move(); root.Draw(view) }
			handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(5, 1)})
			if !child.pointer.Pressing() {
				t.Fatal("cancellation's committed frame did not receive the new press")
			}
			if pos, _ := child.pointer.Position(); pos != image.Pt(1, 1) {
				t.Fatalf("new press used old coordinates: %v", pos)
			}
		})
	}
}

func TestLeaveRedrawCannotDeliverBorrowedPointerGeometry(t *testing.T) {
	for _, kind := range []string{"region", "container", "stack"} {
		t.Run(kind, func(t *testing.T) {
			a := &recursivePointerTarget{}
			b := &movingPointerTarget{area: image.Rect(0, 0, 4, 2)}
			view := grid.NewSurface(8, 4).View()
			var tree headless.Widget
			var handle func(input.Event) bool
			var prepare, move func()
			pos := image.Pt(1, 1)
			switch kind {
			case "region":
				region := &recursivePointerRegion{pointerRegionFixture{child: a, area: b.area}}
				tree, handle = region, region.Handle
				prepare = func() { region.child = b }
				move = func() { region.area = image.Rect(4, 0, 8, 2) }
			case "container":
				container := headless.NewContainer(layout.Across,
					headless.Item{Key: "a", Of: a, Size: layout.Fixed(4)}, headless.Item{Key: "b", Of: b, Size: layout.Fixed(4)})
				tree, handle = container, container.Handle
				prepare = func() {}
				move = func() {
					container.Set(headless.Item{Key: "b", Of: b, Size: layout.Fixed(4)}, headless.Item{Key: "a", Of: a, Size: layout.Fixed(4)})
				}
				pos = image.Pt(5, 1)
			case "stack":
				stack := headless.NewStack(nil)
				stack.Push(a)
				tree, handle = stack, stack.Handle
				prepare = func() { stack.Push(b) }
				move = func() { b.area = image.Rect(4, 0, 8, 2) }
			}
			root := headless.NewRoot(tree)
			root.Draw(view)
			handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 1)})
			prepare()
			root.Draw(view)
			a.leave = func() { move(); root.Draw(view) }
			handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: pos})
			if b.pointer.Pressing() {
				t.Fatal("leave callback's redraw left an old geometry borrow able to start a press")
			}
			for _, event := range b.seen {
				if event.Action == input.MouseDown {
					t.Fatalf("new frame received an old pointer route: %+v", event)
				}
			}
		})
	}
}

func TestNestedPressKeepsItsOwnRoutingCapture(t *testing.T) {
	for _, kind := range []string{"root", "region", "container", "stack", "stack base"} {
		t.Run(kind, func(t *testing.T) {
			a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
			view := grid.NewSurface(8, 4).View()
			var handle func(input.Event) bool
			var replace func()
			pos := image.Pt(1, 1)
			switch kind {
			case "root":
				root := headless.NewRoot(a)
				root.Draw(view)
				handle = root.Handle
				replace = func() { root.SetContent(b); root.Draw(view) }
			case "region":
				region := &recursivePointerRegion{pointerRegionFixture{child: a, area: image.Rect(0, 0, 4, 2)}}
				root := headless.NewRoot(region)
				root.Draw(view)
				handle = region.Handle
				replace = func() { region.child = b; root.Draw(view) }
			case "container":
				container := headless.NewContainer(layout.Across, headless.Item{Of: a, Size: layout.Fixed(4)}, headless.Item{Of: b, Size: layout.Fixed(4)})
				headless.NewRoot(container).Draw(view)
				handle = container.Handle
				pos = image.Pt(5, 1)
				replace = func() {}
			case "stack", "stack base":
				stack := headless.NewStack(nil)
				if kind == "stack" {
					stack.Push(a)
				} else {
					stack.SetBase(a)
				}
				root := headless.NewRoot(stack)
				root.Draw(view)
				handle = stack.Handle
				replace = func() {
					if kind == "stack" {
						stack.Push(b)
					} else {
						stack.SetBase(b)
					}
					root.Draw(view)
				}
			}
			a.press = func() {
				replace()
				handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: pos})
			}
			handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
			if a.pointer.Pressing() || !b.pointer.Pressing() {
				t.Fatal("new press left the older control's gesture active")
			}
			a.seen, b.seen = nil, nil
			handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonRight, Pos: image.Pt(7, 3)})
			handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight, Pos: image.Pt(7, 3)})
			if len(a.seen) != 0 || len(b.seen) != 2 {
				t.Fatalf("older press replaced capture: old=%v new=%v", a.seen, b.seen)
			}
		})
	}
}

func TestRoutersRejectClippedNewPresses(t *testing.T) {
	for _, kind := range []string{"root", "region", "container", "stack base", "stack layer"} {
		t.Run(kind, func(t *testing.T) {
			target := &pointerTarget{takes: true}
			var tree headless.Widget
			var handle func(input.Event) bool
			switch kind {
			case "root":
				tree = target
			case "region":
				region := &pointerRegionFixture{child: target, area: image.Rect(0, 0, 8, 4)}
				tree, handle = region, region.Handle
			case "container":
				container := headless.NewContainer(layout.Down, headless.Item{Of: target, Size: layout.Fixed(4)})
				tree, handle = container, container.Handle
			case "stack base":
				stack := headless.NewStack(target)
				tree, handle = stack, stack.Handle
			case "stack layer":
				stack := headless.NewStack(nil)
				stack.Push(&panel{place: layout.Placement{Anchor: layout.TopLeft, Width: 8, Height: 4}, takesMouse: true})
				tree, handle = stack, stack.Handle
			}
			root := headless.NewRoot(tree)
			root.Draw(grid.NewSurface(3, 1).View().Sub(image.Rect(-2, -1, 6, 3)))
			if handle == nil {
				handle = root.Handle
			}
			if handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)}) {
				t.Fatal("invisible part of the route accepted a new press")
			}
		})
	}
}
