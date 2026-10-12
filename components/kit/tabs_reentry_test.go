package kit_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

type cancellingPane struct {
	cancel func()
	leave  func()
}

func (*cancellingPane) Draw(headless.Frame) {}

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
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []headless.Tab{{Title: "a", Of: pane}, {Title: "b"}}})
	headless.NewRoot(tabs).Draw(grid.NewSurface(8, 4).View())
	if !tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 2)}) {
		t.Fatal("pane declined the first press")
	}
	pane.cancel = func() {
		tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
	}
	if !tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(3, 0)}) || tabs.Controller().Selected() != 0 {
		t.Fatal("superseded press selected a tab after cancellation started newer input")
	}
}

func TestTabsReadStripGeometryAfterCancellationRedraw(t *testing.T) {
	pane := &cancellingPane{}
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []headless.Tab{{Title: "a", Of: pane}, {Title: "b"}}})
	root := headless.NewRoot(tabs)
	view := grid.NewSurface(20, 4).View()
	root.Draw(view)
	tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 2)})
	pane.cancel = func() {
		tabs.Controller().Set(headless.Tab{Title: "long title", Of: pane}, headless.Tab{Title: "b"})
		root.Draw(view)
	}
	if !tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(3, 0)}) || tabs.Controller().Selected() != 0 {
		t.Fatal("strip fallback selected a tab using geometry from before cancellation")
	}
}

func TestTabsCannotFallbackToTheStripAfterReentrantLeave(t *testing.T) {
	pane := &cancellingPane{}
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []headless.Tab{{Title: "a", Of: pane}, {Title: "b"}}})
	headless.NewRoot(tabs).Draw(grid.NewSurface(8, 4).View())
	tabs.Handle(input.Mouse{Action: input.MouseMove, Pos: image.Pt(1, 2)})
	pane.leave = func() {
		tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
	}
	if !tabs.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(3, 0)}) || tabs.Controller().Selected() != 0 {
		t.Fatal("superseded outside press selected a tab after leave started newer input")
	}
}
