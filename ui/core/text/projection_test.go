package text_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

type shownAtom struct {
	column  int
	content string
	width   int
	style   grid.Style
	link    string
}

func shownAtoms(t *testing.T, line text.Line) []shownAtom {
	t.Helper()
	width := max(1, line.Width())
	surface := grid.NewSurface(width, 1)
	if advanced := line.Draw(surface.View(), 0, 0); advanced != line.Width() {
		t.Fatalf("Draw advanced %d, Width reports %d", advanced, line.Width())
	}
	var atoms []shownAtom
	for column := range width {
		cell := cellAt(surface, column)
		if cell.Content() != "" {
			atoms = append(atoms, shownAtom{column, cell.Content(), cell.Width(), cell.Style, cell.Link})
		}
	}
	return atoms
}

func TestFilteredDisplayKeepsAtomGeometryAndAppearance(t *testing.T) {
	first := grid.Style{Attr: grid.Bold}
	last := grid.Style{Attr: grid.Italic}
	const firstLink = "https://example.test/first"
	const lastLink = "https://example.test/last"
	for _, tc := range []struct {
		name, prefix, suffix string
		want                 []shownAtom
	}{
		{"regional indicators", "🇦", "🇧🇨", []shownAtom{{0, "🇦🇧", 2, first, firstLink}, {2, "🇨", 1, last, lastLink}}},
		{"emoji joiner", "👩\u200d", "💻", []shownAtom{{0, "👩‍💻", 2, first, firstLink}}},
		{"Indic linker", "क्", "क", []shownAtom{{0, "क्क", 2, first, firstLink}}},
		{"Hangul", "ᄀ", "ᅡ", []shownAtom{{0, "가", 2, first, firstLink}}},
	} {
		for _, separator := range []string{"\x00", "\xff"} {
			t.Run(tc.name+separator, func(t *testing.T) {
				line := text.Line{
					{Text: tc.prefix + separator, Style: first, Link: firstLink},
					{Text: tc.suffix, Style: last, Link: lastLink},
				}
				width := line.Width() + 1
				wrapped := line.Wrap(width)
				if len(wrapped) != 1 {
					t.Fatalf("fitting line wrapped into %d rows", len(wrapped))
				}
				for name, projected := range map[string]text.Line{
					"direct":   line,
					"wrap":     wrapped[0].Line,
					"truncate": line.Truncate(width, "…"),
					"suffix":   line.WithSuffix(width, ""),
					"pad":      line.Pad(0, layout.Start),
				} {
					t.Run(name, func(t *testing.T) {
						if got := shownAtoms(t, projected); !slices.Equal(got, tc.want) {
							t.Fatalf("atoms = %+v, want %+v", got, tc.want)
						}
					})
				}
				for _, wrap := range []bool{false, true} {
					block := text.NewBlock(text.BlockConfig{Lines: []text.Line{line}, Wrap: wrap})
					rows := block.Rows(width)
					if len(rows) != 1 || rows[0].Text != tc.prefix+tc.suffix {
						t.Fatalf("Block.Rows(%t) = %+v", wrap, rows)
					}
				}
			})
		}
	}
}

func TestLineDoesNotBorrowCellsForStandaloneModifiers(t *testing.T) {
	base := grid.Style{Attr: grid.Bold}
	const target = "https://example.test/existing"
	for _, tc := range []struct {
		name, value  string
		origin, left int
		advance      int
	}{
		{"line start", "\ufe0fx", 1, 0, 1},
		{"tab gap", "\t\ufe0fx", 0, text.TabStop - 1, text.TabStop + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			surface := grid.NewSurface(text.TabStop+3, 1)
			view := surface.View()
			view.Text(tc.left, 0, "❤", base)
			view.Link(tc.left, 0, 1, target)
			before := cellAt(surface, tc.left)
			line := text.Of(tc.value, grid.Style{Attr: grid.Italic})
			if got := line.Draw(view, tc.origin, 0); got != tc.advance || got != line.Width() {
				t.Fatalf("advance = %d, width = %d, want %d", got, line.Width(), tc.advance)
			}
			if got := cellAt(surface, tc.left); got != before {
				t.Fatalf("independent cell changed from %+v to %+v", before, got)
			}
			if got := cellAt(surface, tc.origin+tc.advance-1).Content(); got != "x" {
				t.Fatalf("following character = %q, want x", got)
			}
		})
	}
}

func TestFilteredClustersRetainOriginalBytePositions(t *testing.T) {
	const source = "🇦\x00🇧🇨"
	rows := text.Of(source, grid.Style{}).Wrap(2)
	if len(rows) != 2 || rows[0].Line.String() != "🇦🇧" || rows[1].Line.String() != "🇨" {
		t.Fatalf("rows = %+v", rows)
	}
	const second = len("🇦\x00🇧")
	if rows[0].From != 0 || rows[0].To != second || rows[1].From != second || rows[1].To != len(source) {
		t.Fatalf("source intervals = %+v", rows)
	}
	for _, tc := range []struct{ offset, column int }{{0, 0}, {len("🇦"), 2}, {second, 2}, {len(source), 3}} {
		if got := text.ColumnOf(source, tc.offset); got != tc.column {
			t.Errorf("ColumnOf(%d) = %d, want %d", tc.offset, got, tc.column)
		}
	}
	for _, tc := range []struct{ column, offset int }{{0, 0}, {1, 0}, {2, second}, {3, len(source)}} {
		if got := text.OffsetAt(source, tc.column); got != tc.offset {
			t.Errorf("OffsetAt(%d) = %d, want %d", tc.column, got, tc.offset)
		}
	}
	surface := grid.NewSurface(3, 1)
	text.Of(source, grid.Style{}).Draw(surface.View(), 0, 0)
	if column, width := text.StampLink(surface.View(), 0, 0, source, second, len(source), "https://example.test/last"); column != 2 || width != 1 {
		t.Fatalf("last cluster's link occupies [%d,%d), want [2,3)", column, column+width)
	}
}

func TestHighlightUsesTheFilteredDisplayCluster(t *testing.T) {
	const source = "🇦\x00🇧🇨"
	normal, matched := grid.Style{}, grid.Style{Attr: grid.Bold}
	for _, tc := range []struct {
		name    string
		offsets []int
		first   grid.Style
	}{
		{"retained byte", []int{len("🇦\x00")}, matched},
		{"discarded byte", []int{len("🇦")}, normal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := text.Highlight(source, tc.offsets, normal, matched)
			if line.String() != source {
				t.Fatalf("Highlight changed source bytes: %q", line.String())
			}
			want := []shownAtom{{0, "🇦🇧", 2, tc.first, ""}, {2, "🇨", 1, normal, ""}}
			if got := shownAtoms(t, line); !slices.Equal(got, want) {
				t.Fatalf("atoms = %+v, want %+v", got, want)
			}
		})
	}
}

func FuzzDisplayFilteringKeepsAtomOwnership(f *testing.F) {
	for _, value := range []string{"🇦\x00🇧🇨", "🇦\xff🇧🇨", "🇨\xac🇧0", "👩\u200d\x00💻", "क्\x00क", "ᄀ\xffᅡ", "\ufe0fx", "❤\n\ufe0fX", "a  b"} {
		f.Add(value, len(value)/2)
	}
	f.Fuzz(func(t *testing.T, value string, split int) {
		// Tab expansion intentionally changes unpainted gaps into printable spaces.
		if len(value) > 256 || strings.ContainsRune(value, '\t') {
			return
		}
		split = min(max(0, split), len(value))
		line := text.Line{{Text: value[:split], Style: grid.Style{Attr: grid.Bold}, Link: "https://example.test/first"}, {Text: value[split:], Style: grid.Style{Attr: grid.Italic}, Link: "https://example.test/last"}}
		want := shownAtoms(t, line)
		width := line.Width() + 1
		wrapped := line.Wrap(0)
		for _, projected := range []text.Line{wrapped[0].Line, line.Truncate(width, "…"), line.WithSuffix(width, "")} {
			if got := shownAtoms(t, projected); !slices.Equal(got, want) {
				t.Fatalf("projected atoms %+v, want %+v for %q split at %d", got, want, value, split)
			}
		}
	})
}

func FuzzFilteredCoordinatesFollowDisplay(f *testing.F) {
	for _, value := range []string{"🇦\x00🇧🇨", "🇨\xac🇧0", "👩\u200d\x00💻", "क्\x00क", "ᄀ\xffᅡ", "\ufe0fx", "❤\n\ufe0fX", "a\tb", "\x00", "a\x00"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 256 {
			return
		}
		width := text.Width(value)
		if end := text.ColumnOf(value, len(value)); end != width {
			t.Fatalf("source end maps to %d, display width is %d", end, width)
		}
		previous := 0
		for at := 0; at <= len(value); at++ {
			column := text.ColumnOf(value, at)
			if column < previous || column > width {
				t.Fatalf("ColumnOf(%q, %d) = %d outside [%d,%d]", value, at, column, previous, width)
			}
			previous = column
		}
		previous = 0
		for column := 0; column <= width; column++ {
			at := text.OffsetAt(value, column)
			if at < previous || at > len(value) || text.ColumnOf(value, at) > column {
				t.Fatalf("OffsetAt(%q, %d) = %d outside the preceding display boundary", value, column, at)
			}
			previous = at
		}
	})
}
