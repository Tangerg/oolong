package base_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

// An edit in one view used to reveal every other view's caret, taking a reader
// who had scrolled away back to wherever their caret had been left.
func TestAPeerEditKeepsAnotherReadersScroll(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	session := base.NewEditSession(base.EditSessionConfig{Initial: strings.Repeat("line\n", 200)})
	writer := base.NewEditor(base.EditorConfig{Session: session})
	reader := base.NewEditor(base.EditorConfig{Session: session})
	reader.SetCursor(199, 0)
	paintWidget(scenes, 20, 5, reader)
	reader.Scroll().ToTop()
	paintWidget(scenes, 20, 5, reader)
	if reader.Scroll().Offset() != 0 {
		t.Fatalf("fixture offset = %d", reader.Scroll().Offset())
	}
	writer.SetCursor(0, 0)
	writer.InsertRune('x')
	paintWidget(scenes, 20, 5, reader)
	if got := reader.Scroll().Offset(); got != 0 {
		t.Fatalf("another view's edit scrolled the reader to %d", got)
	}
}
