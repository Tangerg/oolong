package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestEmptyLaTeXKeepsTheEnclosingListItem(t *testing.T) {
	screen, err := newContent(nil)
	if err != nil {
		t.Fatal(err)
	}
	screen.show(markdownFormat, "1. ```math\n   ```\n\n   visible")
	if screen.renderErr != nil {
		t.Fatal(screen.renderErr)
	}
	if height := screen.body.HeightForWidth(30); height != 1 {
		t.Fatalf("item height = %d, want one visible paragraph", height)
	}
	surface := grid.NewSurface(30, 1)
	screen.body.Draw(surface.View())
	for column, want := range map[int]string{0: "1", 1: ".", 3: "v"} {
		cell, ok := surface.CellAt(column, 0)
		if !ok || cell.Content() != want {
			t.Fatalf("cell %d = %q, want %q", column, cell.Content(), want)
		}
	}
	projector, ok := screen.body.(text.Projector)
	if !ok {
		t.Fatal("composed Markdown lost its text projection")
	}
	if got := projector.Rows(30); len(got) != 1 || got[0].Content.String() != "visible" || got[0].Offset != 3 {
		t.Fatalf("projected list item = %+v", got)
	}
}
