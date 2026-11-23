package known_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/internal/known"
)

func TestAnUnknownValueMatchesNothingNotEvenItsZero(t *testing.T) {
	var unknown known.Value[bool]
	if _, ok := unknown.Get(); ok || known.Is(unknown, false) {
		t.Fatal("an unknown value was read as learned")
	}
	learned := known.Of(false)
	if got, ok := learned.Get(); !ok || got || !known.Is(learned, false) || known.Is(learned, true) {
		t.Fatalf("learned false = %v, %v", got, ok)
	}
}
