package headless

import (
	"strings"
	"testing"
)

func BenchmarkEditorInsertAndBackspace(b *testing.B) {
	var editor Editor
	editor.SetText(strings.Repeat("a", 160))
	b.ReportAllocs()
	for b.Loop() {
		editor.InsertRune('x')
		editor.DeleteBack()
	}
}

func BenchmarkEditorCursorOnLongLine(b *testing.B) {
	var editor Editor
	editor.SetText(strings.Repeat("a", 10_000))
	b.ReportAllocs()
	for b.Loop() {
		editor.SetCursor(0, 5_000)
		editor.MoveRight()
	}
}
