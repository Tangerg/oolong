package text_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func TestSpanBoundariesDoNotSplitGraphemes(t *testing.T) {
	source := "👩‍💻"
	for split := 0; split <= len(source); split++ {
		line := text.Line{{Text: source[:split], Style: grid.Style{Attr: grid.Bold}}, {Text: source[split:]}}
		if line.Width() != 2 {
			t.Fatalf("split %d: width %d", split, line.Width())
		}
		wrapped := line.Wrap(2)
		if len(wrapped) != 1 || wrapped[0].Line.String() != source {
			t.Fatalf("split %d: %#v", split, wrapped)
		}
		s := grid.NewSurface(2, 1)
		line.Draw(s.View(), 0, 0)
		if s.Rows()[0] != source {
			t.Fatalf("split %d: %q", split, s.Rows())
		}
	}
}

func TestPartialMarkStartReplacementIncludesReplacement(t *testing.T) {
	marks := text.Edit{Start: 0, End: 3, Text: "xy"}.Shift([]text.Mark{{Start: 2, End: 5}}, 6)
	if len(marks) != 1 || marks[0].Start != 0 {
		t.Fatalf("marks: %#v", marks)
	}
}

func TestCompleteReplacementDestroysMarkIdentity(t *testing.T) {
	for _, atomic := range []bool{false, true} {
		marks := text.Edit{Start: 4, End: 9, Text: "XYZ"}.Shift([]text.Mark{{Start: 4, End: 9, Atomic: atomic}}, 12)
		if len(marks) != 0 {
			t.Fatalf("replaced identity survived: %#v", marks)
		}
	}
}

func TestFilteredBytesCannotReassembleIntoWiderText(t *testing.T) {
	source := "a\xc2\n\xa9bc"
	if got := text.Truncate(source, 2, "…"); text.Width(got) > 2 {
		t.Fatalf("overflow: %q", got)
	}
	for _, row := range text.Of(source, grid.Style{}).Wrap(2) {
		if row.Line.Width() > 2 {
			t.Fatalf("overflow: %q", row.Line.String())
		}
	}
}

func TestDroppedControlsCannotAttachStandaloneVariationSelectors(t *testing.T) {
	line := text.Of("❤\n\ufe0f", grid.Style{})
	for _, row := range line.Wrap(1) {
		if row.Line.Width() > 1 {
			t.Fatalf("reassembled cluster exceeds budget: %q", row.Line.String())
		}
	}
}
