package markdown_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestStreamingReferencesKeepTheirDocumentMeaning(t *testing.T) {
	cases := []struct {
		name, source, destination, labels string
	}{
		{
			name:        "earlier definition shared by later blocks",
			source:      "[ref]: https://example.test/first \"title\"\n\n[first][ref]\n\n[second][ref]\n\nend\n",
			destination: "https://example.test/first",
			labels:      "firstsecond",
		},
		{
			name:        "first normalized definition wins",
			source:      "[A B]: /first\n\nfirst\n\n[a  b]: /second\n\n[link][a b]\n",
			destination: "/first",
			labels:      "link",
		},
		{
			name:        "unfinished definition is revised",
			source:      "[ref]: /partial\n\n[link][ref]\n",
			destination: "/partial",
			labels:      "link",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, chunk := range []int{1, 3, 17, len(tc.source)} {
				t.Run(strconv.Itoa(chunk), func(t *testing.T) {
					renderer := markdown.NewRenderer(markdown.RendererConfig{Look: markdown.Look{Link: grid.Style{Attr: grid.Underline}}})
					body := preparedStream(t, tc.source, chunk, renderer)
					var linked strings.Builder
					for _, row := range captured(t, body, 60) {
						for _, cell := range row {
							if cell.Link == "" {
								continue
							}
							if cell.Link != tc.destination || !cell.Style.Attr.Has(grid.Underline) {
								t.Fatalf("linked cell = %+v, want underlined destination %q", cell, tc.destination)
							}
							linked.WriteString(cell.Content())
						}
					}
					if linked.String() != tc.labels {
						t.Fatalf("linked text = %q, want %q", linked.String(), tc.labels)
					}
				})
			}
		})
	}
}
