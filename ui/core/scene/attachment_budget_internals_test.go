package scene

import (
	"errors"
	"math"
	"testing"
)

func TestSiblingPreflightSharesOneAttachmentIdentityBudget(t *testing.T) {
	left, right := NewGroup(GroupConfig{}), NewGroup(GroupConfig{})
	parent := NewGroup(GroupConfig{Items: []Item{{Element: left.Element()}, {Element: right.Element()}}})
	root, err := NewRoot(RootConfig{Element: parent.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	root.nextID = math.MaxUint64 - 1
	one, two := NewElement(ElementConfig{}), NewElement(ElementConfig{})
	changes := []GroupItems{{Group: left, Items: []Item{{Element: one}}}, {Group: right, Items: []Item{{Element: two}}}}
	if err := parent.SetChildItems(changes...); !errors.Is(err, ErrInvalid) {
		t.Fatalf("budget rejection=%v", err)
	}
	if one.Alive() || two.Alive() || root.nextID != math.MaxUint64-1 {
		t.Fatal("failed preflight consumed the last identity")
	}
	if err := parent.SetChildItems(changes[:1]...); err != nil || !one.Alive() {
		t.Fatalf("last available identity refused: %v", err)
	}
}
