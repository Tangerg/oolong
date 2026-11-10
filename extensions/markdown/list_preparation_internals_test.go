package markdown

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

type listPreparationChild struct {
	heightCalls, rowCalls atomic.Int64
	onHeight, onRows      func()
}

func (p *listPreparationChild) HeightForWidth(int) int {
	p.heightCalls.Add(1)
	if p.onHeight != nil {
		p.onHeight()
	}
	return 1
}

func (*listPreparationChild) Draw(view grid.View) {
	view.Text(0, 0, "x", grid.Style{})
}

func (p *listPreparationChild) Rows(int) []text.Row {
	p.rowCalls.Add(1)
	if p.onRows != nil {
		p.onRows()
	}
	return []text.Row{{Content: text.Of("x", grid.Style{})}}
}

func TestPreparedListsRetainGeometryAcrossSourceWidths(t *testing.T) {
	for _, source := range []struct{ name, text string }{
		{"unordered", "- ```custom\n  x\n  ```\n"},
		{"ordered", "1. ```custom\n   x\n   ```\n"},
		{"nested", "- outer\n  1. nested\n     - ```custom\n       x\n       ```\n"},
		{"quoted", "> - ```custom\n>   x\n>   ```\n"},
	} {
		for _, sharing := range []struct {
			name             string
			separateSnapshot bool
		}{
			{name: "same snapshot"},
			{name: "shared blocks", separateSnapshot: true},
		} {
			t.Run(source.name+"/"+sharing.name, func(t *testing.T) {
				child := &listPreparationChild{}
				look := Look{Glyphs: Glyphs{Bullet: "•", Bar: "│"}}
				look.SetRenderer(FencedCode, func(string, string) (grid.Drawable, error) { return child, nil })
				var first, second Doc
				first.SetBlocks(mustRender(t, source.text, look))
				other := first.view
				if sharing.separateSnapshot {
					second.SetBlocks(first.Blocks())
					other = second.view
				}
				if err := first.view.PrepareWidth(t.Context(), 20); err != nil {
					t.Fatal(err)
				}
				prepared, ready := first.view.PreparedLayout(20)
				if !ready {
					t.Fatal("prepared list is unavailable")
				}
				height := prepared.HeightForWidth(20)
				before := captureDraw(t, 20, height, prepared.Draw)
				if err := other.PrepareWidth(t.Context(), 10); err != nil {
					t.Fatal(err)
				}
				if child.heightCalls.Load() != 2 || child.rowCalls.Load() != 2 {
					t.Fatalf("two widths prepared height %d times and rows %d times", child.heightCalls.Load(), child.rowCalls.Load())
				}
				if after := captureDraw(t, 20, height, prepared.Draw); after != before {
					t.Fatal("another width changed the retained drawing")
				}
				if _, err := grid.Capture(grid.CaptureConfig{Content: prepared, Width: 20}); err != nil {
					t.Fatal(err)
				}
				if child.heightCalls.Load() != 2 || child.rowCalls.Load() != 2 {
					t.Fatalf("retained drawing repeated preparation: height=%d rows=%d", child.heightCalls.Load(), child.rowCalls.Load())
				}
			})
		}
	}
}

func TestListPreparationObservesCancellationBetweenChildren(t *testing.T) {
	for _, phase := range []string{"geometry", "rows"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			cause := errors.New("nested preparation superseded")
			cancelPreparation := func() { cancel(cause) }
			child := &listPreparationChild{}
			if phase == "geometry" {
				child.onHeight = cancelPreparation
			} else {
				child.onRows = cancelPreparation
			}
			var look Look
			look.SetRenderer(FencedCode, func(string, string) (grid.Drawable, error) { return child, nil })
			var doc Doc
			doc.SetBlocks(mustRender(t, "- outer\n\n  1. ```custom\n     first\n     ```\n\n     ```custom\n     second\n     ```\n", look))
			if err := doc.view.PrepareWidth(ctx, 20); !errors.Is(err, cause) {
				t.Fatalf("preparation lost cancellation: %v", err)
			}
			if _, ready := doc.view.PreparedLayout(20); ready {
				t.Fatal("cancelled list preparation published a ready tree")
			}
			if phase == "geometry" {
				if child.heightCalls.Load() != 1 || child.rowCalls.Load() != 0 {
					t.Fatalf("cancelled geometry continued: height=%d rows=%d", child.heightCalls.Load(), child.rowCalls.Load())
				}
			} else if child.heightCalls.Load() != 2 || child.rowCalls.Load() != 1 {
				t.Fatalf("cancelled projection continued: height=%d rows=%d", child.heightCalls.Load(), child.rowCalls.Load())
			}
			child.onHeight, child.onRows = nil, nil
			if err := doc.view.PrepareWidth(t.Context(), 20); err != nil {
				t.Fatal(err)
			}
			if _, ready := doc.view.PreparedLayout(20); !ready {
				t.Fatal("cancelled preparation poisoned a later request")
			}
		})
	}
}

func TestListMeasurementDoesNotMaterializeItsTextExtent(t *testing.T) {
	child := &layoutPreparationChild{height: 1 << 18}
	var look Look
	look.SetRenderer(FencedCode, func(string, string) (grid.Drawable, error) { return child, nil })
	var doc Doc
	doc.SetBlocks(mustRender(t, "- ```custom\n  x\n  ```\n", look))
	if got := doc.HeightForWidth(20); got != child.height {
		t.Fatalf("height=%d, want %d", got, child.height)
	}
	doc.Draw(grid.NewSurface(20, 1).View())
	if child.heightCalls != 1 || child.rowCalls != 0 {
		t.Fatalf("clipped list prepared height %d times and rows %d times", child.heightCalls, child.rowCalls)
	}
}

func TestPreparedListCanBeDrawnWhileAnotherWidthIsPrepared(t *testing.T) {
	child := &listPreparationChild{}
	var look Look
	look.SetRenderer(FencedCode, func(string, string) (grid.Drawable, error) { return child, nil })
	var doc Doc
	doc.SetBlocks(mustRender(t, "- ```custom\n  x\n  ```\n", look))
	if err := doc.view.PrepareWidth(t.Context(), 20); err != nil {
		t.Fatal(err)
	}
	prepared, ready := doc.view.PreparedLayout(20)
	if !ready {
		t.Fatal("prepared list is unavailable")
	}
	want := captureDraw(t, 20, prepared.HeightForWidth(20), prepared.Draw)
	if err := doc.view.PrepareWidth(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Go(func() {
		for range 20 {
			if err := doc.view.PrepareWidth(t.Context(), 10); err != nil {
				t.Error(err)
				return
			}
		}
	})
	workers.Go(func() {
		for range 20 {
			if got := captureDraw(t, 20, prepared.HeightForWidth(20), prepared.Draw); got != want {
				t.Error("concurrent preparation changed retained drawing")
				return
			}
		}
	})
	workers.Wait()
	if child.heightCalls.Load() != 2 || child.rowCalls.Load() != 2 {
		t.Fatalf("concurrent drawing repeated preparation: height=%d rows=%d", child.heightCalls.Load(), child.rowCalls.Load())
	}
}
