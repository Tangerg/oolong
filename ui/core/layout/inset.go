package layout

import "image"

type Inset struct{ Top, Right, Bottom, Left int }

func Uniform(n int) Inset {
	n = max(n, 0)
	return Inset{Top: n, Right: n, Bottom: n, Left: n}
}

// Symmetric is one inset above and below, and another to the left and right. The
// two axes are separate because equal coordinate extents need not occupy equal
// physical space; naming both pairs also avoids repeating four values at every
// call site.
func Symmetric(vertical, horizontal int) Inset {
	vertical, horizontal = max(vertical, 0), max(horizontal, 0)
	return Inset{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

func (i Inset) Size() image.Point {
	i = i.normalized()
	return image.Pt(Sum(i.Left, i.Right), Sum(i.Top, i.Bottom))
}

// Apply is what is left of r after the inset is held clear, and nothing at all when
// the inset is larger than the region.
//
// The rectangle is built by hand rather than with [image.Rect], which puts a
// backwards rectangle the right way round: an inset that overran its region would
// come back as a real region somewhere else instead of as no region at all.
func (i Inset) Apply(r image.Rectangle) image.Rectangle {
	i = i.normalized()
	if r.Empty() || uint(i.Left)+uint(i.Right) >= uint(r.Max.X)-uint(r.Min.X) ||
		uint(i.Top)+uint(i.Bottom) >= uint(r.Max.Y)-uint(r.Min.Y) {
		return image.Rectangle{}
	}
	out := image.Rectangle{
		Min: image.Pt(r.Min.X+i.Left, r.Min.Y+i.Top),
		Max: image.Pt(r.Max.X-i.Right, r.Max.Y-i.Bottom),
	}
	return out
}

func (i Inset) normalized() Inset {
	i.Top = max(i.Top, 0)
	i.Right = max(i.Right, 0)
	i.Bottom = max(i.Bottom, 0)
	i.Left = max(i.Left, 0)
	return i
}
