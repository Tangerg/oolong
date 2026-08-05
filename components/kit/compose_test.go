package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

func typeInto(c *kit.Composer, s string) {
	for _, r := range s {
		c.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func TestAZeroComposerTakesText(t *testing.T) {
	// Every widget here is a struct a caller fills in, so the zero value has to work.
	// One that looked finished and answered no keys would be the worse kind of broken.
	var c kit.Composer
	typeInto(&c, "hello")
	if got := c.Text(); got != "hello" {
		t.Fatalf("text = %q, want what was typed", got)
	}
	if c.Empty() {
		t.Fatal("the composer reports empty after being typed into")
	}
}

func TestComposerResetEmptiesTheField(t *testing.T) {
	var c kit.Composer
	typeInto(&c, "sent")
	c.Reset()
	if !c.Empty() {
		t.Fatalf("text = %q, want nothing left after a reset", c.Text())
	}
}

func TestComposerLeavesEnterToItsInterface(t *testing.T) {
	// Whether enter sends or breaks the line is the interface's decision. A composer
	// that swallowed it would take that decision away from everything embedding one.
	var c kit.Composer
	if c.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the composer consumed enter")
	}
}

func TestComposerDrawsItsPromptAndPlaceholder(t *testing.T) {
	c := kit.Composer{Prompt: "› ", Placeholder: "ask"}
	rows := paint(12, 1, func(v grid.View) { c.Draw(v) })
	if !strings.HasPrefix(rows[0], "› ") {
		t.Fatalf("row = %q, want the prompt marker first", rows[0])
	}
	if !strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder beside it", rows[0])
	}
}

func TestComposerPlaceholderGivesWayToText(t *testing.T) {
	c := kit.Composer{Prompt: "› ", Placeholder: "ask"}
	typeInto(&c, "hi")
	rows := paint(12, 1, func(v grid.View) { c.Draw(v) })
	if strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder gone once there is text", rows[0])
	}
	if !strings.Contains(rows[0], "hi") {
		t.Fatalf("row = %q, want what was typed", rows[0])
	}
}

func TestComposerMeasuresTheFieldAndItsHints(t *testing.T) {
	c := kit.Composer{Prompt: "› "}
	if got := c.Measure(20); got != 1 {
		t.Fatalf("an empty composer with no hints = %d rows, want 1", got)
	}
	c.Hints = []headless.Binding{{Key: input.Key{Code: input.Enter}, Does: "send"}}
	if got := c.Measure(20); got != 2 {
		t.Fatalf("with a hint row = %d rows, want the field and the hints", got)
	}
}

func TestComposerHintsThatAreAllHiddenTakeNoRow(t *testing.T) {
	// A hint row with nothing in it is a blank line the user cannot account for.
	c := kit.Composer{Hints: []headless.Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send", Hidden: true},
	}}
	if got := c.Measure(20); got != 1 {
		t.Fatalf("= %d rows, want no room given to hints nobody is shown", got)
	}
}

func TestComposerGrowsWithItsTextUpToItsCap(t *testing.T) {
	c := kit.Composer{MaxRows: 2}
	typeInto(&c, "one two three four five six seven eight")
	if got := c.Measure(10); got != 2 {
		t.Fatalf("= %d rows, want it capped at 2", got)
	}
}

func TestComposerDrawsTheHintsUnderTheField(t *testing.T) {
	c := kit.Composer{
		Prompt: "› ",
		Hints:  []headless.Binding{{Key: input.Key{Code: input.Enter}, Does: "send"}},
	}
	rows := paint(20, 2, func(v grid.View) { c.Draw(v) })
	if !strings.Contains(rows[1], "send") {
		t.Fatalf("second row = %q, want the hints under the field", rows[1])
	}
}

func TestStatusAdvancesOnlyWhenTold(t *testing.T) {
	// It holds no clock, which is what lets a test step it and what keeps an idle
	// interface from waking up to animate.
	s := kit.Status{Doing: "thinking"}
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
	s := kit.Status{Doing: "thinking", Elapsed: "4s"}
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
	s := kit.Status{Doing: "thinking", Elapsed: "4s"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if strings.Contains(rows[0], "4s") {
		t.Fatalf("row = %q, want the elapsed time dropped when it does not fit", rows[0])
	}
}

func TestAMessageMeasuresWhatItThenDraws(t *testing.T) {
	// Printing takes a row count before there is a view, so the two have to agree or
	// the block below lands on top of the message.
	m := kit.Message{Speaker: "you", Body: "one two three four five six seven eight nine"}
	width := 20
	rows := paint(width, m.Measure(width)+3, func(v grid.View) { m.Draw(v) })

	last := -1
	for i, row := range rows {
		if strings.TrimRight(row, ".") != "" {
			last = i
		}
	}
	if last >= m.Measure(width) {
		t.Fatalf("drew content on row %d, but measured only %d rows", last, m.Measure(width))
	}
}

func TestAMessageLeavesRoomAfterItself(t *testing.T) {
	// Consecutive messages that ran together would read as one.
	m := kit.Message{Speaker: "you", Body: "short"}
	if got := m.Measure(20); got != 3 {
		t.Fatalf("= %d rows, want the speaker, the body and a blank row", got)
	}
	m.Trailing = 2
	if got := m.Measure(20); got != 4 {
		t.Fatalf("= %d rows, want the trailing rows asked for", got)
	}
}

func TestAMessageWithNoSpeakerHasNoLabelRow(t *testing.T) {
	m := kit.Message{Body: "just this"}
	if got := m.Measure(20); got != 2 {
		t.Fatalf("= %d rows, want the body and a blank row and no label", got)
	}
}

func TestAMessageIndentsItsBodyUnderItsSpeaker(t *testing.T) {
	m := kit.Message{Speaker: "you", Body: "said"}
	rows := paint(20, 3, func(v grid.View) { m.Draw(v) })
	if !strings.HasPrefix(rows[0], "you") {
		t.Fatalf("first row = %q, want the speaker", rows[0])
	}
	if !strings.HasPrefix(rows[1], "..said") {
		t.Fatalf("second row = %q, want the body indented under it", rows[1])
	}
}

func TestAMessageWrapsToTheWidthItIsGiven(t *testing.T) {
	m := kit.Message{Body: "one two three four five six seven eight"}
	wide, narrow := m.Measure(40), m.Measure(12)
	if narrow <= wide {
		t.Fatalf("narrow = %d rows and wide = %d, want the narrow one taller", narrow, wide)
	}
}

func TestADialogIsAModalTheStackCanDrive(_ *testing.T) {
	// The contract is the point: the appearance half has to satisfy the interface
	// the behaviour half drives, or neither is any use.
	var _ headless.Modal = (*kit.Dialog)(nil)
	var _ headless.Backdrop = (*kit.Dialog)(nil)
}

func TestADialogFramesItsBodyAndTitlesIt(t *testing.T) {
	d := &kit.Dialog{Title: "Confirm", Body: kit.Label{Text: "really?"}}
	rows := paint(20, 5, func(v grid.View) { d.Draw(v) })
	if !strings.Contains(rows[0], "Confirm") {
		t.Fatalf("top row = %q, want the title in the border", rows[0])
	}
	if !strings.Contains(strings.Join(rows, "\n"), "really?") {
		t.Fatalf("drew\n%s\nwant the body inside the frame", strings.Join(rows, "\n"))
	}
	if !strings.HasPrefix(rows[0], "╭") {
		t.Fatalf("top row = %q, want the rounded border it defaults to", rows[0])
	}
}

func TestADialogPutsItsHintsInTheBottomBorder(t *testing.T) {
	// Where they do not cost a row, which is the whole reason to put them there.
	d := &kit.Dialog{
		Title: "Confirm",
		Body:  kit.Label{Text: "x"},
		Hints: []headless.Binding{{Key: input.Key{Code: input.Enter}, Does: "ok"}},
	}
	rows := paint(24, 4, func(v grid.View) { d.Draw(v) })
	if !strings.Contains(rows[3], "ok") {
		t.Fatalf("bottom row = %q, want the hints in the border", rows[3])
	}
}

func TestADialogBackdropDimsWhatIsBehindWithoutErasingIt(t *testing.T) {
	// What is behind stays legible and simply recedes, which is what says it is
	// still there rather than gone.
	theme := kit.Dark()
	s := grid.NewSurface(10, 2)
	s.View().Text(0, 0, "behind", theme.Text)
	(&kit.Dialog{Theme: theme}).Backdrop(s.View())

	if got := s.CellAt(0, 0).Content; got != "b" {
		t.Fatalf("cell = %q, want what was behind still there", got)
	}
	dimmed := s.CellAt(0, 0).Style.FG.RGB()
	if dimmed == theme.Text.FG.RGB() {
		t.Fatal("what is behind was not dimmed")
	}
	if dimmed == theme.Scrim.Color.RGB() {
		t.Fatal("what is behind was painted over rather than mixed with")
	}
}

// TestADialogDimsWithTheThemeAndNotWithAnOpinionOfItsOwn: how far an interface
// recedes is part of its look, and a light one takes less of it than a dark one. A
// dialog given no look dims nothing, because dimming toward a colour nobody chose
// would be a guess about what is underneath.
func TestADialogDimsWithTheThemeAndNotWithAnOpinionOfItsOwn(t *testing.T) {
	s := grid.NewSurface(10, 2)
	before := kit.Dark().Text
	s.View().Text(0, 0, "behind", before)
	(&kit.Dialog{}).Backdrop(s.View())

	if got := s.CellAt(0, 0).Style; got != before {
		t.Fatalf("a dialog with no theme dimmed what it covers to %+v", got)
	}
}

func TestADialogPassesInputToABodyThatWantsIt(t *testing.T) {
	editor := &headless.Editor{}
	d := &kit.Dialog{Body: editor}
	if !d.Handle(input.Key{Code: input.Character, Rune: 'q'}) {
		t.Fatal("the dialog did not offer the key to its body")
	}
	if got := editor.Text(); got != "q" {
		t.Fatalf("the body has %q, want the key it was given", got)
	}
}

func TestADialogWithABodyThatIgnoresInputConsumesNothing(t *testing.T) {
	// So the stack can decide what an unconsumed key meant — closing, usually.
	d := &kit.Dialog{Body: kit.Label{Text: "just words"}}
	if d.Handle(input.Key{Code: input.Esc}) {
		t.Fatal("a dialog whose body cannot answer input consumed a key anyway")
	}
}

func TestADialogWithNoBodyIsStillDrawable(t *testing.T) {
	d := &kit.Dialog{Title: "Empty"}
	rows := paint(14, 3, func(v grid.View) { d.Draw(v) })
	if !strings.Contains(rows[0], "Empty") {
		t.Fatalf("top row = %q, want the frame drawn anyway", rows[0])
	}
}

func TestADialogWithNoRoomDrawsNothing(_ *testing.T) {
	d := &kit.Dialog{Title: "Squeezed", Body: kit.Label{Text: "x"}}
	d.Draw(grid.NewSurface(0, 0).View())
	d.Backdrop(grid.NewSurface(0, 0).View())
}

func TestADialogGoesWhereItWasPlaced(t *testing.T) {
	where := layout.Placement{Anchor: layout.Middle, Width: 8, Height: 3}
	d := &kit.Dialog{Where: where}
	if got := d.Place(layout.Size{W: 40, H: 20}); got != where {
		t.Fatalf("= %+v, want the placement it was given", got)
	}
}
