package markdown_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func preparedStream(tb testing.TB, source string, chunk int, renderer content.Renderer) grid.Drawable {
	tb.Helper()
	doc, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		tb.Fatal(err)
	}
	for at := 0; at < len(source); at += chunk {
		if err := doc.Append([]byte(source[at:min(at+chunk, len(source))])); err != nil {
			tb.Fatal(err)
		}
		if err := doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
			tb.Fatal(err)
		}
		if doc.Current().Publishable() {
			tb.Fatal("provisional source became permanent")
		}
	}
	if err := doc.End(nil); err != nil {
		tb.Fatal(err)
	}
	if doc.Current().Publishable() {
		tb.Fatal("source completion reused a provisional parse")
	}
	if err := doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
		tb.Fatal(err)
	}
	if !doc.Current().Publishable() || doc.Current().State != content.Final {
		tb.Fatalf("final state = %+v", doc.Current())
	}
	return doc.Current().Body
}

func captured(tb testing.TB, body grid.Drawable, width int) [][]grid.Cell {
	tb.Helper()
	frozen, err := grid.Capture(grid.CaptureConfig{Content: body, Width: width})
	if err != nil {
		tb.Fatal(err)
	}
	rows, err := frozen.Rows(width)
	if err != nil {
		tb.Fatal(err)
	}
	return rows
}

func TestStreamingFinalSemanticsEqualWholeSource(t *testing.T) {
	sources := []string{
		"[earlier][ref]\n\nsecond\n\n[ref]: https://example.test/final\n",
		"[A B]: /first\n\nfirst\n\n[a  b]: /second\n\n[link][a b]\n",
		"- ```\n  first\n\n  *second*\n  ```\n\nafter\n",
		"```\nfirst\n    ```\n\n*second*\n```\n\nafter\n",
		"```{.python caption=\"x\"}\none\n\ntwo\n```\n\nafter\n",
		"> ```\n> one\n>\n> two\n> ```\n\nafter\n",
		"- step\n\n      ```\n      one\n\n      two\n      ```\n\nafter\n",
		"<div>\n\n```\none\n\ntwo\n```\n\n</div>\n\nafter\n",
		"$$\na\n\nb\n$$\n\nafter\n",
		"a paragraph\n\n```go\nfmt.Println()\n```\n\nafter\n",
		"1. first\n\n1. second\n\n1. third\n\nafter\n",
		"1. first\n\n1.\n\n1. third\n\nafter\n",
		"- group\n\n  1. first\n\n  1. second\n\n  1. third\n\nafter\n",
		"> 1. first\n>\n> 1. second\n>\n> 1. third\n\nafter\n",
		"# title\n\none\n\n- a\n- b\n\ntwo\n",
		"<!--\n\n-->\n\nafter\n", "<?\n\n?>\n\nafter\n",
		"<![CDATA[\n\n]]>\n\nafter\n", "<script>\n\n</script>\n\nafter\n",
		"before\n\n<script>\nhidden\n\nnot prose\n</script>\n\nafter\n",
		"<!-- comment\n\nnot prose\n-->\n\nafter\n",
		"```a`b\n\nparagraph\n\nnext\n", "<!--\n```\n-->\n\nparagraph\n",
		"中👩‍💻文\xe2\x80", "first\r\n\r\nsecond\r\n", "first\r\rsecond\r",
	}
	renderer := markdown.NewRenderer(markdown.RendererConfig{Look: look()})
	for _, source := range sources {
		whole, err := renderer(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		want := captured(t, whole, 40)
		for _, chunk := range []int{1, 3, 7, 17, len(source)} {
			got := captured(t, preparedStream(t, source, chunk, renderer), 40)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("chunk %d source %q:\n%+v\nwant\n%+v", chunk, source, got, want)
			}
		}
	}
}

func TestLateReferenceRevisesProvisionalContent(t *testing.T) {
	doc, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		t.Fatal(err)
	}
	renderer := markdown.NewRenderer(markdown.RendererConfig{})
	if err := doc.Append([]byte("[label][target]\n\n")); err != nil {
		t.Fatal(err)
	}
	if err := doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
		t.Fatal(err)
	}
	preview := captured(t, doc.Current().Body, 40)
	if preview[0][0].Link != "" || doc.Current().Publishable() {
		t.Fatal("preview invented a permanent link")
	}
	if err := doc.Append([]byte("[target]: https://example.test/late\n")); err != nil {
		t.Fatal(err)
	}
	if err := doc.End(nil); err != nil {
		t.Fatal(err)
	}
	if err := doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
		t.Fatal(err)
	}
	final := captured(t, doc.Current().Body, 40)
	if final[0][0].Link != "https://example.test/late" || !doc.Current().Publishable() {
		t.Fatal("final parse lost late reference")
	}
}

func TestRendererOwnsAppearanceAndDoesNotShareDocumentState(t *testing.T) {
	headings := []grid.Style{{Attr: grid.Bold}}
	appearance := markdown.Look{Headings: headings}
	renderer := markdown.NewRenderer(markdown.RendererConfig{Look: appearance})
	headings[0] = grid.Style{Attr: grid.Italic}
	first, err := renderer(context.Background(), "# first\n\n[ref]: /previous\n")
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderer(context.Background(), "[link][ref]\n")
	if err != nil {
		t.Fatal(err)
	}
	if !captured(t, first, 40)[0][0].Style.Attr.Has(grid.Bold) {
		t.Fatal("caller changed captured look")
	}
	if captured(t, second, 40)[0][0].Link != "" {
		t.Fatal("references leaked between documents")
	}
}

func TestStreamBudgetRefusesWithoutChangingLanguageOrSource(t *testing.T) {
	doc, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown", MaxSourceBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Append([]byte("**bold")); err != nil {
		t.Fatal(err)
	}
	snapshot := doc.Snapshot()
	if err := doc.Append([]byte(strings.Repeat("x", 9))); !errors.Is(err, content.ErrSourceLimit) {
		t.Fatalf("limit error = %v", err)
	}
	if doc.Snapshot().Revision() != snapshot.Revision() || doc.Snapshot().Text() != snapshot.Text() {
		t.Fatal("refused bytes altered the document")
	}
}
