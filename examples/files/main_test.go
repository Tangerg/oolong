package main

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

// tree is a fixed one, so the test says what it means rather than whatever the
// working directory happened to hold.
func tree() []headless.Node[entry] {
	return []headless.Node[entry]{
		{
			Item: entry{name: "core", path: "core", dir: true},
			Children: []headless.Node[entry]{
				{Item: entry{name: "grid.go", path: "core/grid.go"}},
			},
		},
		{Item: entry{name: "README.md", path: "README.md"}},
	}
}

func TestTheKeyboardMovesBetweenPanesAndTheBranchesOpen(t *testing.T) {
	host := programtest.New(t, 70, 14)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return headless.NewRoot(newBrowser(runtime, tree()))
			},
		})
	}()

	host.Shows(t, "core")
	host.Shows(t, "README.md")
	// A directory says what it is rather than being read.
	host.Shows(t, "is a directory")

	// Right opens the branch, and what is under it appears.
	host.Press(input.Right)
	host.Shows(t, "grid.go")

	// The keyboard moves to the other pane, where the arrows scroll rather than
	// select: the container decides which pane an event is for, and neither pane
	// knows about the other.
	host.Press(input.Tab)
	host.Press(input.Down)
	host.Shows(t, "tab: other pane")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
