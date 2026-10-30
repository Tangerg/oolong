package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestSelectLocalChoicePreservesItsPositionAfterRemoval(t *testing.T) {
	field := &base.Select[string]{Same: base.Equal[string]}
	field.SetOptions(base.Options("a", "b", "c"))
	field.Do(base.SelectLast)
	field.SetOptions(base.Options("a", "b"))
	chosen, ok := field.Chosen()
	if !ok || chosen.Value != "b" {
		t.Fatalf("local choice=%v exists=%t, want the clamped local position", chosen, ok)
	}
}
