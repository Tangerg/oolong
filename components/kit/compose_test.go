package kit_test

import (
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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
	c := kit.Composer{Prompt: "› "}
	c.Editor().Placeholder = "ask"
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
	c.Editor().Placeholder = "ask"
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
	if got := c.Measure(20); got != 1 {
		t.Fatalf("an empty composer with no hints = %d rows, want 1", got)
	}
	c.Editor().Keys, c.Hints = sendKeys(), []keymap.Action{"send"}
	if got := c.Measure(20); got != 2 {
		t.Fatalf("with a hint row = %d rows, want the field and the hints", got)
	}
}

func TestComposerHintsNobodyCanPressTakeNoRow(t *testing.T) {
	// A hint row with nothing in it is a blank line the user cannot account for.
	c := kit.Composer{Hints: []keymap.Action{"unbound"}}
	c.Editor().Keys = sendKeys()
	if got := c.Measure(20); got != 1 {
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
	if got := c.Measure(10); got != 2 {
		t.Fatalf("= %d rows, want it capped at 2", got)
	}
}

func TestComposerDrawsTheHintsUnderTheField(t *testing.T) {
	c := kit.Composer{
		Prompt: "› ",
		Hints:  []keymap.Action{"send"},
	}
	c.Editor().Keys = sendKeys()
	rows := paintWidget(20, 2, &c)
	if !strings.Contains(rows[1], "send") {
		t.Fatalf("second row = %q, want the hints under the field", rows[1])
	}
}

func TestComposerUsesOneDefaultMapForEditingAndHints(t *testing.T) {
	c := kit.Composer{Hints: []keymap.Action{headless.Yank}}
	c.Editor().SetText("restored")
	c.Editor().MoveLineStart()
	c.Editor().KillToEnd()

	if !c.Handle(input.Key{Code: input.Character, Rune: 'y', Mods: input.Ctrl}) {
		t.Fatal("the default editor binding was not active through Composer.Handle")
	}
	if got := c.Text(); got != "restored" {
		t.Fatalf("text = %q, want the default yank binding to restore it", got)
	}
	rows := paintWidget(24, c.Measure(24), &c)
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

func TestAMessageWithoutASpeakerUsesTheWholeWidth(t *testing.T) {
	message := kit.Message{Body: "12345"}
	if got := message.Measure(5); got != 2 { // one body row and the trailing row
		t.Fatalf("height = %d, want the body to fit one row without a speaker gutter", got)
	}
	rows := paint(5, 2, message.Draw)
	if rows[0] != "12345" {
		t.Fatalf("body row = %q, want it to begin in the first column", rows[0])
	}
	copyRows := message.Rows(5)
	if len(copyRows) == 0 || copyRows[0].Offset != 0 {
		t.Fatalf("copy rows = %+v, want no hidden gutter offset", copyRows)
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

func TestAMessageCanBeCopiedWithoutCopyingItsVisualGutter(t *testing.T) {
	m := kit.Message{Speaker: "assistant", Body: "answer", Trailing: 1}
	rows := m.Rows(20)
	if len(rows) != 3 || rows[0].Text != "assistant" ||
		rows[1].Text != "answer" || rows[1].Offset != 2 || rows[2].Text != "" {
		t.Fatalf("message rows = %+v", rows)
	}
}

func TestMessageMutationInvalidatesItsPrivateWrap(t *testing.T) {
	old := grid.Style{FG: grid.RGBColor(1, 2, 3)}
	newStyle := grid.Style{FG: grid.RGBColor(4, 5, 6)}
	message := kit.Message{Body: "old body", Theme: kit.Theme{Text: old}}
	message.Measure(20)

	message.Body = "new body"
	message.Theme.Text = newStyle
	surface := grid.NewSurface(20, message.Measure(20))
	message.Draw(surface.View())
	if got := rowOf(surface.View(), 0, 20); !strings.HasPrefix(got, "new body") || strings.Contains(got, "old") {
		t.Fatalf("drawn body = %q, want only the replacement", got)
	}
	if got := cellAt(surface, 0, 0).Style; got != newStyle {
		t.Fatalf("drawn style = %+v, want %+v", got, newStyle)
	}
}

func TestADialogIsAModalTheStackCanDrive(_ *testing.T) {
	// The contract is the point: the appearance half has to satisfy the interface
	// the behaviour half drives, or neither is any use.
	var _ headless.Modal = (*kit.DialogPanel)(nil)
	var _ headless.Backdrop = (*kit.DialogPanel)(nil)
}

func TestNewDialogComposesPolishedAppearanceOverHeadlessOwnership(t *testing.T) {
	stack := &headless.Stack{}
	dialog := kit.NewDialog(
		stack, kit.Dark(), kit.Unicode(), "Confirm",
		headless.Static{Of: kit.Label{Text: "really?"}},
	)
	dialog.Controller().SetDescription("A structural description")
	dialog.Show()

	rows := paintWidget(20, 5, stack)
	if !strings.Contains(rows[0], "Confirm") || !strings.Contains(strings.Join(rows, "\n"), "really?") {
		t.Fatalf("composed dialog drew\n%s", strings.Join(rows, "\n"))
	}
	if dialog.Panel() == nil || dialog.Controller() == nil ||
		!dialog.Semantics().State.Has(headless.StateOpen) {
		t.Fatal("the short composition hid or disconnected its headless parts")
	}
}

func TestDialogPanelTransfersFocusWithItsBody(t *testing.T) {
	first := &panelChild{}
	second := &panelChild{}
	dialog := kit.NewDialog(&headless.Stack{}, kit.Theme{}, kit.Unicode(), "", first)
	dialog.Show()
	if dialog.Panel().Body() != first || !first.focused {
		t.Fatal("the open dialog did not give its body the keyboard")
	}

	dialog.Panel().SetBody(second)
	if first.focused || !second.focused {
		t.Fatalf("focus after replacement: first=%v second=%v", first.focused, second.focused)
	}
	dialog.Dismiss()
	if second.focused {
		t.Fatal("the dialog body kept focus after dismissal")
	}
}

func TestADialogFramesItsBodyAndTitlesIt(t *testing.T) {
	d := kit.NewDialog(
		&headless.Stack{}, kit.Theme{}, kit.Unicode(), "Confirm",
		headless.Static{Of: kit.Label{Text: "really?"}},
	)
	rows := paintWidget(20, 5, d.Panel())
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
	keys := &keymap.Map{}
	keys.Bind("ok", input.Chord{Code: input.Enter})
	d := kit.NewDialog(
		&headless.Stack{}, kit.Theme{}, kit.Unicode(), "Confirm",
		headless.Static{Of: kit.Label{Text: "x"}},
	)
	d.Panel().Keys = keys
	d.Panel().Hints = []keymap.Action{"ok"}
	rows := paintWidget(24, 4, d.Panel())
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
	(&kit.DialogPanel{Theme: theme}).Backdrop(s.View())

	if got := cellAt(s, 0, 0).Content; got != "b" {
		t.Fatalf("cell = %q, want what was behind still there", got)
	}
	dimmed := cellAt(s, 0, 0).Style.FG.RGB()
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
	(&kit.DialogPanel{}).Backdrop(s.View())

	if got := cellAt(s, 0, 0).Style; got != before {
		t.Fatalf("a dialog with no theme dimmed what it covers to %+v", got)
	}
}

func TestADialogPassesInputToABodyThatWantsIt(t *testing.T) {
	editor := &headless.Editor{}
	d := kit.NewDialog(&headless.Stack{}, kit.Theme{}, kit.Glyphs{}, "", editor)
	if !d.Panel().Handle(input.Key{Code: input.Character, Rune: 'q'}) {
		t.Fatal("the dialog did not offer the key to its body")
	}
	if got := editor.Text(); got != "q" {
		t.Fatalf("the body has %q, want the key it was given", got)
	}
}

func TestADialogWithABodyThatIgnoresInputConsumesNothing(t *testing.T) {
	// So the stack can decide what an unconsumed key meant — closing, usually.
	d := kit.NewDialog(
		&headless.Stack{}, kit.Theme{}, kit.Glyphs{}, "",
		headless.Static{Of: kit.Label{Text: "just words"}},
	)
	if d.Panel().Handle(input.Key{Code: input.Esc}) {
		t.Fatal("a dialog whose body cannot answer input consumed a key anyway")
	}
}

func TestADialogWithNoBodyIsStillDrawable(t *testing.T) {
	d := kit.NewDialog(&headless.Stack{}, kit.Theme{}, kit.Unicode(), "Empty", nil)
	rows := paintWidget(14, 3, d.Panel())
	if !strings.Contains(rows[0], "Empty") {
		t.Fatalf("top row = %q, want the frame drawn anyway", rows[0])
	}
}

func TestADialogWithNoRoomDrawsNothing(_ *testing.T) {
	d := kit.NewDialog(
		&headless.Stack{}, kit.Theme{}, kit.Glyphs{}, "Squeezed",
		headless.Static{Of: kit.Label{Text: "x"}},
	)
	headless.NewRoot(d.Panel()).Draw(grid.NewSurface(0, 0).View())
	d.Panel().Backdrop(grid.NewSurface(0, 0).View())
}

func TestADialogGoesWhereItWasPlaced(t *testing.T) {
	where := layout.Placement{Anchor: layout.Middle, Width: 8, Height: 3}
	d := kit.NewDialog(&headless.Stack{}, kit.Theme{}, kit.Glyphs{}, "", nil)
	d.Panel().Where = where
	if got := d.Panel().Place(image.Pt(40, 20)); got != where {
		t.Fatalf("= %+v, want the placement it was given", got)
	}
}
