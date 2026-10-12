package headless_test

import (
	"image"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type focusingPointerTarget struct {
	recursivePointerTarget
	focus func(bool)
}

func (c *focusingPointerTarget) Focus(has bool) {
	if c.focus != nil {
		c.focus(has)
	}
}

type callbackPointerTarget struct {
	movingPointerTarget
	handle func(input.Mouse) bool
}

type abortingRouteFrame struct {
	child headless.Widget
	abort bool
}

func (f *abortingRouteFrame) Draw(frame headless.Frame) {
	f.child.Draw(frame)
	if f.abort {
		panic("aborted route frame")
	}
}

func (c *callbackPointerTarget) Handle(event input.Event) bool {
	if mouse, ok := event.(input.Mouse); ok && c.handle != nil {
		c.seen = append(c.seen, mouse)
		return c.handle(mouse)
	}
	return c.recursivePointerTarget.Handle(event)
}

func TestFocusRedrawEndsTheBorrowedPressRoute(t *testing.T) {
	a, b := &focusingPointerTarget{}, &focusingPointerTarget{}
	first := headless.Item{Key: "a", Of: a, Size: layout.Fixed(4)}
	second := headless.Item{Key: "b", Of: b, Size: layout.Fixed(4)}
	container := headless.NewContainer(layout.Across, first, second)
	root := headless.NewRoot(container)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	a.focus = func(has bool) {
		if !has {
			a.focus = nil
			container.Set(second, first)
			root.Draw(view)
		}
	}
	if !container.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(5, 1)}) {
		t.Fatal("focus replacement did not consume the superseded press")
	}
	if b.pointer.Pressing() || len(b.seen) != 0 {
		t.Fatal("focus callback's frame received the previous frame's press")
	}
	if !container.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}) || !b.pointer.Pressing() {
		t.Fatal("the next press did not use the committed placement")
	}
}

func TestSupersededRegionReportsHandledWithoutDelivery(t *testing.T) {
	a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
	region := &recursivePointerRegion{pointerRegionFixture{child: a, area: image.Rect(0, 0, 4, 2)}}
	root := headless.NewRoot(region)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	region.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 1)})
	region.child = b
	root.Draw(view)
	a.leave = func() { root.Draw(view) }
	handled, delivered := region.region.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	if !handled || delivered || len(b.seen) != 0 {
		t.Fatalf("superseded route: handled=%t delivered=%t events=%v", handled, delivered, b.seen)
	}
}

func TestDeclinedPressRedrawDoesNotAcquireCapture(t *testing.T) {
	for _, kind := range []string{"root", "region", "container", "stack"} {
		t.Run(kind, func(t *testing.T) {
			child := &callbackPointerTarget{area: image.Rect(0, 0, 4, 2)}
			var tree headless.Widget = child
			var handle func(input.Event) bool
			switch kind {
			case "region":
				region := &recursivePointerRegion{pointerRegionFixture{child: child, area: child.area}}
				tree, handle = region, region.Handle
			case "container":
				container := headless.NewContainer(layout.Across, headless.Item{Of: child, Size: layout.Fixed(4)})
				tree, handle = container, container.Handle
			case "stack":
				stack := headless.NewStack(nil)
				stack.Push(child)
				tree, handle = stack, stack.Handle
			}
			root := headless.NewRoot(tree)
			view := grid.NewSurface(8, 4).View()
			root.Draw(view)
			if handle == nil {
				handle = root.Handle
			}
			child.handle = func(mouse input.Mouse) bool {
				if mouse.Action == input.MouseDown {
					root.Draw(view)
				}
				return false
			}
			if !handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}) {
				t.Fatal("redraw did not consume the superseded press")
			}
			child.seen = nil
			handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(9, 9)})
			for _, event := range child.seen {
				if event.Action == input.MouseDrag {
					t.Fatal("a declined press acquired capture when its callback redrew")
				}
			}
		})
	}
}

func TestOutsidePressCannotDismissAfterALeaveRedraw(t *testing.T) {
	child := &movingPointerTarget{area: image.Rect(0, 0, 4, 2)}
	stack := headless.NewStack(nil)
	stack.Push(child)
	root := headless.NewRoot(stack)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	stack.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 1)})
	child.leave = func() { child.area = image.Rect(4, 0, 8, 2); root.Draw(view) }
	stack.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(5, 1)})
	if stack.Depth() != 1 {
		t.Fatal("an old outside hit dismissed the newly placed modal")
	}
}

func TestWheelRedrawCannotContinueThroughAnOlderStack(t *testing.T) {
	base := &recursivePointerTarget{}
	child := &callbackPointerTarget{area: image.Rect(4, 0, 8, 2)}
	stack := headless.NewStack(base)
	id := stack.Push(child)
	stack.Push(&movingPointerTarget{area: image.Rect(0, 0, 4, 2)})
	root := headless.NewRoot(stack)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	child.handle = func(input.Mouse) bool { stack.Remove(id); root.Draw(view); return false }
	if !stack.Handle(input.Mouse{Action: input.WheelDown, Pos: image.Pt(5, 1)}) {
		t.Fatal("redraw did not consume the superseded wheel route")
	}
	if len(base.seen) != 0 {
		t.Fatalf("old stack propagated a wheel after redraw: %v", base.seen)
	}
}

func TestAbortedRedrawPreservesTheCurrentPointerRoute(t *testing.T) {
	a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
	region := &recursivePointerRegion{pointerRegionFixture{child: a, area: image.Rect(0, 0, 4, 2)}}
	frame := &abortingRouteFrame{child: region}
	root := headless.NewRoot(frame)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	region.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 1)})
	region.child = b
	root.Draw(view)
	a.leave = func() {
		frame.abort = true
		defer func() {
			if got := recover(); got != "aborted route frame" {
				t.Fatalf("panic = %v", got)
			}
		}()
		root.Draw(view)
	}
	if !region.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}) || !b.pointer.Pressing() {
		t.Fatal("an aborted frame invalidated the committed route")
	}
}

func TestRemovedPointerOwnerCannotResumeAfterItsRelease(t *testing.T) {
	for _, kind := range []string{"region", "container", "stack"} {
		t.Run(kind, func(t *testing.T) {
			child := &recursivePointerTarget{}
			view := grid.NewSurface(8, 4).View()
			var tree headless.Widget
			var handle func(input.Event) bool
			var remove, restore func()
			switch kind {
			case "region":
				region := &recursivePointerRegion{pointerRegionFixture{child: child, area: image.Rect(0, 0, 4, 2)}}
				tree, handle = region, region.Handle
				remove = func() { region.child = nil }
				restore = func() { region.child = child }
			case "container":
				item := headless.Item{Of: child, Size: layout.Fixed(4)}
				container := headless.NewContainer(layout.Across, item)
				tree, handle = container, container.Handle
				remove = func() { container.Set() }
				restore = func() { container.Set(item) }
			case "stack":
				stack := headless.NewStack(nil)
				id := stack.Push(child)
				tree, handle = stack, stack.Handle
				remove = func() { stack.Remove(id) }
				restore = func() { stack.Push(child) }
			}
			root := headless.NewRoot(tree)
			root.Draw(view)
			handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
			remove()
			root.Draw(view)
			handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
			restore()
			root.Draw(view)
			if child.pointer.Pressing() {
				t.Fatal("removed recipient retained the released gesture")
			}
		})
	}
}

func TestCancellationCannotClearReentrantHover(t *testing.T) {
	a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
	region := &recursivePointerRegion{pointerRegionFixture{child: a, area: image.Rect(0, 0, 4, 2)}}
	root := headless.NewRoot(region)
	view := grid.NewSurface(8, 4).View()
	root.Draw(view)
	region.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	region.child = b
	root.Draw(view)
	a.cancel = func() { region.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 1)}) }
	region.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	if !b.pointer.Over() || b.pointer.Pressing() || a.pointer.Pressing() {
		t.Fatal("older cancellation cleared reentrant hover or left its recipient pressed")
	}
}

func TestDeclinedPressCannotRetainCaptureAfterReentrantMovement(t *testing.T) {
	child := &callbackPointerTarget{}
	root := headless.NewRoot(child)
	root.Draw(grid.NewSurface(8, 4).View())
	child.handle = func(mouse input.Mouse) bool {
		if mouse.Action == input.MouseDown {
			root.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 1)})
		}
		return false
	}
	if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}) {
		t.Fatal("reentrant movement did not supersede the old press route")
	}
	child.seen = nil
	root.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(9, 9)})
	for _, event := range child.seen {
		if event.Action == input.MouseDrag {
			t.Fatal("reentrant movement retained a declined press receipt")
		}
	}
}

func TestAbnormalPointerCallbackPreservesTheNewerCapture(t *testing.T) {
	for _, phase := range []string{"press", "cancel"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(phase+"/"+exit, func(t *testing.T) {
				a, b := &recursivePointerTarget{}, &recursivePointerTarget{}
				root := headless.NewRoot(a)
				view := grid.NewSurface(8, 4).View()
				root.Draw(view)
				interrupt := func() {
					root.SetContent(b)
					root.Draw(view)
					root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: image.Pt(1, 1)})
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic("interrupted pointer callback")
				}
				press := func() {
					root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
				}
				if phase == "cancel" {
					press()
					a.cancel = interrupt
				} else {
					a.press = interrupt
				}
				if exit == "Goexit" {
					done := make(chan struct{})
					go func() { defer close(done); press() }()
					<-done
				} else {
					func() {
						defer func() {
							if got := recover(); got != "interrupted pointer callback" {
								t.Fatalf("panic = %v", got)
							}
						}()
						press()
					}()
				}
				a.seen, b.seen = nil, nil
				root.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonRight, Pos: image.Pt(9, 9)})
				root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight, Pos: image.Pt(9, 9)})
				if a.pointer.Pressing() || b.pointer.Pressing() || len(a.seen) != 0 || len(b.seen) != 2 {
					t.Fatalf("abnormal callback replaced the newer gesture: old=%v new=%v", a.seen, b.seen)
				}
			})
		}
	}
}
