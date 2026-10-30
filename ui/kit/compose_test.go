package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/kit"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func typeInto(c *kit.Composer, s string) {
	for _, r := range s {
		newTestRoot(c).Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func TestAZeroComposerTakesText(t *testing.T) {
	// Every widget here is a struct a caller fills in, so the zero value has to work.
	// One that looked finished and answered no keys would be the worse kind of broken.
	var c kit.Composer
	typeInto(&c, "hello")
	if got := c.Controller().Text(); got != "hello" {
		t.Fatalf("text = %q, want what was typed", got)
	}
	if c.Controller().Empty() {
		t.Fatal("the composer reports empty after being typed into")
	}
}

func TestComposerResetEmptiesTheField(t *testing.T) {
	var c kit.Composer
	typeInto(&c, "sent")
	c.Controller().Clear()
	if !c.Controller().Empty() {
		t.Fatalf("text = %q, want nothing left after a reset", c.Controller().Text())
	}
}

func TestComposerLeavesEnterToItsInterface(t *testing.T) {
	// Whether enter sends or breaks the line is the interface's decision. A composer
	// that swallowed it would take that decision away from everything embedding one.
	var c kit.Composer
	if newTestRoot(&c).Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the composer consumed enter")
	}
}

func TestComposerDrawsItsPromptAndPlaceholder(t *testing.T) {
	c := kit.Composer{Prompt: "› "}
	c.Controller().Placeholder = "ask"
	rows := paintWidget(12, 1, &c)
	if !strings.HasPrefix(rows[0], "› ") {
		t.Fatalf("row = %q, want the prompt marker first", rows[0])
	}
	if !strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder beside it", rows[0])
	}
}

func TestComposerPlaceholderGivesWayToText(t *testing.T) {
	c := kit.Composer{Prompt: "› "}
	c.Controller().Placeholder = "ask"
	typeInto(&c, "hi")
	rows := paintWidget(12, 1, &c)
	if strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder gone once there is text", rows[0])
	}
	if !strings.Contains(rows[0], "hi") {
		t.Fatalf("row = %q, want what was typed", rows[0])
	}
}

func TestComposerMeasuresTheFieldAndItsHints(t *testing.T) {
	c := kit.Composer{Prompt: "› "}
	if got := c.HeightForWidth(20); got != 1 {
		t.Fatalf("an empty composer with no hints = %d rows, want 1", got)
	}
	c.Controller().Keys, c.Hints = sendKeys(), []keymap.Action{"send"}
	if got := c.HeightForWidth(20); got != 2 {
		t.Fatalf("with a hint row = %d rows, want the field and the hints", got)
	}
}

func TestComposerHintsNobodyCanPressTakeNoRow(t *testing.T) {
	// A hint row with nothing in it is a blank line the user cannot account for.
	c := kit.Composer{Hints: []keymap.Action{"unbound"}}
	c.Controller().Keys = sendKeys()
	if got := c.HeightForWidth(20); got != 1 {
		t.Fatalf("= %d rows, want no room given to hints nobody is shown", got)
	}
}

// sendKeys is a map with one action in it, which is all a hint row needs.
func sendKeys() *keymap.Map {
	keys := &keymap.Map{}
	keys.Bind("send", input.Chord{Code: input.Enter})
	return keys
}

func TestComposerGrowsWithItsTextUpToItsCap(t *testing.T) {
	c := kit.Composer{MaxRows: 2}
	typeInto(&c, "one two three four five six seven eight")
	if got := c.HeightForWidth(10); got != 2 {
		t.Fatalf("= %d rows, want it capped at 2", got)
	}
}

func TestComposerDrawsTheHintsUnderTheField(t *testing.T) {
	c := kit.Composer{
		Prompt: "› ",
		Hints:  []keymap.Action{"send"},
	}
	c.Controller().Keys = sendKeys()
	rows := paintWidget(20, 2, &c)
	if !strings.Contains(rows[1], "send") {
		t.Fatalf("second row = %q, want the hints under the field", rows[1])
	}
}

func TestComposerUsesOneDefaultMapForEditingAndHints(t *testing.T) {
	c := kit.Composer{Hints: []keymap.Action{base.Yank}}
	c.Controller().SetText("restored")
	c.Controller().MoveLineStart()
	c.Controller().KillToEnd()

	if !newTestRoot(&c).Handle(input.Key{Code: input.Character, Rune: 'y', Mods: input.Ctrl}) {
		t.Fatal("the default editor binding was not active through Composer.Handle")
	}
	if got := c.Controller().Text(); got != "restored" {
		t.Fatalf("text = %q, want the default yank binding to restore it", got)
	}
	rows := paintWidget(24, c.HeightForWidth(24), &c)
	if !strings.Contains(rows[len(rows)-1], "y yank") {
		t.Fatalf("hint row = %q, want the binding used by the editor", rows[len(rows)-1])
	}
}

func TestStatusAdvancesOnlyWhenTold(t *testing.T) {
	// It holds no clock, which is what lets a test step it and what keeps an idle
	// interface from waking up to animate.
	s := kit.Status{Glyphs: kit.Glyphs{Spinner: []string{"a", "b"}}, Doing: "thinking"}
	first := paint(20, 1, func(v grid.View) { s.Draw(v) })
	again := paint(20, 1, func(v grid.View) { s.Draw(v) })
	if first[0] != again[0] {
		t.Fatal("drawing twice advanced the spinner")
	}
	s.Tick()
	after := paint(20, 1, func(v grid.View) { s.Draw(v) })
	if after[0] == first[0] {
		t.Fatal("a tick did not advance the spinner")
	}
}

func TestStatusPinsTheElapsedTimeToTheRight(t *testing.T) {
	s := kit.Status{Glyphs: kit.ASCII(), Doing: "thinking", Elapsed: "4s"}
	rows := paint(20, 1, func(v grid.View) { s.Draw(v) })
	if !strings.HasSuffix(rows[0], "4s") {
		t.Fatalf("row = %q, want the elapsed time at the right edge", rows[0])
	}
	if !strings.Contains(rows[0], "thinking") {
		t.Fatalf("row = %q, want the label too", rows[0])
	}
}

func TestStatusDropsTheElapsedTimeWithNoRoomForIt(t *testing.T) {
	// A number crushed against a truncated label is worse than no number.
	s := kit.Status{Glyphs: kit.ASCII(), Doing: "thinking", Elapsed: "4s"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if strings.Contains(rows[0], "4s") {
		t.Fatalf("row = %q, want the elapsed time dropped when it does not fit", rows[0])
	}
}

func TestAnEntryMeasuresWhatItThenDraws(t *testing.T) {
	// Printing takes a row count before there is a view, so the two have to agree or
	// the block below lands on top of the entry.
	m := kit.Entry{Label: "build", Body: "one two three four five six seven eight nine"}
	width := 20
	rows := paint(width, m.HeightForWidth(width)+3, func(v grid.View) { m.Draw(v) })

	last := -1
	for i, row := range rows {
		if strings.TrimRight(row, ".") != "" {
			last = i
		}
	}
	if last >= m.HeightForWidth(width) {
		t.Fatalf("drew content on row %d, but measured only %d rows", last, m.HeightForWidth(width))
	}
}

func TestAnEntryWithoutALabelUsesTheWholeWidth(t *testing.T) {
	entry := kit.Entry{Body: "12345"}
	if got := entry.HeightForWidth(5); got != 2 { // one body row and the trailing row
		t.Fatalf("height = %d, want the body to fit one row without a label gutter", got)
	}
	rows := paint(5, 2, entry.Draw)
	if rows[0] != "12345" {
		t.Fatalf("body row = %q, want it to begin in the first column", rows[0])
	}
	copyRows := entry.Rows(5)
	if len(copyRows) == 0 || copyRows[0].Offset != 0 {
		t.Fatalf("copy rows = %+v, want no hidden gutter offset", copyRows)
	}
}

func TestAnEntryLeavesRoomAfterItself(t *testing.T) {
	// Consecutive entries that ran together would read as one.
	m := kit.Entry{Label: "build", Body: "short"}
	if got := m.HeightForWidth(20); got != 3 {
		t.Fatalf("= %d rows, want the label, the body and a blank row", got)
	}
	m.Trailing = 2
	if got := m.HeightForWidth(20); got != 4 {
		t.Fatalf("= %d rows, want the trailing rows asked for", got)
	}
}

func TestAnEntryWithNoLabelHasNoLabelRow(t *testing.T) {
	m := kit.Entry{Body: "just this"}
	if got := m.HeightForWidth(20); got != 2 {
		t.Fatalf("= %d rows, want the body and a blank row and no label", got)
	}
}

func TestAnEntryIndentsItsBodyUnderItsLabel(t *testing.T) {
	m := kit.Entry{Label: "build", Body: "said"}
	rows := paint(20, 3, func(v grid.View) { m.Draw(v) })
	if !strings.HasPrefix(rows[0], "build") {
		t.Fatalf("first row = %q, want the label", rows[0])
	}
	if !strings.HasPrefix(rows[1], "..said") {
		t.Fatalf("second row = %q, want the body indented under it", rows[1])
	}
}

func TestAnEntryLeavesItsLabelRoleWithTheCaller(t *testing.T) {
	muted := grid.Style{FG: grid.RGBColor(1, 2, 3)}
	emphasis := grid.Style{FG: grid.RGBColor(4, 5, 6), Attr: grid.Bold}
	e := kit.Entry{
		Theme: kit.Theme{Muted: muted}, Label: "source", LabelStyle: emphasis, Body: "said",
	}
	surface := grid.NewSurface(20, e.HeightForWidth(20))
	e.Draw(surface.View())
	if got := cellAt(surface, 0, 0).Style; got != emphasis {
		t.Fatalf("label style = %+v, want caller-owned role %+v", got, emphasis)
	}

	e.LabelStyle = grid.Style{}
	e.Draw(surface.View())
	if got := cellAt(surface, 0, 0).Style; got != muted {
		t.Fatalf("default label style = %+v, want muted role %+v", got, muted)
	}
}

func TestAnEntryWrapsToTheWidthItIsGiven(t *testing.T) {
	m := kit.Entry{Body: "one two three four five six seven eight"}
	wide, narrow := m.HeightForWidth(40), m.HeightForWidth(12)
	if narrow <= wide {
		t.Fatalf("narrow = %d rows and wide = %d, want the narrow one taller", narrow, wide)
	}
}

func TestAnEntryCanBeCopiedWithoutCopyingItsVisualGutter(t *testing.T) {
	m := kit.Entry{Label: "source", Body: "result", Trailing: 1}
	rows := m.Rows(20)
	if len(rows) != 3 || rows[0].Text != "source" ||
		rows[1].Text != "result" || rows[1].Offset != 2 || rows[2].Text != "" {
		t.Fatalf("entry rows = %+v", rows)
	}
}

func TestEntryMutationInvalidatesItsPrivateWrap(t *testing.T) {
	old := grid.Style{FG: grid.RGBColor(1, 2, 3)}
	newStyle := grid.Style{FG: grid.RGBColor(4, 5, 6)}
	entry := kit.Entry{Body: "old body", Theme: kit.Theme{Text: old}}
	entry.HeightForWidth(20)

	entry.Body = "new body"
	entry.Theme.Text = newStyle
	surface := grid.NewSurface(20, entry.HeightForWidth(20))
	entry.Draw(surface.View())
	if got := rowOf(surface.View(), 0, 20); !strings.HasPrefix(got, "new body") || strings.Contains(got, "old") {
		t.Fatalf("drawn body = %q, want only the replacement", got)
	}
	if got := cellAt(surface, 0, 0).Style; got != newStyle {
		t.Fatalf("drawn style = %+v, want %+v", got, newStyle)
	}
}
