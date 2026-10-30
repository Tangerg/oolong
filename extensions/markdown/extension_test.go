package markdown_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/content"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

type drawing struct{}

func (drawing) HeightForWidth(width int) int {
	if width <= 0 {
		return 0
	}
	return 2
}

func (drawing) Draw(v grid.View) {
	text.Of("diagram", grid.Style{}).Draw(v, 0, 0)
	text.Of("second", grid.Style{}).Draw(v, 0, 1)
}

func TestEmbeddedDrawableRetainsDrawingAndBlankTextRows(t *testing.T) {
	appearance := look()
	calls := 0
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) { calls++; return drawing{}, nil })
	blocks := mustRender(t, "> ```mermaid\n> graph TD\n> ```\n\nafter", appearance)
	equal(t, rows(t, 12, blocks), []string{"│ diagram", "│ second", "", "after"})
	doc := new(markdown.Doc)
	doc.SetBlocks(blocks)
	projected := doc.Rows(12)
	if len(projected) != doc.HeightForWidth(12) || projected[0].Text != "" || projected[0].Offset != 2 {
		t.Fatalf("projection=%+v", projected)
	}
	for _, width := range []int{0, 1, 2, 3, 20} {
		if got := len(doc.Rows(width)); got != doc.HeightForWidth(width) {
			t.Fatalf("width %d projection height %d", width, got)
		}
	}
	if calls != 1 {
		t.Fatalf("layout invoked renderer %d times", calls)
	}
}

func TestExtensionErrorsKeepReadableContentAndIdentity(t *testing.T) {
	failure := errors.New("backend unavailable")
	for _, tt := range []struct {
		name      string
		result    grid.Drawable
		err       error
		wantError bool
		want      string
	}{
		{name: "failed", err: failure, wantError: true, want: "source"},
		{name: "partial", result: text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of("partial", grid.Style{})}}), err: failure, wantError: true, want: "partial"},
		{name: "nil", wantError: true, want: "source"},
		{name: "typed nil", result: (*text.Block)(nil), wantError: true, want: "source"},
		{name: "decline", err: markdown.ErrUnhandled, want: "source"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			appearance := look()
			appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) { return tt.result, tt.err })
			blocks, err := markdown.Render("```custom\nsource\n```", appearance)
			if (err != nil) != tt.wantError {
				t.Fatalf("error=%v", err)
			}
			if errors.Is(tt.err, failure) && !errors.Is(err, failure) {
				t.Fatal("lost error identity")
			}
			if err != nil {
				var extension *markdown.ExtensionError
				if !errors.As(err, &extension) || extension.Block != 0 || extension.Info != "custom" {
					t.Fatalf("location=%+v", extension)
				}
			}
			equal(t, rows(t, 20, blocks), []string{tt.want})
		})
	}
}

func TestPreparedDocumentKeepsErrorsAndChildrenStable(t *testing.T) {
	failure := errors.New("broken extension")
	calls := 0
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) { calls++; return drawing{}, failure })
	renderer := markdown.NewRenderer(markdown.RendererConfig{Look: appearance})
	doc, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Append([]byte("```custom\nx\n```\n")); err != nil {
		t.Fatal(err)
	}
	if err := doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
		t.Fatal(err)
	}
	first := doc.Current()
	if !errors.Is(first.PrepareError, failure) || calls != 1 {
		t.Fatalf("result error=%v calls=%d", first.PrepareError, calls)
	}
	_ = doc.Current().Body.HeightForWidth(20)
	_ = doc.Current().Body.HeightForWidth(30)
	if calls != 1 {
		t.Fatal("read or layout reran renderer")
	}
	if err := doc.End(nil); err != nil {
		t.Fatal(err)
	}
	if err := doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
		t.Fatal(err)
	}
	if doc.Current().State != content.Partial || !errors.Is(doc.Current().PrepareError, failure) {
		t.Fatalf("final result = %+v", doc.Current())
	}
	firstRows := captured(t, first.Body, 20)
	if firstRows[0][0].Content() != "d" {
		t.Fatal("later preparation changed prior result")
	}
}

func TestNestedListDrawableKeepsIndentationAndClipsAtNarrowWidths(t *testing.T) {
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) { return drawing{}, nil })
	blocks := mustRender(t, "- outer\n  - inner\n\n    ```mermaid\n    graph TD\n    ```", appearance)
	equal(t, rows(t, 20, blocks), []string{"• outer", "  • inner", "", "    diagram", "    second"})
	doc := new(markdown.Doc)
	doc.SetBlocks(blocks)
	for _, width := range []int{1, 3, 4, 5} {
		surface := grid.NewSurface(width, doc.HeightForWidth(width))
		doc.Draw(surface.View())
		if len(doc.Rows(width)) != doc.HeightForWidth(width) {
			t.Fatal("narrow projection and drawing differ")
		}
	}
}

func TestDeclineJoinedWithFailureDoesNotHideDiagnostic(t *testing.T) {
	failure := errors.New("failed after declining")
	appearance := look()
	appearance.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) { return nil, errors.Join(markdown.ErrUnhandled, failure) })
	blocks, err := markdown.Render("```custom\nsource\n```", appearance)
	if !errors.Is(err, failure) {
		t.Fatal("joined failure was suppressed")
	}
	equal(t, rows(t, 20, blocks), []string{"source"})
}
