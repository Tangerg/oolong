package text_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestMalformedUTF8PreservesValidNeighbors(t *testing.T) {
	for _, tc := range []struct {
		source string
		drawn  string
		width  int
	}{
		{"000000000\xd7", "000000000", 9},
		{"A\xe2", "A", 1},
		{"A\xf0\x9f", "A", 1},
		{"A\xe2B", "AB", 2},
		{"A\xf0BC", "ABC", 3},
		{"\xe2AB", "AB", 2},
		{"\xf0\x9f", "", 0},
		{"中\xe2👩‍💻x", "中👩‍💻x", 5},
		{"A\ufffdB", "A\ufffdB", 3},
	} {
		t.Run(tc.source, func(t *testing.T) {
			if got := text.Width(tc.source); got != tc.width {
				t.Fatalf("Width = %d, want %d", got, tc.width)
			}
			line := text.Of(tc.source, grid.Style{})
			surface := grid.NewSurface(tc.width+1, 1)
			if got := line.Draw(surface.View(), 0, 0); got != tc.width {
				t.Fatalf("Line.Draw = %d, want %d", got, tc.width)
			}
			if got := surface.Rows()[0]; got != tc.drawn {
				t.Fatalf("Line.Draw content = %q, want %q", got, tc.drawn)
			}
			surface.Reset()
			if got := surface.View().Text(0, 0, tc.source, grid.Style{}); got != tc.width {
				t.Fatalf("View.Text = %d, want %d", got, tc.width)
			}
			if got := surface.Rows()[0]; got != tc.drawn {
				t.Fatalf("View.Text content = %q, want %q", got, tc.drawn)
			}
			if got := surface.View().Text(0, 1, tc.source, grid.Style{}); got != tc.width {
				t.Fatalf("clipped View.Text = %d, want %d", got, tc.width)
			}
			if got := line.Wrap(tc.width + 1); len(got) != 1 || got[0].Line.String() != tc.drawn {
				t.Fatalf("Wrap = %#v, want one row containing %q", got, tc.drawn)
			}
			if got := line.Truncate(tc.width+1, "…").String(); got != tc.drawn {
				t.Fatalf("Truncate = %q, want %q", got, tc.drawn)
			}
		})
	}
}

func TestMalformedUTF8KeepsTheFirstByteSpanAsAppearanceOwner(t *testing.T) {
	const source = "A\xe2e\u0301B"
	clusters := []string{"A", "e\u0301", "B"}
	for split := 0; split <= len(source); split++ {
		line := text.Line{
			{Text: source[:split], Style: grid.Style{Attr: grid.Bold}},
			{Text: source[split:]},
		}
		surface := grid.NewSurface(len(clusters), 1)
		line.Draw(surface.View(), 0, 0)
		if got := surface.Rows()[0]; got != strings.Join(clusters, "") {
			t.Fatalf("split %d: drew %q", split, got)
		}
		for column, cluster := range clusters {
			want := grid.Style{}
			if strings.Index(source, cluster) < split {
				want.Attr = grid.Bold
			}
			if got := cellAt(surface, column).Style; got != want {
				t.Fatalf("split %d, cluster %q: style = %v, want %v", split, cluster, got, want)
			}
		}
	}
}

func TestTruncateSuffixCannotLoseValidTextBesideMalformedUTF8(t *testing.T) {
	const budget = 1
	line := text.Of("long", grid.Style{}).Truncate(budget, "X\xe2Y")
	if got := line.Width(); got != budget {
		t.Fatalf("Width = %d, want %d", got, budget)
	}
	if got := grid.Render(budget, 1, func(view grid.View) { line.Draw(view, 0, 0) })[0]; got != "X" {
		t.Fatalf("Truncate suffix drew %q, want X", got)
	}
}

func FuzzClustersPartitionOriginalBytes(f *testing.F) {
	for _, seed := range []string{"", "plain ASCII", "中👩‍💻ｶﾞ", "A\xe2B", "A\xf0BC", "A\xe2\u0301B", "A\ufffdB"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		end := 0
		for at, cluster := range text.Clusters(source) {
			if at != end || cluster == "" || len(cluster) > len(source)-at {
				t.Fatalf("cluster %q at %d does not partition source after %d", cluster, at, end)
			}
			end = at + len(cluster)
			if source[at:end] != cluster {
				t.Fatalf("cluster %q at %d changed source bytes", cluster, at)
			}
			if !utf8.ValidString(cluster) && len(cluster) != 1 {
				t.Fatalf("malformed cluster %q at %d absorbed adjacent bytes", cluster, at)
			}
		}
		if end != len(source) {
			t.Fatalf("segmentation ended at %d, source has %d bytes", end, len(source))
		}
	})
}
