package kit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

type said struct{ rows []string }

func (s *said) HeightForWidth(int) int { return len(s.rows) }
func (s *said) Draw(v grid.View) {
	for y, row := range s.rows {
		v.Text(0, y, row, grid.Style{})
	}
}

func (s *said) Rows(int) []text.Row {
	out := make([]text.Row, len(s.rows))
	for i, row := range s.rows {
		out[i] = text.Row{Text: row}
	}
	return out
}

func stageContent(tb testing.TB, content *base.Transcript, width int) {
	tb.Helper()
	element := content.Element()
	if err := element.SetPaint(func(frame scene.Frame) { content.Stage(frame, width) }); err != nil {
		tb.Fatal(err)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		tb.Fatal(err)
	}
	defer root.Close()
	candidate, err := root.Prepare(grid.NewSurface(max(width, 1), 1).View())
	if err != nil {
		tb.Fatal(err)
	}
	candidate.Commit(1)
}

func transcriptSkin(tb testing.TB, cfg kit.TranscriptConfig, blocks ...[]string) (*kit.Transcript, *scene.Root) {
	tb.Helper()
	skin, err := kit.NewTranscript(cfg)
	if err != nil {
		tb.Fatal(err)
	}
	for _, rows := range blocks {
		if _, addErr := skin.Controller().Add(&said{rows: rows}); addErr != nil {
			tb.Fatal(addErr)
		}
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: skin.Element()})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(root.Close)
	return skin, root
}

func transcriptPaint(tb testing.TB, root *scene.Root, width, height int, sequence uint64) *grid.Surface {
	tb.Helper()
	surface := grid.NewSurface(width, height)
	candidate, err := root.Prepare(surface.View())
	if err != nil {
		tb.Fatal(err)
	}
	candidate.Commit(sequence)
	root.Wrote(sequence)
	return surface
}

func TestTranscriptSkinUsesItsControllersSingleElementAndWindow(t *testing.T) {
	skin, root := transcriptSkin(t, kit.TranscriptConfig{}, []string{"one", "two", "three", "four"})
	if skin.Element() != skin.Controller().Element() {
		t.Fatal("skin installed another attachment identity")
	}
	skin.Controller().Scroll().ToTop()
	surface := transcriptPaint(t, root, 20, 2, 1)
	if transcriptRow(surface.View(), 0) != "one" || transcriptRow(surface.View(), 1) != "two" {
		t.Fatal("wrong initial window")
	}
	if !skin.Controller().Handle(input.Mouse{Action: input.WheelDown}) {
		t.Fatal("wheel not consumed")
	}
	surface = transcriptPaint(t, root, 20, 2, 2)
	if transcriptRow(surface.View(), 0) != "two" {
		t.Fatal("scroll owner and skin disagreed")
	}
}

func TestTranscriptSelectionAndMatchesDecorateTheSharedProjection(t *testing.T) {
	theme := kit.Dark()
	skin, root := transcriptSkin(t, kit.TranscriptConfig{Theme: theme}, []string{"cat and cat"})
	transcriptPaint(t, root, 20, 1, 1)
	model := skin.Controller()
	a, _ := model.Layout().Hit(base.Point{Row: 0, Col: 0})
	b, _ := model.Layout().Hit(base.Point{Row: 0, Col: 2})
	model.Selection().Set(base.TextRange{Start: a.Start, End: b.End})
	surface := transcriptPaint(t, root, 20, 1, 2)
	for x := range 3 {
		if transcriptCell(surface.View(), x, 0).Style != theme.Selection {
			t.Fatalf("unmarked selection at %d", x)
		}
	}
	if transcriptCell(surface.View(), 3, 0).Style == theme.Selection {
		t.Fatal("selection extended past logical end")
	}
	if transcriptRow(surface.View(), 0) != "cat and cat" {
		t.Fatal("highlight changed text")
	}
	model.Selection().Clear()
	model.SetMatches([]base.Match{{Row: 0, Spans: []base.Span{{Col: 0, Width: 3}}}, {Row: 0, Spans: []base.Span{{Col: 8, Width: 3}}}})
	if !model.RevealMatch(1) {
		t.Fatal("match was not revealed")
	}
	surface = transcriptPaint(t, root, 20, 1, 3)
	if transcriptCell(surface.View(), 0, 0).Style != theme.Selection || transcriptCell(surface.View(), 8, 0).Style != theme.Accent {
		t.Fatal("current match appearance lost")
	}
}

func TestTranscriptPinnedHeaderSharesContentAndSelectionCoordinates(t *testing.T) {
	theme := kit.Dark()
	skin, root := transcriptSkin(t, kit.TranscriptConfig{Theme: theme}, []string{"question"}, []string{"one", "two", "three", "four", "five"})
	model := skin.Controller()
	model.Sticky().Add(model.FirstBlock())
	model.Sticky().Gap = 1
	model.Scroll().ToTop()
	transcriptPaint(t, root, 20, 4, 1)
	a, _ := model.Layout().Hit(base.Point{Row: 0, Col: 0})
	b, _ := model.Layout().Hit(base.Point{Row: 0, Col: 2})
	model.Selection().Set(base.TextRange{Start: a.Start, End: b.End})
	model.Scroll().By(1)
	surface := transcriptPaint(t, root, 20, 4, 2)
	if transcriptRow(surface.View(), 0) != "question" || transcriptRow(surface.View(), 2) != "one" {
		t.Fatalf("header/body mismatch: %q / %q", transcriptRow(surface.View(), 0), transcriptRow(surface.View(), 2))
	}
	if transcriptCell(surface.View(), 0, 0).Style != theme.Surface.Merge(theme.Selection) {
		t.Fatal("pinned logical selection was not styled")
	}
	model.Scroll().ToBottom()
	surface = transcriptPaint(t, root, 20, 4, 3)
	if transcriptRow(surface.View(), 3) != "five" {
		t.Fatal("header consumed room without keeping end visible")
	}
}

func TestTranscriptRetainsUnsupportedContentAndAcceptedPrefixOnly(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	skin, root := transcriptSkin(t, kit.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
	model := skin.Controller()
	first, err := model.Add(&said{rows: []string{"first"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := model.Add(&said{rows: []string{"second"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.State() != base.ContentRetired || second.State() == base.ContentRetired || model.Len() != 1 {
		t.Fatal("retired beyond accepted prefix")
	}
	publisher.Commit(publisher.Prepare(), 1)
	publisher.Written(1)
	publisher.FlushNotifications()
	if second.State() != base.ContentRetired || model.Len() != 0 {
		t.Fatal("capacity release did not resume retention")
	}
	transcriptPaint(t, root, 20, 2, 1)
	publisher.Close(errors.New("done"))
	third, err := model.Add(&said{rows: []string{"still here"}})
	if err != nil {
		t.Fatal(err)
	}
	if third.State() == base.ContentRetired || !errors.Is(third.Err(), publication.ErrClosed) {
		t.Fatal("closed output lost retained content")
	}
	if surface := transcriptPaint(t, root, 20, 2, 2); !strings.HasPrefix(transcriptRow(surface.View(), 0), "still here") {
		t.Fatal("retained failure disappeared")
	}
}

func transcriptCell(view grid.View, x, y int) grid.Cell {
	cell, ok := view.CellAt(x, y)
	if !ok {
		panic("test read outside grid")
	}
	return cell
}

func transcriptRow(view grid.View, y int) string {
	width, _ := view.Size()
	var out strings.Builder
	for x := range width {
		cell := transcriptCell(view, x, y)
		if cell.Content() == "" {
			out.WriteByte(' ')
		} else {
			out.WriteString(cell.Content())
		}
	}
	return strings.TrimRight(out.String(), " ")
}

func TestTranscriptThemeScopeKeepsBehaviorIdentityAcrossPaletteAndWidthChanges(t *testing.T) {
	owner := kit.NewThemeOwner(kit.Dark())
	override := grid.Style{BG: grid.RGBColor(10, 20, 30)}
	skin, root := transcriptSkin(t, kit.TranscriptConfig{Scope: owner.Scope(), Appearance: kit.ThemeOverride{Selection: &override}}, []string{"context"})
	element := skin.Element()
	transcriptPaint(t, root, 9, 1, 1)
	model := skin.Controller()
	span, ok := model.Layout().Hit(base.Point{Row: 0, Col: 1})
	if !ok {
		t.Fatal("text has no logical selection")
	}
	model.Selection().Set(span)
	owner.Set(kit.Light())
	surface := transcriptPaint(t, root, 12, 1, 2)
	if skin.Element() != element || skin.Controller() != model || !model.Selection().Active() {
		t.Fatal("theme or resize replaced a behavior owner")
	}
	if got := transcriptCell(surface.View(), 1, 0).Style; got != override {
		t.Fatalf("instance override lost: %+v", got)
	}
	if got := transcriptRow(surface.View(), 0); got != "context" {
		t.Fatalf("theme changed content: %q", got)
	}
}
