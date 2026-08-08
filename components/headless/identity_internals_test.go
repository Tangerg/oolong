package headless

import (
	"image"
	"math"
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

func TestIdentitySequenceStopsBeforeReusingAValue(t *testing.T) {
	sequence := identitySequence{last: math.MaxUint64 - 1}
	if got, ok := sequence.next(); !ok || got != math.MaxUint64 {
		t.Fatalf("last identity = %d, %t; want %d, true", got, ok, uint64(math.MaxUint64))
	}
	if got, ok := sequence.next(); ok || got != 0 {
		t.Fatalf("exhausted identity = %d, %t; want 0, false", got, ok)
	}
}

func TestIdentityOwnersFailBeforeChangingTheirState(t *testing.T) {
	t.Run("editor", func(t *testing.T) {
		editor := NewEditor()
		editor.SetText("kept")
		editor.elementIDs.last = math.MaxUint64
		got := panicFrom(func() { editor.InsertElement(0, "new") })
		if got != "headless: editor exhausted element identities" {
			t.Fatalf("panic = %v, want the editor exhaustion contract", got)
		}
		if text := editor.Text(); text != "kept" {
			t.Fatalf("text = %q after refused insertion, want kept", text)
		}
	})

	t.Run("stack", func(t *testing.T) {
		stack := &Stack{layerIDs: identitySequence{last: math.MaxUint64}}
		got := panicFrom(func() { stack.Push(identityModal{}) })
		if got != "headless: stack exhausted layer identities" {
			t.Fatalf("panic = %v, want the stack exhaustion contract", got)
		}
		if stack.Top() != nil {
			t.Fatal("a refused layer was installed")
		}
	})

	t.Run("tree", func(t *testing.T) {
		tree := &Tree[string]{nodeIDs: identitySequence{last: math.MaxUint64}}
		got := panicFrom(func() { tree.SetNodes([]Node[string]{{Item: "new"}}) })
		if got != "headless: tree exhausted node identities" {
			t.Fatalf("panic = %v, want the tree exhaustion contract", got)
		}
		if len(tree.Nodes()) != 0 {
			t.Fatal("a refused node was installed")
		}
	})
}

func panicFrom(f func()) (value any) {
	defer func() { value = recover() }()
	f()
	return nil
}

type identityModal struct{}

func (identityModal) Draw(Frame)                         {}
func (identityModal) Handle(input.Event) bool            { return false }
func (identityModal) Place(image.Point) layout.Placement { return layout.Placement{} }
