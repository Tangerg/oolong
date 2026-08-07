package main

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/fake"
)

func TestThePanesAndTheOrderAreBothTheReadersToChoose(t *testing.T) {
	host := fake.New(t, 70, 14)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return headless.NewRoot(newDashboard(runtime))
			},
		})
	}()

	// The strip names both panes and the first one is showing.
	host.Shows(t, "tasks")
	host.Shows(t, "activity")
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

	// Alt+right moves to the other pane, which is about the queue as one number.
	host.Send(input.Key{Code: input.Right, Mods: input.Alt})
	host.Shows(t, "all of it")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
