package markdown

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

type layoutPreparationChild struct {
	heightCalls, rowCalls int
	height                int
	onHeight, onRows      func()
}

func (p *layoutPreparationChild) HeightForWidth(int) int {
	p.heightCalls++
	if p.onHeight != nil {
		p.onHeight()
	}
	return p.height
}

func (*layoutPreparationChild) Draw(view grid.View) {
	view.Text(0, 0, "child", grid.Style{})
}

func (p *layoutPreparationChild) Rows(int) []text.Row {
	p.rowCalls++
	if p.onRows != nil {
		p.onRows()
	}
	return []text.Row{{Text: "child"}}
}

func TestDocumentMeasurementDoesNotPrepareAllTextRows(t *testing.T) {
	child := &layoutPreparationChild{height: 1 << 18}
	doc := &Snapshot{blocks: []Block{{content: child}}}
	if got := doc.HeightForWidth(20); got != child.height {
		t.Fatalf("height = %d, want %d", got, child.height)
	}
	surface := grid.NewSurface(20, 1)
	doc.Draw(surface.View())
	if child.heightCalls != 1 || child.rowCalls != 0 {
		t.Fatalf("measurement and clipped draw called child height %d times and rows %d times", child.heightCalls, child.rowCalls)
	}
	if _, ready := doc.PreparedLayout(20); ready {
		t.Fatal("geometry alone was reported as fully prepared")
	}
}

func TestWidthPreparationWarmsMeasurementAndRows(t *testing.T) {
	child := &layoutPreparationChild{height: 2}
	doc := &Snapshot{blocks: []Block{{content: child}}}
	if err := doc.PrepareWidth(t.Context(), 20); err != nil {
		t.Fatal(err)
	}
	prepared, ready := doc.PreparedLayout(20)
	if !ready || prepared == nil {
		t.Fatal("prepared width was not available")
	}
	for range 3 {
		if got := doc.HeightForWidth(20); got != 2 {
			t.Fatalf("height = %d", got)
		}
		rows := doc.Rows(20)
		if len(rows) != 2 || rows[0].Text != "child" || rows[1].Text != "" {
			t.Fatalf("rows = %+v", rows)
		}
		rows[0].Text = "caller mutation"
		if _, err := grid.Capture(grid.CaptureConfig{Content: prepared, Width: 20}); err != nil {
			t.Fatal(err)
		}
	}
	if child.heightCalls != 1 || child.rowCalls != 1 {
		t.Fatalf("UI reads repeated preparation: height=%d rows=%d", child.heightCalls, child.rowCalls)
	}
	if err := doc.PrepareWidth(t.Context(), 12); err != nil {
		t.Fatal(err)
	}
	if _, ready := doc.PreparedLayout(20); ready {
		t.Fatal("the document retained a second width cache")
	}
	if _, ready := doc.PreparedLayout(12); !ready || child.heightCalls != 2 || child.rowCalls != 2 {
		t.Fatalf("new width not fully prepared: ready=%v height=%d rows=%d", ready, child.heightCalls, child.rowCalls)
	}
}

func TestCancelledLayoutPreparationNeverPublishesReadyRows(t *testing.T) {
	for _, phase := range []string{"admission", "geometry", "rows"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			cause := errors.New("preparation superseded")
			cancelPreparation := func() { cancel(cause) }
			child := &layoutPreparationChild{height: 1}
			switch phase {
			case "admission":
				cancelPreparation()
			case "geometry":
				child.onHeight = cancelPreparation
			case "rows":
				child.onRows = cancelPreparation
			}
			doc := &Snapshot{blocks: []Block{{content: child}}}
			if err := doc.PrepareWidth(ctx, 20); !errors.Is(err, cause) {
				t.Fatalf("preparation lost cancellation: %v", err)
			}
			if _, ready := doc.PreparedLayout(20); ready {
				t.Fatal("cancelled preparation published ready rows")
			}
			if phase == "admission" && (child.heightCalls != 0 || child.rowCalls != 0) {
				t.Fatal("cancelled admission invoked the child")
			}
			child.onHeight, child.onRows = nil, nil
			if err := doc.PrepareWidth(t.Context(), 20); err != nil {
				t.Fatal(err)
			}
			if _, ready := doc.PreparedLayout(20); !ready {
				t.Fatal("cancelled preparation poisoned a later request")
			}
		})
	}
}
