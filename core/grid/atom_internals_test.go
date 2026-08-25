package grid

import (
	"image"
	"slices"
	"strings"
	"testing"
)

// FuzzSurfaceMaintainsDisplayAtoms protects the private representation rather
// than one spelling of the drawing code. All six public cell-changing operations
// may meet a previous atom at either edge; none may split its geometry or
// appearance.
func FuzzSurfaceMaintainsDisplayAtoms(f *testing.F) {
	for _, seed := range [][]byte{
		{8, 0, 0, 0, 0, 0},
		{4, 1, 2, 3, 4, 5, 6, 7},
		{16, 255, 128, 64, 32, 16, 8, 4, 2, 1},
	} {
		f.Add(seed)
	}

	texts := [...]string{"x", "中", "ｶﾞ", "あﾞ", "é", "🙂", "\t"}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		width := 1 + int(data[0])%16
		surface := NewSurface(width, 2)
		view := surface.View()
		for i := 1; i < len(data); i += 4 {
			a, b, c, d := fuzzBytes(data[i:])
			draw := view
			if c&8 != 0 {
				left := int(d) % width
				span := 1 + int(a)%(width-left)
				draw = view.Sub(Rect(left, 0, span, 2))
			}
			before := slices.Clone(surface.cells)
			x := int(a)%24 - 4
			y := int(b) % 2
			style := Style{
				FG:   RGBColor(a, b, c),
				BG:   RGBColor(d, c, b),
				Attr: Attr(1 << (d % 6)),
			}
			kind := contentMutation
			switch c % 6 {
			case 0:
				draw.Text(x, y, texts[int(d)%len(texts)], style)
			case 1:
				from := int(d)%24 - 4
				draw.Fill(Rect(from, y, int(a)%8, 1), style)
			case 2:
				kind = appearanceMutation
				draw.MergeStyle(x, y, style)
			case 3:
				kind = appearanceMutation
				draw.Blend(Rect(x, y, int(a)%8, 1), RGBColor(d, a, b), 0.5)
			case 4:
				kind = appearanceMutation
				draw.Fade(Rect(x, y, int(a)%8, 1), 0.5)
			case 5:
				kind = appearanceMutation
				draw.Link(x, y, int(a)%8, "https://example.test")
			}
			assertDisplayAtoms(t, surface)
			assertClipChanges(t, before, surface, draw.clip, kind)
		}
	})
}

// FuzzAppearancePartitionsMatchWhole protects the ownership rule that a display
// atom belongs to the region containing its head. Applying a non-idempotent
// appearance to adjacent regions must therefore equal applying it to their union.
func FuzzAppearancePartitionsMatchWhole(f *testing.F) {
	f.Add(byte(7), byte(3), []byte{0, 0, 3, 0, 0, 0}, false)
	f.Add(byte(15), byte(8), []byte{1, 2, 4, 5, 0, 3}, true)

	clusters := [...]string{"x", "中", "ｶﾞ", "あﾞ", "é", "🙂"}
	base := Style{FG: RGBColor(200, 200, 200), BG: RGBColor(0, 0, 0)}
	f.Fuzz(func(t *testing.T, widthByte, splitByte byte, data []byte, fade bool) {
		width := 1 + int(widthByte)%16
		split := int(splitByte) % (width + 1)
		var source strings.Builder
		for _, choice := range data[:min(len(data), 32)] {
			source.WriteString(clusters[int(choice)%len(clusters)])
		}

		makeSurface := func() *Surface {
			surface := NewSurface(width, 1)
			surface.View().Text(0, 0, source.String(), base)
			return surface
		}
		apply := func(view View) {
			if fade {
				view.Fade(view.Bounds(), 0.5)
				return
			}
			view.Blend(view.Bounds(), RGBColor(0, 0, 0), 0.5)
		}

		whole := makeSurface()
		apply(whole.View())

		partitioned := makeSurface()
		view := partitioned.View()
		apply(view.Sub(Rect(0, 0, split, 1)))
		apply(view.Sub(Rect(split, 0, width-split, 1)))
		if !slices.Equal(partitioned.cells, whole.cells) {
			t.Fatalf("partition at %d produced %+v, whole produced %+v", split, partitioned.cells, whole.cells)
		}
	})
}

type clipMutation uint8

const (
	contentMutation clipMutation = iota
	appearanceMutation
)

func fuzzBytes(data []byte) (a, b, c, d byte) {
	if len(data) > 0 {
		a = data[0]
	}
	if len(data) > 1 {
		b = data[1]
	}
	if len(data) > 2 {
		c = data[2]
	}
	if len(data) > 3 {
		d = data[3]
	}
	return a, b, c, d
}

func assertDisplayAtoms(t *testing.T, surface *Surface) {
	t.Helper()
	for y := range surface.h {
		row := surface.row(y)
		for x, cell := range row {
			switch {
			case cell.span == 0:
			case cell.span >= 2:
				width := int(cell.span)
				if x+width > len(row) {
					t.Fatalf("head (%d,%d) width %d crosses row boundary", x, y, width)
				}
				for offset := 1; offset < width; offset++ {
					continuation := row[x+offset]
					if continuation.span != span(-offset) || continuation.content != "" ||
						continuation.Style != cell.Style || continuation.Link != cell.Link {
						t.Fatalf("head (%d,%d) continuation %d = %+v", x, y, offset, continuation)
					}
				}
			case cell.span < 0:
				offset := -int(cell.span)
				head := x - offset
				if head < 0 || row[head].span < 2 || int(row[head].span) <= offset {
					t.Fatalf("continuation (%d,%d) offset %d has no matching head", x, y, offset)
				}
			default:
				t.Fatalf("cell (%d,%d) has invalid span %d", x, y, cell.span)
			}
		}
	}
}

// assertClipChanges makes View's ownership boundary executable. Outside a clip,
// content operations may only clear an intersected old atom while preserving its
// style; appearance operations may only restyle or relink an atom whose head the
// clip owns.
func assertClipChanges(t *testing.T, before []Cell, surface *Surface, clip image.Rectangle, kind clipMutation) {
	t.Helper()
	for y := range surface.h {
		beforeRow := before[y*surface.w : (y+1)*surface.w]
		afterRow := surface.row(y)
		for x, old := range beforeRow {
			if image.Pt(x, y).In(clip) || afterRow[x] == old {
				continue
			}
			got := afterRow[x]
			head := x
			if old.span < 0 {
				head += int(old.span)
			}
			if head < 0 || head >= len(beforeRow) || beforeRow[head].span < 2 {
				t.Fatalf("write changed non-atom cell (%d,%d) outside clip", x, y)
			}
			width := min(int(beforeRow[head].span), len(beforeRow)-head)
			switch kind {
			case contentMutation:
				intersected := false
				for column := head; column < head+width; column++ {
					if image.Pt(column, y).In(clip) && afterRow[column] != beforeRow[column] {
						intersected = true
						break
					}
				}
				if !intersected {
					t.Fatalf("content write changed atom [%d,%d) outside clip without changing its clipped part", head, head+width)
				}
				if got.content != "" || got.span != 0 || got.Link != "" || got.Style != old.Style {
					t.Fatalf("content write changed (%d,%d) outside clip from %+v to %+v", x, y, old, got)
				}
			case appearanceMutation:
				if !image.Pt(head, y).In(clip) || afterRow[head] == beforeRow[head] {
					t.Fatalf("appearance changed atom [%d,%d) outside a clip that owns its head", head, head+width)
				}
				if got.content != old.content || got.span != old.span {
					t.Fatalf("appearance write changed geometry at (%d,%d) outside clip from %+v to %+v", x, y, old, got)
				}
			}
		}
	}
}
