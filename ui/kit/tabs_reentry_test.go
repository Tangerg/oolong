package kit_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/kit"
)

type cancellingPane struct {
	cancel func()
	leave  func()
}

func (*cancellingPane) Draw(scene.Frame) {}

func (p *cancellingPane) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	if mouse.Action == input.MouseCancel && p.cancel != nil {
		cancel := p.cancel
		p.cancel = nil
		cancel()
	}
	if mouse.Action == input.MouseLeave && p.leave != nil {
		leave := p.leave
		p.leave = nil
		leave()
	}
	return mouse.Action == input.MouseDown
}

func TestTabsCannotFallbackToTheStripAfterReentrantCancellation(t *testing.T) {
	pane := &cancellingPane{}
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []base.Tab{{Title: "a", Content: testElement(pane)}, {Title: "b"}}})
	newTestRoot(tabs).Draw(grid.NewSurface(8, 4).View())
	if !newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 2)}) {
		t.Fatal("pane declined the first press")
	}
	pane.cancel = func() {
		newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
	}
	if !newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(3, 0)}) || tabs.Controller().Selected() != 0 {
		t.Fatal("superseded press selected a tab after cancellation started newer input")
	}
}

func TestTabsRetainTheArrivalFrameDuringCancellationRedraw(t *testing.T) {
	pane := &cancellingPane{}
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []base.Tab{{Title: "a", Content: testElement(pane)}, {Title: "b"}}})
	root := newTestRoot(tabs)
	view := grid.NewSurface(20, 4).View()
	root.Draw(view)
	newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 2)})
	pane.cancel = func() {
		tabs.Controller().Set(base.Tab{Title: "long title", Content: testElement(pane)}, base.Tab{Title: "b"})
		root.Draw(view)
	}
	if !newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(3, 0)}) || tabs.Controller().Selected() != 1 {
		t.Fatal("input replaced its arrival-frame target with cancellation redraw geometry")
	}
}

func TestTabsCannotFallbackToTheStripAfterReentrantLeave(t *testing.T) {
	pane := &cancellingPane{}
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []base.Tab{{Title: "a", Content: testElement(pane)}, {Title: "b"}}})
	newTestRoot(tabs).Draw(grid.NewSurface(8, 4).View())
	newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 2)})
	pane.leave = func() {
		newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
	}
	if !newTestRoot(tabs).Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(3, 0)}) || tabs.Controller().Selected() != 0 {
		t.Fatal("superseded outside press selected a tab after leave started newer input")
	}
}
