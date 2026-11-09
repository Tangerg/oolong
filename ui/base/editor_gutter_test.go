package base_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

type numberedGutter struct{}

func (numberedGutter) Width(int) int { return 2 }

func (numberedGutter) Draw(view grid.View, rows []text.Row) {
	for y, row := range rows {
		if !row.Joined {
			view.Text(0, y, strconv.Itoa(row.Line), grid.Style{})
		}
	}
}

type recordingGutter struct{ rows []text.Row }

func (*recordingGutter) Width(int) int { return 1 }

func (g *recordingGutter) Draw(_ grid.View, rows []text.Row) {
	g.rows = append(g.rows[:0], rows...)
}

func TestEditorGutterSharesTheEditorsWrapAndGeometry(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	editor := &base.Editor{}
	editor.SetGutter(numberedGutter{})
	editor.SetText("alpha beta\nx")

	if got := editor.HeightForWidth(7); got != 4 {
		t.Fatalf("HeightForWidth(7) = %d, want 4 rows including the full-line insertion row", got)
	}
	equalRows(t, paintWidget(scenes, 7, 4, editor), []string{
		"1.alpha",
		".. beta",
		".......",
		"2.x....",
	})

	if _, ok := editor.At(0, 0, 7); ok {
		t.Fatal("At accepted a point in the gutter")
	}
	if at, ok := editor.At(2, 0, 7); !ok || at != (base.Caret{}) {
		t.Fatalf("At(2, 0) = %#v, %v; want the start of the text", at, ok)
	}

	editor.SetCursor(0, 0)
	if editor.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("mouse press in the committed gutter was consumed")
	}

	spans := editor.Spans(base.Caret{}, base.Caret{Byte: 2}, 7)
	if len(spans) != 1 || spans[0].Col != 2 {
		t.Fatalf("Spans = %#v, want columns aligned past the gutter", spans)
	}
}

func TestMaskedEditorDoesNotDiscloseItsValueToAGutter(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	gutter := &recordingGutter{}
	editor := &base.Editor{}
	editor.SetGutter(gutter)
	editor.SetMask("•")
	editor.SetText("secret")
	paintWidget(scenes, 10, 1, editor)

	if len(gutter.rows) != 1 || gutter.rows[0].Text != "••••••" {
		t.Fatalf("gutter rows = %#v, want only the displayed mask", gutter.rows)
	}
}

type measuringGutter struct{ check func() }

func (g measuringGutter) Width(int) int            { g.check(); return 1 }
func (measuringGutter) Draw(grid.View, []text.Row) {}

func TestStandaloneEditorMeasurementCannotEditItsDocument(t *testing.T) {
	e := base.NewEditor(base.EditorConfig{Initial: "unchanged"})
	e.SetGutter(measuringGutter{check: func() { e.SetText("replaced") }})
	var failure error
	func() { defer func() { failure, _ = recover().(error) }(); e.HeightForWidth(10) }()
	if !errors.Is(failure, scene.ErrBusy) || e.Text() != "unchanged" {
		t.Fatalf("measurement changed the document: failure=%v text=%q", failure, e.Text())
	}
}

type queryingGutter struct{ editor *base.Editor }

func (queryingGutter) Width(int) int                { return 1 }
func (g queryingGutter) Draw(grid.View, []text.Row) { g.editor.HeightForWidth(20) }

func TestGutterMeasurementCannotReplaceThePaintedLayout(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	e := base.NewEditor(base.EditorConfig{Initial: "abcdef"})
	e.SetGutter(queryingGutter{e})
	e.SetCursor(0, 0)
	equalRows(t, paintWidget(scenes, 4, 2, e), []string{".abc", ".def"})
}

func TestZeroEditorGutterReceivesItsEmptyLogicalLine(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	gutter := new(recordingGutter)
	e := base.NewEditor(base.EditorConfig{Gutter: gutter})
	paintWidget(scenes, 8, 1, e)
	if len(gutter.rows) != 1 || gutter.rows[0].Line != 1 || gutter.rows[0].Text != "" {
		t.Fatalf("empty editor gutter rows=%v", gutter.rows)
	}
}
