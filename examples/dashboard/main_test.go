package main

import (
	"context"
	"image"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestCompletedDashboardStopsPainting(t *testing.T) {
	for _, motion := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			host := uitest.New(t, uitest.Config{Width: 70, Height: 14})
			var paints atomic.Int64
			ctx, cancel := context.WithCancel(t.Context())
			finished := make(chan error, 1)
			go func() {
				app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) { //nolint:contextcheck // Runtime scopes inherit Run(ctx); the builder has no separate context parameter.
					d := newDashboard(runtime)
					d.motion = motion
					d.work.rows.SetItems([]task{{name: "last task", total: 1}})
					return d.Element(), d.Element().SetPaint(func(scene.Frame) { paints.Add(1) })
				}})
				if err == nil {
					err = app.Run(ctx)
				}
				finished <- err
			}()
			defer func() {
				cancel()
				if err := <-finished; err != nil {
					t.Error(err)
				}
			}()
			host.Shows(t, "waiting")
			host.Shows(t, "done")
			time.Sleep(dashboardTickInterval)
			before := paints.Load()
			time.Sleep(4 * dashboardTickInterval)
			if after := paints.Load(); after != before {
				t.Fatalf("motion=%v: completed dashboard painted %d extra frames", motion, after-before)
			}
		})
	}
}

func TestThePanesAndTheOrderAreBothTheReadersToChoose(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 70, Height: 14})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newDashboard(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	// The strip names every pane and the first one is showing.
	host.Shows(t, "tasks")
	host.Shows(t, "activity")
	host.Shows(t, "settings")
	host.Shows(t, "build")

	// A press on a heading sorts by it. The geometry is the table's, so the column
	// under the pointer is the column that gets the order.
	//
	// What is asserted is what reached the screen, and not the table's own state:
	// one goroutine owns the interface, and a test that reached into it from outside
	// would be breaking the rule the whole library is built on.
	host.Send(input.Mouse{
		Pos:    image.Pt(2, 2), // the strip is two rows, then the header
		Action: input.MouseDown,
		Button: input.ButtonLeft,
	})
	snapshot, err := host.Repaint(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	lint, test := strings.Index(snapshot.Frame, "lint"), strings.Index(snapshot.Frame, "test")
	if lint < 0 || test < 0 || lint >= test {
		t.Fatalf("rows are not in the selected order: %q", snapshot.Frame)
	}

	// A numeric shortcut writes the caller-owned selection and asks the controller
	// to settle focus. The same strip then projects that one source of truth.
	host.Type("2")
	host.Shows(t, "all of it")
	host.Shows(t, "1 tasks/tick")
	host.Send(input.Key{Code: input.Right})
	host.Shows(t, "2 tasks/tick")

	// The settings list changes the same bounded value, rather than holding a second
	// preference that only looks like it. Moving back proves both panes observe one
	// source of truth.
	host.Type("3")
	host.Shows(t, "motion")
	host.Shows(t, "2 tasks/tick")
	host.Send(input.Key{Code: input.Right})
	host.Shows(t, "3 tasks/tick")
	host.Type("2")
	host.Shows(t, "3 tasks/tick")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}

func TestTheSliderAnswersThePressThatLandedOnIt(t *testing.T) {
	// A pointer region is staged in the coordinates of the frame the press will
	// arrive in. Asking the child frame for its own bounds gives a rectangle that
	// begins at zero, so the slider claimed whichever row happened to be first: a
	// press on the slider did nothing and a press on the row above it moved the rate.
	host := uitest.New(t, uitest.Config{Width: 70, Height: 14})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newDashboard(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	host.Type("2")
	host.Shows(t, "1 tasks/tick")

	// The activity pane begins below the two-row strip: the overall progress, then
	// the rate slider.
	const progressRow, sliderRow = 2, 3
	press := func(y int) {
		host.Send(input.Mouse{
			Pos:    image.Pt(55, y),
			Action: input.MouseDown,
			Button: input.ButtonLeft,
		})
	}

	press(progressRow)
	host.Shows(t, "1 tasks/tick")
	press(sliderRow)
	host.Shows(t, "3 tasks/tick")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}

func TestTaskStatusIsDerivedFromCompletedWork(t *testing.T) {
	work := newQueue(kit.Theme{}, kit.Glyphs{})
	state := func(row int) string {
		surface := grid.NewSurface(10, 1)
		work.cell(row, 1).Draw(surface.View(), grid.Style{})
		return strings.TrimSpace(surface.Rows()[0])
	}
	if state(0) != running || state(2) != waiting {
		t.Fatalf("initial build=%q vet=%q", state(0), state(2))
	}
	work.advance(100)
	for row := range work.rows.Len() {
		if got := state(row); got != done {
			t.Errorf("finished row %d: %q", row, got)
		}
	}
}

func TestNumberedPaneShortcutsAndHelpFollowTheTabOwner(t *testing.T) {
	d := &dashboard{tabs: base.NewTabs(base.TabsConfig{Keys: base.DefaultTabsKeys(), Items: []base.Tab{
		{Title: "first"}, {Title: "second"}, {Title: "third"}, {Title: "fourth"},
	}})}
	if !d.Handle(input.Key{Rune: '4'}) || d.tabs.Selected() != 3 {
		t.Fatal("a newly added fourth pane has no numeric shortcut")
	}
	if d.Handle(input.Key{Rune: '2', Mods: input.Ctrl}) || d.tabs.Selected() != 3 {
		t.Fatal("modified digit unexpectedly selected a pane")
	}
	visible := func() string {
		surface := grid.NewSurface(80, 1)
		d.drawHelp(scene.Frame{View: surface.View()})
		return surface.Rows()[0]
	}
	if help := visible(); !strings.Contains(help, "1–4 pane") {
		t.Fatalf("help does not reflect the fourth pane: %q", help)
	}
	d.tabs.Set(base.Tab{Title: "only"})
	if d.Handle(input.Key{Rune: '4'}) {
		t.Fatal("removed pane retained a numeric shortcut")
	}
	if help := visible(); !strings.Contains(help, "1 pane") || strings.Contains(help, "1–4") {
		t.Fatalf("help retained a removed pane: %q", help)
	}
	d.tabs.Keys = new(keymap.Map)
	d.tabs.Keys.Bind(base.NextTab, input.Alt.Rune('n'))
	d.tabs.Keys.Bind(quitDashboard, input.Chord{Rune: 'x'})
	if help := visible(); !strings.Contains(help, "alt+n next tab") || !strings.Contains(help, "x quit") {
		t.Fatalf("help retained a replaced key map: %q", help)
	}
}
