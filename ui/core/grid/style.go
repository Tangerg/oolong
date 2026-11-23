package grid

type Attr uint8

const (
	Bold Attr = 1 << iota
	Dim
	Italic
	Underline
	Reverse
	Strike
)

func (a Attr) Has(want Attr) bool { return a&want == want }

// Style is how a cell looks. The zero Style is the terminal's own appearance.
type Style struct {
	FG, BG Color
	Attr   Attr
}

// Merge lays over on top of s: whatever over states wins, whatever it leaves at
// its default is inherited. Attributes accumulate, because an overlay that adds
// emphasis should not silently drop the emphasis underneath it.
func (s Style) Merge(over Style) Style {
	out := s
	if !over.FG.Default() {
		out.FG = over.FG
	}
	if !over.BG.Default() {
		out.BG = over.BG
	}
	out.Attr |= over.Attr
	return out
}

// Ground holds queried terminal colors so default cells can follow terminal
// recoloring while blending can resolve actual RGB values. The zero value leaves
// unknown colors unresolved; blending cannot guess a missing background.
type Ground struct{ FG, BG Color }

// Light reports a background the terminal described as light. A background it never
// described counts as dark, which is what nearly every terminal ships with, so content
// chosen for a dark ground is the safer guess when nothing was said.
func (g Ground) Light() bool { return !g.BG.Default() && !g.BG.RGB().Dark() }

// Resolve fills in whatever a style left to the terminal, so a caller that needs
// numbers has them. What the terminal never said stays default.
//
// [Reverse] is deliberately not applied. It swaps the two colours on the way to the
// screen, and swapping them here would mean a caller that resolved a style and drew
// it back would reverse it twice.
func (g Ground) Resolve(s Style) Style {
	if s.FG.Default() {
		s.FG = g.FG
	}
	if s.BG.Default() {
		s.BG = g.BG
	}
	return s
}
