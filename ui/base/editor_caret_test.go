package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestEditorFullRowsKeepTheInsertionCaretVisible(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	for _, value := range []string{"abcd", "abcdefgh", "界界", "a\u0301bcd"} {
		t.Run(value, func(t *testing.T) {
			editor := &base.Editor{}
			editor.SetText(value)
			editor.SetCursor(0, len(value))
			fixture := scenes.Mount(editor)
			t.Cleanup(fixture.Root.Close)
			screen := grid.NewScreen(4, 1)
			fixture.Draw(screen.Frame())
			if cursor := screen.Cursor(); !cursor.Visible || cursor.Pos != image.Pt(0, 0) {
				t.Fatalf("full-row insertion cursor = %+v", cursor)
			}
			if at, ok := editor.At(0, 0, 4); !ok || at != (base.Caret{Byte: len(value)}) {
				t.Fatalf("scrolled insertion row hit = %+v, %t", at, ok)
			}
			editor.MoveUp()
			_, above := editor.Cursor()
			if above >= len(value) {
				t.Fatal("up did not leave the insertion row")
			}
			editor.MoveDown()
			if line, column := editor.Cursor(); line != 0 || column != len(value) {
				t.Fatalf("down missed insertion row: (%d, %d)", line, column)
			}
			screen = grid.NewScreen(5, 4)
			fixture.Draw(screen.Frame())
			if cursor := screen.Cursor(); !cursor.Visible || cursor.Pos.X >= 5 || cursor.Pos.Y >= editor.HeightForWidth(5) {
				t.Fatalf("resized insertion cursor = %+v", cursor)
			}
		})
	}
}
