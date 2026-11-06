package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestEditorPreservesDesiredColumnAcrossWrappedElement(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	for _, test := range []struct {
		name   string
		column int
		from   int
		to     int
		move   func(*base.Editor)
	}{
		{name: "down zero", column: 0, from: 0, to: 2, move: (*base.Editor).MoveDown},
		{name: "down nonzero", column: 1, from: 0, to: 2, move: (*base.Editor).MoveDown},
		{name: "up zero", column: 0, from: 2, to: 0, move: (*base.Editor).MoveUp},
		{name: "up nonzero", column: 1, from: 2, to: 0, move: (*base.Editor).MoveUp},
	} {
		t.Run(test.name, func(t *testing.T) {
			var editor base.Editor
			editor.SetText("a\n\nwxyz")
			editor.SetCursor(1, 0)
			editor.InsertElement(1, "abcdefg")
			editor.SetCursor(test.from, test.column)
			fixture := scenes.Mount(&editor)
			t.Cleanup(fixture.Root.Close)
			fixture.Draw(grid.NewSurface(4, 8).View())
			for range 4 {
				test.move(&editor)
				line, byteOffset := editor.Cursor()
				if line == test.to {
					if byteOffset != test.column {
						t.Fatalf("vertical movement reached byte %d, want visual column %d", byteOffset, test.column)
					}
					return
				}
				editor.Elements()
				editor.HeightForWidth(4)
				editor.At(0, 0, 4)
				fixture.Draw(grid.NewSurface(4, 8).View())
			}
			t.Fatal("vertical movement did not reach the next logical line")
		})
	}
}

func TestSharedEditorViewsKeepIndependentDesiredColumns(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	session := base.NewEditSession(base.EditSessionConfig{Initial: "a\n\nwxyz"})
	first := base.NewEditor(base.EditorConfig{Session: session})
	first.SetCursor(1, 0)
	first.InsertElement(1, "abcdefg")
	views := []*base.Editor{first, base.NewEditor(base.EditorConfig{Session: session})}
	for column, editor := range views {
		editor.SetCursor(0, column)
		fixture := scenes.Mount(editor)
		t.Cleanup(fixture.Root.Close)
		fixture.Draw(grid.NewSurface(4, 8).View())
	}
	for range 4 {
		for _, editor := range views {
			if line, _ := editor.Cursor(); line < 2 {
				editor.MoveDown()
			}
		}
	}
	for column, editor := range views {
		if line, byteOffset := editor.Cursor(); line != 2 || byteOffset != column {
			t.Fatalf("view %d cursor = (%d,%d), want (2,%d)", column, line, byteOffset, column)
		}
	}
}
