package main

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

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
	host.Until(t, "the rows to be in the order of the column that was pressed", func() bool {
		host.Repaint()
		frame := host.Frame()
		lint, test := strings.Index(frame, "lint"), strings.Index(frame, "test")
		return lint >= 0 && test >= 0 && lint < test
	})

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
