package kit

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/headless"
	"github.com/Tangerg/oolong/primitives/grid"
	"github.com/Tangerg/oolong/primitives/input"
)

func typeInto(c *Composer, s string) {
	for _, r := range s {
		c.Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func TestAZeroComposerTakesText(t *testing.T) {
	// Every widget here is a struct a caller fills in, so the zero value has to work.
	// One that looked finished and answered no keys would be the worse kind of broken.
	var c Composer
	typeInto(&c, "hello")
	if got := c.Text(); got != "hello" {
		t.Fatalf("text = %q, want what was typed", got)
	}
	if c.Empty() {
		t.Fatal("the composer reports empty after being typed into")
	}
}

func TestComposerResetEmptiesTheField(t *testing.T) {
	var c Composer
	typeInto(&c, "sent")
	c.Reset()
	if !c.Empty() {
		t.Fatalf("text = %q, want nothing left after a reset", c.Text())
	}
}

func TestComposerLeavesEnterToItsInterface(t *testing.T) {
	// Whether enter sends or breaks the line is the interface's decision. A composer
	// that swallowed it would take that decision away from everything embedding one.
	var c Composer
	if c.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the composer consumed enter")
	}
}

func TestComposerDrawsItsPromptAndPlaceholder(t *testing.T) {
	c := Composer{Prompt: "› ", Placeholder: "ask"}
	rows := paint(12, 1, func(v grid.View) { c.Draw(v) })
	if !strings.HasPrefix(rows[0], "› ") {
		t.Fatalf("row = %q, want the prompt marker first", rows[0])
	}
	if !strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder beside it", rows[0])
	}
}

func TestComposerPlaceholderGivesWayToText(t *testing.T) {
	c := Composer{Prompt: "› ", Placeholder: "ask"}
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
	c := Composer{Prompt: "› "}
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
	c := Composer{Hints: []headless.Binding{
		{Key: input.Key{Code: input.Enter}, Does: "send", Hidden: true},
	}}
	if got := c.Measure(20); got != 1 {
		t.Fatalf("= %d rows, want no room given to hints nobody is shown", got)
	}
}

func TestComposerGrowsWithItsTextUpToItsCap(t *testing.T) {
	c := Composer{MaxRows: 2}
	typeInto(&c, "one two three four five six seven eight")
	if got := c.Measure(10); got != 2 {
		t.Fatalf("= %d rows, want it capped at 2", got)
	}
}

func TestComposerDrawsTheHintsUnderTheField(t *testing.T) {
	c := Composer{
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
	s := Status{Doing: "thinking"}
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
	s := Status{Doing: "thinking", Elapsed: "4s"}
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
	s := Status{Doing: "thinking", Elapsed: "4s"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if strings.Contains(rows[0], "4s") {
		t.Fatalf("row = %q, want the elapsed time dropped when it does not fit", rows[0])
	}
}

func TestAMessageMeasuresWhatItThenDraws(t *testing.T) {
	// Printing takes a row count before there is a view, so the two have to agree or
	// the block below lands on top of the message.
	m := Message{Speaker: "you", Body: "one two three four five six seven eight nine"}
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
	m := Message{Speaker: "you", Body: "short"}
	if got := m.Measure(20); got != 3 {
		t.Fatalf("= %d rows, want the speaker, the body and a blank row", got)
	}
	m.Trailing = 2
	if got := m.Measure(20); got != 4 {
		t.Fatalf("= %d rows, want the trailing rows asked for", got)
	}
}

func TestAMessageWithNoSpeakerHasNoLabelRow(t *testing.T) {
	m := Message{Body: "just this"}
	if got := m.Measure(20); got != 2 {
		t.Fatalf("= %d rows, want the body and a blank row and no label", got)
	}
}

func TestAMessageIndentsItsBodyUnderItsSpeaker(t *testing.T) {
	m := Message{Speaker: "you", Body: "said"}
	rows := paint(20, 3, func(v grid.View) { m.Draw(v) })
	if !strings.HasPrefix(rows[0], "you") {
		t.Fatalf("first row = %q, want the speaker", rows[0])
	}
	if !strings.HasPrefix(rows[1], "..said") {
		t.Fatalf("second row = %q, want the body indented under it", rows[1])
	}
}

func TestAMessageWrapsToTheWidthItIsGiven(t *testing.T) {
	m := Message{Body: "one two three four five six seven eight"}
	wide, narrow := m.Measure(40), m.Measure(12)
	if narrow <= wide {
		t.Fatalf("narrow = %d rows and wide = %d, want the narrow one taller", narrow, wide)
	}
}
