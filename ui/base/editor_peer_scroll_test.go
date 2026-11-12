package base_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
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

func TestPeerEditsOnlyRefineAnUnconsumedCursorReveal(t *testing.T) {
	for _, mode := range []string{"pending", "committed", "aborted"} {
		t.Run(mode, func(t *testing.T) {
			scenes := scenetest.New()
			t.Cleanup(scenes.Close)
			session := base.NewEditSession(base.EditSessionConfig{Initial: strings.Repeat("line\n", 30)})
			writer := base.NewEditor(base.EditorConfig{Session: session})
			reader := base.NewEditor(base.EditorConfig{Session: session})
			paintWidget(scenes, 20, 3, reader)
			reader.SetCursor(10, 0)
			switch mode {
			case "committed":
				paintWidget(scenes, 20, 3, reader)
			case "aborted":
				prepared, err := scenes.Mount(reader).Prepare(grid.NewSurface(20, 3).View())
				if err != nil {
					t.Fatal(err)
				}
				prepared.Abort()
			}
			before := reader.Scroll().Offset()
			writer.Insert("added\n")
			want := before
			if mode != "committed" {
				want++
			}
			if got := reader.Scroll().Offset(); got != want {
				t.Fatalf("peer edit changed offset %d to %d, want %d", before, got, want)
			}
			paintWidget(scenes, 20, 3, reader)
			if got := reader.Scroll().Offset(); got != want {
				t.Fatalf("peer edit's frame offset = %d, want %d", got, want)
			}
		})
	}
}
