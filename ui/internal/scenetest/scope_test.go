package scenetest_test

import (
	"runtime"
	"testing"
	"weak"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

type drawable [1024]byte

func (*drawable) HeightForWidth(int) int { return 1 }
func (*drawable) Draw(view grid.View)    { view.Text(0, 0, "x", grid.Style{}) }

func TestAdapterIdentityBelongsToOneScope(t *testing.T) {
	first, second := scenetest.New(), scenetest.New()
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)
	widget := &drawable{}
	firstElement := first.Element(widget)
	if first.Element(widget) != firstElement {
		t.Fatal("one scope created two adapters for the same pointer")
	}
	secondElement := second.Element(widget)
	if firstElement == secondElement {
		t.Fatal("independent scopes shared adapter identity")
	}
	first.Mount(widget)
	second.Mount(widget)
	first.Close()
	if firstElement.Alive() || !secondElement.Alive() {
		t.Fatal("closing one scope changed another scope's attachment")
	}
}

func TestMountedChildrenShareFocusAndFrameSequence(t *testing.T) {
	scope := scenetest.New()
	t.Cleanup(scope.Close)
	first := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	queries := 0
	second := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true },
		Semantics: func() scene.Semantic {
			queries++
			return scene.Semantic{}
		},
	})
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{
		{Element: first, Size: layout.Flex(1)},
		{Element: second, Size: layout.Flex(1)},
	}})
	root := scope.Mount(group)
	beforeMount := queries
	if scope.Mount(first) != root || scope.Mount(second) != root {
		t.Fatal("a nested element acquired another scene root")
	}
	if queries != beforeMount {
		t.Fatal("finding a scene owner ran semantic projection callbacks")
	}
	scope.Focus(second, true)
	if !second.Focused() || first.Focused() {
		t.Fatal("focus did not move through the owning scene")
	}
	const frames = 3
	for range frames {
		scope.Mount(second).Draw(grid.NewSurface(2, 1).View())
	}
	if progress := root.Progress(); progress.WrittenSequence != frames || progress.WrittenFrame != progress.LayoutRevision {
		t.Fatalf("fixture did not settle a single frame sequence: %+v", progress)
	}
	pending, err := root.Prepare(grid.NewSurface(2, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Abort()
	beforeMount = queries
	if scope.Mount(second) != root || queries != beforeMount {
		t.Fatal("pending preparation changed attachment ownership or ran semantic projection")
	}
}

func TestMountFollowsAnElementMovedToAnotherOwnedRoot(t *testing.T) {
	scope := scenetest.New()
	t.Cleanup(scope.Close)
	element := scene.NewElement(scene.ElementConfig{})
	first := scope.Mount(element)
	second := scope.Mount(scene.NewElement(scene.ElementConfig{}))
	first.SetContent(scene.Element{})
	second.SetContent(element)
	if scope.Mount(element) != second {
		t.Fatal("fixture membership overrode the element's current scene owner")
	}
}

func TestCloseIncludesRootsWhoseContentWasReplaced(t *testing.T) {
	scope := scenetest.New()
	t.Cleanup(scope.Close)
	element := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	first := scope.Mount(element)
	scope.Focus(element, false)
	if element.Alive() {
		t.Fatal("focus removal left the old content attached")
	}
	second := scope.Mount(element)
	if first == second {
		t.Fatal("detached content reused a different live root's lifetime")
	}
	firstChanges, secondChanges := first.Changes(), second.Changes()
	scope.Close()
	for _, changes := range []<-chan struct{}{firstChanges, secondChanges} {
		select {
		case <-changes:
		default:
			t.Fatal("scope abandoned a replaced root")
		}
	}
	if element.Alive() || first.Root != nil || second.Root != nil {
		t.Fatal("closed scope retained a live root")
	}
	scope.Close()
}

func TestCloseFinishesOtherRootsWhenCleanupPanics(t *testing.T) {
	scope := scenetest.New()
	completed := 0
	first := scene.NewElement(scene.ElementConfig{Detached: func() { completed++ }})
	second := scene.NewElement(scene.ElementConfig{Detached: func() { panic("cleanup failed") }})
	firstRoot, secondRoot := scope.Mount(first), scope.Mount(second)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("cleanup failure was hidden")
			}
		}()
		scope.Close()
	}()
	if first.Alive() || second.Alive() || completed != 1 || firstRoot.Root != nil || secondRoot.Root != nil {
		t.Fatal("cleanup panic left another root or retained reference alive")
	}
	scope.Close()
}

type completedSample struct {
	scope  *scenetest.Scope
	scene  *scenetest.Scene
	root   weak.Pointer[scene.Root]
	widget weak.Pointer[drawable]
}

func finishSample() completedSample {
	scope := scenetest.New()
	widget := &drawable{}
	fixture := scope.Mount(widget)
	fixture.Draw(grid.NewSurface(1, 1).View())
	completed := completedSample{scope: scope, scene: fixture, root: weak.Make(fixture.Root), widget: weak.Make(widget)}
	scope.Close()
	return completed
}

func TestCompletedSamplesReleaseAdaptersAndRoots(t *testing.T) {
	const samples = 256
	completed := make([]completedSample, samples)
	for i := range completed {
		completed[i] = finishSample()
	}
	runtime.GC()
	for i, sample := range completed {
		if sample.scene.Root != nil || sample.root.Value() != nil || sample.widget.Value() != nil {
			t.Fatalf("completed sample %d retained its root or adapter value", i)
		}
		sample.scope.Close()
	}
	runtime.KeepAlive(completed)
}

func TestClosedScopeRejectsNewWork(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*scenetest.Scope)
	}{
		{name: "element", run: func(scope *scenetest.Scope) { scope.Element(&drawable{}) }},
		{name: "mount", run: func(scope *scenetest.Scope) { scope.Mount(&drawable{}) }},
		{name: "focus", run: func(scope *scenetest.Scope) { scope.Focus(&drawable{}, true) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			scope := scenetest.New()
			scope.Close()
			defer func() {
				if recover() == nil {
					t.Error("closed scope accepted new work")
				}
			}()
			operation.run(scope)
		})
	}
}

type control struct {
	focused bool
	events  int
}

func (*control) Draw(frame scene.Frame) { frame.Text(0, 0, "c", grid.Style{}) }
func (c *control) Focus(has bool)       { c.focused = has }
func (c *control) Handle(input.Event) bool {
	c.events++
	return true
}
func (*control) HeightForWidth(width int) int { return width + 1 }

func TestAdapterComposesPaintInputFocusAndMeasurement(t *testing.T) {
	scope := scenetest.New()
	t.Cleanup(scope.Close)
	widget := &control{}
	root := scope.Mount(widget)
	if !widget.focused || scope.Element(widget).Measure(layout.Down, 2) != 3 {
		t.Fatal("adapter lost focus or measurement capability")
	}
	view := grid.NewSurface(1, 1)
	root.Draw(view.View())
	cell, _ := view.CellAt(0, 0)
	if cell.Content() != "c" || !root.Handle(input.Key{Rune: 'x'}) || widget.events != 1 {
		t.Fatal("adapter lost paint or input capability")
	}
	scope.Focus(widget, false)
	if widget.focused {
		t.Fatal("detachment left adapter focused")
	}
}
