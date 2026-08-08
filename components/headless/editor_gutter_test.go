package headless_test

import (
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/text"
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
	editor := headless.NewEditor()
	editor.Gutter = numberedGutter{}
	editor.SetText("alpha beta\nx")

	if got := editor.Measure(7); got != 3 {
		t.Fatalf("Measure(7) = %d, want 3 wrapped rows", got)
	}
	equalRows(t, paintWidget(7, 3, editor), []string{
		"1.alpha",
		".. beta",
		"2.x....",
	})

	if _, ok := editor.At(0, 0, 7); ok {
		t.Fatal("At accepted a point in the gutter")
	}
	if at, ok := editor.At(2, 0, 7); !ok || at != (headless.Caret{}) {
		t.Fatalf("At(2, 0) = %#v, %v; want the start of the text", at, ok)
	}

	editor.SetCursor(0, 0)
	if editor.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("mouse press in the committed gutter was consumed")
	}

	spans := editor.Spans(headless.Caret{}, headless.Caret{Col: 2}, 7)
	if len(spans) != 1 || spans[0].Col != 2 {
		t.Fatalf("Spans = %#v, want columns aligned past the gutter", spans)
	}
}

func TestMaskedEditorDoesNotDiscloseItsValueToAGutter(t *testing.T) {
	gutter := &recordingGutter{}
	editor := headless.NewEditor()
	editor.Gutter = gutter
	editor.Mask = "•"
	editor.SetText("secret")
	paintWidget(10, 1, editor)

	if len(gutter.rows) != 1 || gutter.rows[0].Text != "••••••" {
		t.Fatalf("gutter rows = %#v, want only the displayed mask", gutter.rows)
	}
}
