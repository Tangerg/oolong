package markdown

import (
	"context"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
)

// Comparing complete cells includes links and styles, not just visible text.
// Full-source parsing is the oracle for every possible transport partition.
func FuzzStreamSaysTheSameThingHoweverItArrives(f *testing.F) {
	for _, source := range []string{
		"plain text", "$$\n\\frac{a}{b}\n$$\n", "```math\nx^2\n```\n", "$$\nnot closed",
		"> - nested\n>\n> content\n", "```a`b\n\nafter\n\nmore\n", "<!--\n```\n-->\n\nafter\n",
		"- ```go\n  x\n  ```\n\nafter\n", "one\n\ntwo\n\nthree\n",
		"[ref]: /target\n\n[first][ref]\n\n[second][ref]\n\nend\n",
		"[first][ref]\n\n[ref]: /late\n", "| a | b |\n| - | - |\n| 1 | 2 |\n\nafter\n",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 8<<10 {
			t.Skip()
		}
		renderer := NewRenderer(RendererConfig{})
		whole, err := renderer(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		want, err := grid.Capture(grid.CaptureConfig{Content: whole, Width: 40})
		if err != nil {
			t.Fatal(err)
		}
		wantRows, err := want.Rows(40)
		if err != nil {
			t.Fatal(err)
		}
		for _, chunk := range []int{1, 3, 17} {
			doc, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
			if err != nil {
				t.Fatal(err)
			}
			for at := 0; at < len(source); at += chunk {
				if err = doc.Append([]byte(source[at:min(at+chunk, len(source))])); err != nil {
					t.Fatal(err)
				}
				if err = doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
					t.Fatal(err)
				}
			}
			if err = doc.End(nil); err != nil {
				t.Fatal(err)
			}
			if err = doc.Accept(doc.Snapshot().Prepare(context.Background(), renderer)); err != nil {
				t.Fatal(err)
			}
			got, err := grid.Capture(grid.CaptureConfig{Content: doc.Current().Body, Width: 40})
			if err != nil {
				t.Fatal(err)
			}
			gotRows, err := got.Rows(40)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotRows, wantRows) {
				t.Fatalf("chunk %d changed final cells for %q", chunk, source)
			}
		}
	})
}
