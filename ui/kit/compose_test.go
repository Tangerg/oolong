package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
	"github.com/Tangerg/oolong/ui/kit"
)

func typeInto(scenes *scenetest.Scope, c *kit.Editor, s string) {
	for _, r := range s {
		scenes.Mount(c).Handle(input.Key{Code: input.Character, Rune: r})
	}
}

func TestADefaultEditorTakesText(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	// An editor configured with nothing but its defaults must still answer keys.
	// One that looked finished and answered no keys would be the worse kind of broken.
	c := kit.NewEditor(kit.EditorConfig{})
	typeInto(scenes, c, "hello")
	if got := c.Controller().Text(); got != "hello" {
		t.Fatalf("text = %q, want what was typed", got)
	}
	if c.Controller().Empty() {
		t.Fatal("the editor reports empty after being typed into")
	}
}

func TestEditorResetEmptiesTheField(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	c := kit.NewEditor(kit.EditorConfig{})
	typeInto(scenes, c, "sent")
	c.Controller().Clear()
	if !c.Controller().Empty() {
		t.Fatalf("text = %q, want nothing left after a reset", c.Controller().Text())
	}
}

func TestEditorLeavesEnterToItsInterface(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	// Whether enter sends or breaks the line is the interface's decision. An editor
	// that swallowed it would take that decision away from everything embedding one.
	c := kit.NewEditor(kit.EditorConfig{})
	if scenes.Mount(c).Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the editor consumed enter")
	}
}

func TestEditorDrawsItsPromptAndPlaceholder(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	c := kit.NewEditor(kit.EditorConfig{Prompt: "› "})
	c.Controller().SetPlaceholder("ask")
	rows := paintWidget(scenes, 12, 1, c)
	if !strings.HasPrefix(rows[0], "› ") {
		t.Fatalf("row = %q, want the prompt marker first", rows[0])
	}
	if !strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder beside it", rows[0])
	}
}

func TestEditorPlaceholderGivesWayToText(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	c := kit.NewEditor(kit.EditorConfig{Prompt: "› "})
	c.Controller().SetPlaceholder("ask")
	typeInto(scenes, c, "hi")
	rows := paintWidget(scenes, 12, 1, c)
	if strings.Contains(rows[0], "ask") {
		t.Fatalf("row = %q, want the placeholder gone once there is text", rows[0])
	}
	if !strings.Contains(rows[0], "hi") {
		t.Fatalf("row = %q, want what was typed", rows[0])
	}
}

func TestEditorMeasuresTheFieldAndItsHints(t *testing.T) {
	c := kit.NewEditor(kit.EditorConfig{Prompt: "› "})
	if got := c.HeightForWidth(20); got != 1 {
		t.Fatalf("an empty editor with no hints = %d rows, want 1", got)
	}
	c.Controller().SetKeys(sendKeys())
	c.SetHints([]keymap.Action{"send"})
	if got := c.HeightForWidth(20); got != 2 {
		t.Fatalf("with a hint row = %d rows, want the field and the hints", got)
	}
}

func TestEditorHintsNobodyCanPressTakeNoRow(t *testing.T) {
	// A hint row with nothing in it is a blank line the user cannot account for.
	c := kit.NewEditor(kit.EditorConfig{Hints: []keymap.Action{"unbound"}})
	c.Controller().SetKeys(sendKeys())
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

func TestEditorGrowsWithItsTextUpToItsCap(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	c := kit.NewEditor(kit.EditorConfig{Behavior: base.EditorConfig{MaxRows: 2}})
	typeInto(scenes, c, "one two three four five six seven eight")
	if got := c.HeightForWidth(10); got != 2 {
		t.Fatalf("= %d rows, want it capped at 2", got)
	}
}

func TestEditorOwnsTheRowLimit(t *testing.T) {
	c := kit.NewEditor(kit.EditorConfig{Behavior: base.EditorConfig{MaxRows: 2, Initial: "one\ntwo\nthree\nfour\nfive\nsix"}})
	if got := c.Controller().MaxRows(); got != 2 {
		t.Fatalf("editor row limit = %d, want the configured limit", got)
	}
	for _, limit := range []int{2, 4, 0} {
		c.Controller().SetMaxRows(limit)
		want := limit
		if want == 0 {
			want = 6
		}
		if got := c.HeightForWidth(20); got != want {
			t.Fatalf("row limit %d: editor height = %d, want %d", limit, got, want)
		}
	}
}

func TestEditorMaxRowsMeansWhatTheBaseEditorsDoes(t *testing.T) {
	const lines = 12
	c := kit.NewEditor(kit.EditorConfig{})
	c.Controller().SetText(strings.Repeat("line\n", lines))
	if got := c.Controller().MaxRows(); got != 0 {
		t.Fatalf("unset row limit = %d, want the editor's unlimited zero", got)
	}
	if got := c.HeightForWidth(20); got != lines+1 {
		t.Fatalf("unlimited editor height = %d, want %d", got, lines+1)
	}
}

func TestEditorDrawsTheHintsUnderTheField(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	c := kit.NewEditor(kit.EditorConfig{
		Prompt: "› ",
		Hints:  []keymap.Action{"send"},
	})
	c.Controller().SetKeys(sendKeys())
	rows := paintWidget(scenes, 20, 2, c)
	if !strings.Contains(rows[1], "send") {
		t.Fatalf("second row = %q, want the hints under the field", rows[1])
	}
}

func TestEditorUsesOneDefaultMapForEditingAndHints(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	c := kit.NewEditor(kit.EditorConfig{Hints: []keymap.Action{base.Yank}})
	c.Controller().SetText("restored")
	c.Controller().MoveLineStart()
	c.Controller().KillToEnd()

	if !scenes.Mount(c).Handle(input.Key{Code: input.Character, Rune: 'y', Mods: input.Ctrl}) {
		t.Fatal("the default editor binding was not active through Editor.Handle")
	}
	if got := c.Controller().Text(); got != "restored" {
		t.Fatalf("text = %q, want the default yank binding to restore it", got)
	}
	rows := paintWidget(scenes, 24, c.HeightForWidth(24), c)
	if !strings.Contains(rows[len(rows)-1], "y yank") {
		t.Fatalf("hint row = %q, want the binding used by the editor", rows[len(rows)-1])
	}
}

func TestStatusAdvancesOnlyWhenTold(t *testing.T) {
	// It holds no clock, which is what lets a test step it and what keeps an idle
	// interface from waking up to animate.
	s := kit.Status{Glyphs: kit.Glyphs{Spinner: []string{"a", "b"}}, Label: "thinking"}
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
	s := kit.Status{Glyphs: kit.ASCII(), Label: "thinking", Elapsed: "4s"}
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
	s := kit.Status{Glyphs: kit.ASCII(), Label: "thinking", Elapsed: "4s"}
	rows := paint(3, 1, func(v grid.View) { s.Draw(v) })
	if strings.Contains(rows[0], "4s") {
		t.Fatalf("row = %q, want the elapsed time dropped when it does not fit", rows[0])
	}
}

func TestAnEntryMeasuresWhatItThenDraws(t *testing.T) {
	// Printing takes a row count before there is a view, so the two have to agree or
	// the block below lands on top of the entry.
	m := kit.Entry{Label: "build", Body: kit.NewParagraph("one two three four five six seven eight nine", grid.Style{})}
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
	entry := kit.Entry{Body: kit.NewParagraph("12345", grid.Style{})}
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
	m := kit.Entry{Label: "build", Body: kit.NewParagraph("short", grid.Style{})}
	if got := m.HeightForWidth(20); got != 3 {
		t.Fatalf("= %d rows, want the label, the body and a blank row", got)
	}
	m.Trailing = 2
	if got := m.HeightForWidth(20); got != 4 {
		t.Fatalf("= %d rows, want the trailing rows asked for", got)
	}
}

func TestAnEntryWithNoLabelHasNoLabelRow(t *testing.T) {
	m := kit.Entry{Body: kit.NewParagraph("just this", grid.Style{})}
	if got := m.HeightForWidth(20); got != 2 {
		t.Fatalf("= %d rows, want the body and a blank row and no label", got)
	}
}

func TestAnEntryIndentsItsBodyUnderItsLabel(t *testing.T) {
	m := kit.Entry{Label: "build", Body: kit.NewParagraph("said", grid.Style{})}
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
		Theme: kit.Theme{Muted: muted}, Label: "source", LabelStyle: emphasis, Body: kit.NewParagraph("said", grid.Style{}),
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
	m := kit.Entry{Body: kit.NewParagraph("one two three four five six seven eight", grid.Style{})}
	wide, narrow := m.HeightForWidth(40), m.HeightForWidth(12)
	if narrow <= wide {
		t.Fatalf("narrow = %d rows and wide = %d, want the narrow one taller", narrow, wide)
	}
}

func TestAnEntryCanBeCopiedWithoutCopyingItsVisualGutter(t *testing.T) {
	m := kit.Entry{Label: "source", Body: kit.NewParagraph("result", grid.Style{}), Trailing: 1}
	rows := m.Rows(20)
	if len(rows) != 3 || rows[0].Content.String() != "source" ||
		rows[1].Content.String() != "result" || rows[1].Offset != 2 || rows[2].Content.String() != "" {
		t.Fatalf("entry rows = %+v", rows)
	}
}

// An entry labels any passive body the way it labels a paragraph, and copying it
// reads that body's own text.
func TestAnEntryLabelsAnyPassiveBody(t *testing.T) {
	body := kit.Stack{kit.Label{Text: "first"}, kit.Split{Start: text.Of("second", grid.Style{}), End: text.Of("done", grid.Style{})}}
	entry := kit.Entry{Label: "build", Body: body}
	rows := paint(20, entry.HeightForWidth(20), entry.Draw)
	if len(rows) != 4 || !strings.HasPrefix(rows[1], "..first") || !strings.HasSuffix(rows[2], "done") {
		t.Fatalf("entry drew:\n%s", strings.Join(rows, "\n"))
	}
	copied := entry.Rows(20)
	if copied[1].Content.String() != "first" || copied[1].Offset != 2 || !strings.HasPrefix(copied[2].Content.String(), "second") {
		t.Fatalf("entry rows = %+v, want the body's own text under the label", copied)
	}
	if got := (kit.Entry{Label: "build"}).HeightForWidth(20); got != 2 {
		t.Fatalf("an entry without a body took %d rows, want the label and the trailing row", got)
	}
}

func TestAnIndentRuleMarksEveryBodyRowWithoutProjectingText(t *testing.T) {
	body := kit.Stack{kit.Label{Text: "one"}, kit.Label{Text: "two"}}
	style := grid.Style{Attr: grid.Bold}
	indent := kit.Indent{Body: body, Left: 2, Rule: kit.Unicode().Vertical, RuleStyle: style}
	surface := grid.NewSurface(6, 3)
	indent.Draw(surface.View())
	if got := surface.Rows(); got[0] != "│ one" || got[1] != "│ two" || got[2] != "" {
		t.Fatalf("drew %q, want a rule beside the two body rows only", got)
	}
	if cell, _ := surface.CellAt(0, 1); cell.Style != style {
		t.Fatalf("rule style = %+v, want %+v", cell.Style, style)
	}
	rows := indent.Rows(6)
	if len(rows) != 2 || rows[0].Offset != 2 || rows[0].Content.String() != "one" {
		t.Fatalf("projected %+v, want the body alone at offset 2", rows)
	}
}

// A framed block copies as its text: the frame takes rows and columns and says
// nothing, and the body's rows sit where the body is drawn.
func TestAFramedBodyMeasuresDrawsAndCopiesInsideItsFrame(t *testing.T) {
	framed := kit.Framed{Box: kit.Box{Glyphs: kit.ASCII(), Padding: layout.Symmetric(0, 1)}, Body: kit.NewParagraph("one two three", grid.Style{})}
	if got := framed.HeightForWidth(11); got != 4 {
		t.Fatalf("height = %d, want two wrapped rows inside a frame of two", got)
	}
	rows := paint(11, 4, framed.Draw)
	if rows[1] != "|.one two.|" || rows[2] != "|.three...|" {
		t.Fatalf("framed drew:\n%s", strings.Join(rows, "\n"))
	}
	copied := framed.Rows(11)
	if len(copied) != 4 || copied[0].Content.String() != "" || copied[1].Content.String() != "one two" || copied[1].Offset != 2 || copied[3].Content.String() != "" {
		t.Fatalf("framed rows = %+v, want the body's text inside blank frame rows", copied)
	}
}

func TestADisclosureShowsItsDetailsOnlyWhileOpen(t *testing.T) {
	summary := kit.Split{Start: text.Of("read file", grid.Style{}), End: text.Of("done", grid.Style{})}
	disclosure := kit.Disclosure{Glyphs: kit.ASCII(), Summary: summary, Details: kit.Label{Text: "contents"}}
	if got := disclosure.HeightForWidth(20); got != 1 {
		t.Fatalf("closed height = %d, want the summary alone", got)
	}
	if rows := paint(20, 1, disclosure.Draw); rows[0] != "+.read file.....done" {
		t.Fatalf("closed row = %q, want the closed marker before the summary", rows[0])
	}
	disclosure.Open = true
	rows := paint(20, 2, disclosure.Draw)
	if rows[0] != "-.read file.....done" || !strings.HasPrefix(rows[1], "..contents") {
		t.Fatalf("open disclosure drew:\n%s", strings.Join(rows, "\n"))
	}
	copied := disclosure.Rows(20)
	if len(copied) != 2 || !strings.HasPrefix(copied[0].Content.String(), "read file") || copied[0].Offset != 2 || copied[1].Content.String() != "contents" {
		t.Fatalf("disclosure rows = %+v, want the summary and details without the marker", copied)
	}

	leaf := kit.Disclosure{Glyphs: kit.ASCII(), Summary: summary, Open: true}
	if rows := paint(20, 1, leaf.Draw); leaf.HeightForWidth(20) != 1 || !strings.HasPrefix(rows[0], "*.read") {
		t.Fatalf("a disclosure with nothing to open drew %q", rows[0])
	}
}
