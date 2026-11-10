package kit_test

import (
	"reflect"
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

func TestTabsDrawTheirMeasuredTitleProjection(t *testing.T) {
	for _, test := range []struct {
		name, title, shown string
		width              int
	}{
		{name: "tab", title: "a\tb", shown: "a       b", width: 9},
		{name: "filtered grapheme", title: "❤\n\ufe0fX", shown: "❤X", width: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			scenes := scenetest.New()
			t.Cleanup(scenes.Close)
			tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Items: []base.Tab{{Title: test.title}}})
			if got := tabs.Element().Measure(layout.Across, 1); got != test.width {
				t.Fatalf("width = %d, want %d", got, test.width)
			}
			for _, width := range []int{test.width, test.width - 1} {
				actual := grid.NewSurface(width+3, 1)
				expected := grid.NewSurface(width+3, 1)
				area := grid.Area(3, 0, width, 1)
				scenes.Mount(tabs).Draw(actual.View().Sub(area))
				expected.View().Sub(area).Text(0, 0, test.shown, grid.Style{})
				if got, want := actual.Rows(), expected.Rows(); !reflect.DeepEqual(got, want) {
					t.Fatalf("width %d: drawn %q, want %q", width, got, want)
				}
			}
		})
	}
}

func TestButtonKeepsItsBracketsOutsideTheLabelProjection(t *testing.T) {
	for _, test := range []struct {
		name, label, ellipsis, shown string
		width                        int
	}{
		{name: "leading combining mark", label: "\u0301X", shown: "[ X ]", width: 5},
		{name: "tab", label: "a\tb", shown: "[ a       b ]", width: 13},
		{name: "joining-only ellipsis", label: "❤xx", ellipsis: "\ufe0f", shown: "[  ]", width: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			scenes := scenetest.New()
			t.Cleanup(scenes.Close)
			button := kit.NewButton(kit.ButtonConfig{Behavior: base.ButtonConfig{Label: test.label}, Ellipsis: test.ellipsis})
			surface := grid.NewSurface(test.width, 1)
			scenes.Mount(button).Draw(surface.View())
			if got := surface.Rows()[0]; got != test.shown {
				t.Fatalf("button = %q, want %q", got, test.shown)
			}
		})
	}
}

func TestComposerPromptSharesTheEditorInset(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	composer := kit.NewComposer(kit.ComposerConfig{Initial: "Z"})
	for _, test := range []struct{ prompt, shown string }{
		{prompt: "a\tb", shown: "a       bZ"},
		{prompt: "❤\n\ufe0fX", shown: "❤XZ"},
		{prompt: "a\x00\u0301X", shown: "aXZ"},
	} {
		composer.SetPrompt(test.prompt)
		surface := grid.NewSurface(15, 1)
		scenes.Mount(composer).Draw(surface.View().Sub(grid.Area(3, 0, 12, 1)))
		if got, want := surface.Rows()[0], "   "+test.shown; got != want {
			t.Fatalf("prompt %q: drawn %q, want %q", test.prompt, got, want)
		}
	}
}

func TestHelpBudgetsItsProjectedLabelsAndSeparator(t *testing.T) {
	for _, test := range []struct {
		name, first, second, separator, shown, firstShown string
		width                                             int
	}{
		{name: "separator tab", first: "alpha", second: "beta", separator: "\t", shown: "a alpha        b beta", firstShown: "a alpha", width: 21},
		{name: "action tab and control", first: "c\td", second: "c\n\u0301d", separator: "|", shown: "a c       d|b cd", firstShown: "a c       d", width: 16},
		{name: "filtered separator", first: "alpha", second: "beta", separator: "❤\n\ufe0fX", shown: "a alpha❤Xb beta", firstShown: "a alpha", width: 15},
		{name: "standalone combining separator", first: "alpha", second: "beta", separator: "\u0301|", shown: "a alpha|b beta", firstShown: "a alpha", width: 14},
	} {
		t.Run(test.name, func(t *testing.T) {
			var bindings keymap.Map
			bindings.Bind(keymap.Action(test.first), input.Chord{Rune: 'a'})
			bindings.Bind(keymap.Action(test.second), input.Chord{Rune: 'b'})
			help := kit.Help{Keys: &bindings, Show: []keymap.Action{keymap.Action(test.first), keymap.Action(test.second)}, Separator: test.separator}
			for _, size := range []struct {
				width int
				shown string
			}{
				{width: test.width, shown: test.shown},
				{width: test.width - 1, shown: test.firstShown},
			} {
				surface := grid.NewSurface(size.width+3, 1)
				help.Draw(surface.View().Sub(grid.Area(3, 0, size.width, 1)))
				if got, want := surface.Rows()[0], "   "+size.shown; got != want {
					t.Fatalf("width %d: hints = %q, want %q", size.width, got, want)
				}
			}
		})
	}
}

func TestStatusRightAlignsTheProjectedElapsedText(t *testing.T) {
	for _, test := range []struct {
		elapsed, shown string
		width          int
	}{
		{elapsed: "a\tb", shown: "a       b", width: 9},
		{elapsed: "❤\n\ufe0fX", shown: "❤X", width: 2},
	} {
		status := kit.Status{Elapsed: test.elapsed}
		surface := grid.NewSurface(23, 1)
		status.Draw(surface.View().Sub(grid.Area(3, 0, 20, 1)))
		if got, want := surface.Rows()[0], strings.Repeat(" ", 23-test.width)+test.shown; got != want {
			t.Fatalf("elapsed %q: drawn %q, want %q", test.elapsed, got, want)
		}
	}
}

func TestEntryLabelDrawingAndSelectionShareOneProjection(t *testing.T) {
	for _, test := range []struct {
		name, label, projected, shown string
		width                         int
	}{
		{name: "tab", label: "a\tb", projected: "a\tb", shown: "a       b", width: 9},
		{name: "tab before combining mark", label: "\t\u0301X", projected: "\t\u0301X", shown: "        X", width: 9},
		{name: "tab before variation selector", label: "\t\ufe0fX", projected: "\t\ufe0fX", shown: "        X", width: 9},
		{name: "tab before joiner", label: "\t\u200dX", projected: "\t\u200dX", shown: "        X", width: 9},
		{name: "tab before spacing modifier", label: "\t🏻X", projected: "\t🏻X", shown: "        🏻X", width: 11},
		{name: "separated variation selector", label: "❤\n\ufe0fX", projected: "❤X", shown: "❤X", width: 2},
		{name: "separated combining mark", label: "a\x00\u0301X", projected: "aX", shown: "aX", width: 2},
		{name: "malformed byte", label: "a\xffX", projected: "aX", shown: "aX", width: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := &kit.Entry{
				Label: test.label, Theme: kit.Theme{Muted: grid.Style{Attr: grid.Italic}},
				LabelStyle: grid.Style{Attr: grid.Bold},
			}
			if got := entry.Rows(test.width)[0].Content.String(); got != test.projected {
				t.Fatalf("label row = %q, want %q", got, test.projected)
			}
			skin, root := transcriptSkin(t, kit.TranscriptConfig{})
			model := skin.Controller()
			if _, err := model.Add(entry); err != nil {
				t.Fatal(err)
			}
			surface := transcriptPaint(t, root, test.width, entry.HeightForWidth(test.width), 1)
			if got := surface.Rows()[0]; got != test.shown {
				t.Fatalf("drawn label = %q, want %q", got, test.shown)
			}
			projected := grid.NewSurface(test.width, 1)
			entry.Rows(test.width)[0].Content.Draw(projected.View(), 0, 0)
			if !reflect.DeepEqual(projected.Row(0), surface.Row(0)) {
				t.Fatal("label projection changed cells or appearance")
			}
			first, firstOK := model.Layout().Hit(base.Point{Row: 0, Col: 0})
			last, lastOK := model.Layout().Hit(base.Point{Row: 0, Col: test.width - 1})
			if !firstOK || !lastOK {
				t.Fatal("displayed label endpoints could not be selected")
			}
			model.Selection().Set(base.TextRange{Start: first.Start, End: last.End})
			sequence := uint64(1)
			for _, width := range []int{test.width, test.width - 1} {
				sequence++
				transcriptPaint(t, root, width, entry.HeightForWidth(width), sequence)
				if got, err := model.Selection().Text(model.Layout()); err != nil || got != test.projected {
					t.Fatalf("selection at width %d = %q, %v; want %q", width, got, err, test.projected)
				}
			}
		})
	}
}

func TestTreeMarksAlignBranchesAndLeavesAfterProjection(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	tree := kit.NewTree(kit.TreeConfig[string]{
		Glyphs: kit.Glyphs{Expanded: "\t+", Collapsed: "❤\n\ufe0fX"},
		Nodes: []base.Node[string]{
			{Item: "root", Children: []base.Node[string]{{Item: "child"}}},
			{Item: "tail"},
		},
		Text: func(value string) string { return value },
	})
	root := scenes.Mount(tree)
	surface := grid.NewSurface(20, 3)
	root.Draw(surface.View())
	if got, want := surface.Rows(), []string{"❤X        root", "          tail", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("closed rows = %q, want %q", got, want)
	}
	if !tree.Controller().Open(0) {
		t.Fatal("root did not open")
	}
	surface.Reset()
	root.Draw(surface.View())
	if got, want := surface.Rows(), []string{"        + root", "            child", "          tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("open rows = %q, want %q", got, want)
	}
	tree.SetGlyphs(kit.Glyphs{Expanded: "\n", Collapsed: "\u0301"})
	surface.Reset()
	root.Draw(surface.View())
	if got, want := surface.Rows(), []string{"root", "  child", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("invisible marks = %q, want %q", got, want)
	}
}

func TestPaletteMarkerUsesTheReservedTextColumns(t *testing.T) {
	palette := kit.Palette{
		Found:  []base.Found{{Command: base.Command{Name: "one"}}, {Command: base.Command{Name: "two"}}},
		Glyphs: kit.Glyphs{Marker: "a\tb"},
	}
	surface := grid.NewSurface(16, 2)
	palette.Draw(surface.View().Sub(grid.Area(3, 0, 13, 2)))
	if got, want := surface.Rows(), []string{"   a       b one", "             two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("palette rows = %q, want %q", got, want)
	}
}

func TestSpinnerFramesUseIndependentTextProjection(t *testing.T) {
	spinner := kit.Spinner{Glyphs: kit.Glyphs{Spinner: []string{"a\tb", "❤\n\ufe0fX"}}, Label: "Z"}
	for _, want := range []string{"a       b Z", "❤X Z"} {
		surface := grid.NewSurface(16, 1)
		spinner.Draw(surface.View().Sub(grid.Area(3, 0, 13, 1)))
		if got := surface.Rows()[0]; got != "   "+want {
			t.Fatalf("spinner = %q, want %q", got, "   "+want)
		}
		spinner.Tick()
	}
}

func TestLabelProjectionCannotBorrowTheCellBeforeItsAlignment(t *testing.T) {
	surface := grid.NewSurface(5, 1)
	surface.View().Text(1, 0, "❤", grid.Style{})
	kit.Label{Text: "\ufe0fX", Align: layout.Center}.Draw(surface.View())
	if got := cellAt(surface, 1, 0); got.Content() != "❤" || got.Width() != 1 {
		t.Fatalf("label changed its neighbor to %+v", got)
	}
	if got := cellAt(surface, 2, 0).Content(); got != "X" {
		t.Fatalf("aligned label = %q, want X at column 2", surface.Rows())
	}
}

func TestBoxLabelProjectionCannotBorrowItsBorder(t *testing.T) {
	surface := grid.NewSurface(8, 3)
	kit.Box{Glyphs: kit.ASCII(), Title: "\u0301X", Footer: "\u0301Y"}.Draw(surface.View())
	for _, row := range []int{0, 2} {
		if got := cellAt(surface, 1, row).Content(); got != "-" {
			t.Fatalf("row %d: label changed its adjacent border to %q", row, got)
		}
	}
}

func TestTruncatedLabelCannotAttachAModifierToATabGap(t *testing.T) {
	for _, value := range []string{"\t\u0301X", "\t\ufe0fX", "\t\u200dX"} {
		surface := grid.NewSurface(text.TabStop, 1)
		kit.Label{Text: value}.Draw(surface.View())
		if got := surface.Rows()[0]; got != "" {
			t.Fatalf("clipped label %q shows %q, want an empty tab gap", value, got)
		}
	}
}

func TestLineNumbersKeepTheSeparatorsOwnTextOrigin(t *testing.T) {
	for _, test := range []struct {
		separator string
		last      int
	}{
		{separator: "\tX", last: 9},
		{separator: "\ufe0f\u20e3X", last: 1},
	} {
		numbers := kit.LineNumbers{Separator: test.separator}
		surface := grid.NewSurface(numbers.Width(1), 1)
		numbers.Draw(surface.View(), []text.Row{{SourceLine: 1}})
		if got := cellAt(surface, 0, 0); got.Content() != "1" || got.Width() != 1 {
			t.Fatalf("separator %q: line number became %+v", test.separator, got)
		}
		if got := cellAt(surface, test.last, 0).Content(); got != "X" {
			t.Fatalf("separator %q: drawn %q, want X at column %d", test.separator, surface.Rows(), test.last)
		}
	}
}
