package scene

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestExhaustedLayoutRevisionDoesNotClaimThePaintPhase(t *testing.T) {
	painted := 0
	element := NewElement(ElementConfig{Key: "content", Paint: func(Frame) { painted++ }})
	root, err := NewRoot(RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		defer func() {
			if failure := recover(); failure != nil {
				t.Errorf("rejected preparation prevented shutdown: %v", failure)
			}
		}()
		root.Close()
	}()
	view := grid.NewSurface(1, 1).View()
	prepared, err := root.Prepare(view)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Commit(0)
	root.layout.revision = math.MaxUint64
	before := root.Progress()
	changed := root.Changes()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("exhausted layout revision was accepted")
			}
		}()
		_, _ = root.Prepare(view)
	}()
	if painted != 1 || root.Progress() != before || root.Changes() != changed {
		t.Fatal("rejected preparation painted or advanced scene progress")
	}
	if observation, err := root.Observe(Query{}); err != nil || len(observation.Nodes) != 1 || observation.Nodes[0].Target != element.Target() {
		t.Fatalf("rejected preparation blocked observation: %+v, %v", observation, err)
	}
}
