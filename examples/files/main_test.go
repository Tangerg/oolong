package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/oolong/ui/kit"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

// tree is a fixed one, so the test says what it means rather than whatever the
// working directory happened to hold.
func tree() []base.Node[entry] {
	return []base.Node[entry]{
		{
			Item: entry{name: "core", path: "core", dir: true},
			Children: []base.Node[entry]{
				{Item: entry{name: "grid.go", path: "core/grid.go"}},
			},
		},
		{Item: entry{name: "README.md", path: "README.md"}},
	}
}

func TestTheKeyboardMovesBetweenPanesAndTheBranchesOpen(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 70, Height: 14})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newBrowser(runtime, tree()).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
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
	host.Shows(t, "tab focus next")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}

func TestDirectoryReadErrorsReachTheRootAndPreview(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	nodes, err := read(missing, 1)
	if len(nodes) != 0 || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing root returned %d selectable entries and error %v", len(nodes), err)
	}
	lines := preview(entry{path: missing, dir: true, scanErr: err}, kit.Theme{})
	if len(lines) != 1 || lines[0].String() != err.Error() {
		t.Fatal("directory preview lost its scan error")
	}
}

func TestPreviewDoesNotFollowASelectedLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	const contents = "only visible when the target is selected"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("cannot create a symbolic link on this host: %v", err)
	}
	lines := preview(entry{path: link}, kit.Theme{})
	if len(lines) != 1 || lines[0].String() == contents {
		t.Fatalf("preview followed the selected link: %v", lines)
	}
}
