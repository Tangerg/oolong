package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/kit"
)

func TestABarSaysHowMuchOfTheWorkIsDone(t *testing.T) {
	p := kit.Progress{Glyphs: kit.ASCII(), Done: 5, Total: 10}
	equalRows(t, paint(10, 1, p.Draw), []string{"#####-----"})

	p.Done = 10
	equalRows(t, paint(10, 1, p.Draw), []string{"##########"})
	if !p.Finished() {
		t.Error("work with nothing left is not finished")
	}

	// Work whose size is nobody knows draws an empty bar rather than a full one:
	// nothing has been done, and a bar that read as complete because there was
	// nothing to do would be the one lie a progress bar must not tell.
	p = kit.Progress{Glyphs: kit.ASCII(), Done: 3}
	equalRows(t, paint(6, 1, p.Draw), []string{"------"})
	if p.Finished() {
		t.Error("work with no total finished")
	}
}

func TestTheCellABarEndsInIsDrawnAsAFractionOfItself(t *testing.T) {
	// Half of eight columns is four cells; half of nine is four and a half, and the
	// half is what keeps a bar from moving in jumps of a whole column.
	p := kit.Progress{Glyphs: kit.Unicode(), Done: 1, Total: 2}
	equalRows(t, paint(9, 1, p.Draw), []string{"████▌░░░░"})

	// A set with no pieces draws the same bar in whole cells, which is what it
	// degrades to rather than a different design.
	p.Glyphs = kit.ASCII()
	equalRows(t, paint(9, 1, p.Draw), []string{"####-----"})
}

func TestTheNumberBesideABarDoesNotMoveTheBar(t *testing.T) {
	// A field wide enough for a hundred of them, right-aligned. A bar that gave the
	// number its own width would shrink by a column between 9% and 10%, and a bar
	// that goes backwards while the work goes forwards is worse than no bar.
	nine := kit.Progress{Glyphs: kit.ASCII(), Done: 9, Total: 100, Percent: true}
	hundred := kit.Progress{Glyphs: kit.ASCII(), Done: 100, Total: 100, Percent: true}
	// Nine per cent of nine columns is less than a whole one, and a set with no
	// pieces of a cell has nothing to draw for it.
	equalRows(t, paint(14, 1, nine.Draw), []string{"---------...9%"})
	equalRows(t, paint(14, 1, hundred.Draw), []string{"#########.100%"})
}

func TestABarWithNoGlyphsDrawsNothing(t *testing.T) {
	// The rule the whole package keeps: a widget given no glyphs draws no furniture,
	// because which characters a terminal can draw is a fact about the terminal and
	// not something to guess at.
	p := kit.Progress{Done: 1, Total: 2}
	equalRows(t, paint(6, 1, p.Draw), []string{"......"})
}

func TestABarSaysWhatTheWorkIs(t *testing.T) {
	p := kit.Progress{Glyphs: kit.ASCII(), Label: "fetching", Done: 1, Total: 4, Percent: true}
	equalRows(t, paint(24, 1, p.Draw), []string{"fetching.##--------..25%"})

	// The label gives way before the bar does, and never takes more than half the
	// room: a bar with no columns left says nothing at all.
	p.Label = "a name much longer than the room there is"
	equalRows(t, paint(20, 1, p.Draw), []string{"a name ....#---..25%"})
}
