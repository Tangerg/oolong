package kit

import "strings"

// Glyphs are the characters a look draws its furniture with.
//
// # Why they are gathered
//
// Not every terminal can draw them. A terminal running in a locale that is not UTF-8
// shows a box-drawing character as a question mark or as two bytes of mojibake, and a
// panel drawn in mojibake is worse than a panel drawn in dashes. There is no way to
// ask a terminal about this, so the answer is the locale — which is a fact about the
// environment, and therefore an argument rather than something read here.
//
// Gathering them is what makes one answer possible. Scattered through the widgets,
// each would need its own fallback and the set would be inconsistent the first time
// somebody added a glyph and forgot one.
//
// The zero value draws nothing, which is not useful: [Unicode] and [ASCII] are the two
// sets, and [GlyphsFor] picks between them.
type Glyphs struct {
	// The lines a box is drawn with, rounded and square.
	Horizontal, Vertical                       string
	TopLeft, TopRight, BottomLeft, BottomRight string
	RoundTopLeft, RoundTopRight                string
	RoundBottomLeft, RoundBottomRight          string

	// Ellipsis stands for text that did not fit.
	Ellipsis string
	// Bullet marks an item in a list, and Marker the one under the cursor.
	Bullet, Marker string
	// Taken and Free are the two states of a choice: one that has been made, and one
	// that is still on offer. They are a pair and are drawn in the same column, so
	// they have to be the same width or the labels beside them do not line up.
	Taken, Free string
	// ScrollTrack and ScrollThumb are the two halves of a scrollbar.
	ScrollTrack, ScrollThumb string
	// BarFull and BarEmpty are the two halves of a progress bar, and BarSteps are the
	// pieces of a cell it is part way through — from the narrowest to the widest, in
	// order, with as many of them as the set has. A set with none draws the last cell
	// whole or not at all, which is what an eight-step bar degrades to and not a
	// separate design.
	BarFull, BarEmpty string
	BarSteps          []string
	// Expanded and Collapsed are the two states of a branch in a tree: one showing
	// what is under it, and one inviting the reader to look. Like Taken and Free they
	// are drawn in the same column and have to be the same width.
	Expanded, Collapsed string
	// Ascending and Descending mark the column a table is sorted by.
	Ascending, Descending string
	// Spinner is the frames of a busy indicator, in order.
	Spinner []string
}

// Unicode is the set for a terminal that can draw them, which is nearly all of them.
func Unicode() Glyphs {
	return Glyphs{
		Horizontal: "─", Vertical: "│",
		TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘",
		RoundTopLeft: "╭", RoundTopRight: "╮",
		RoundBottomLeft: "╰", RoundBottomRight: "╯",
		Ellipsis:    "…",
		Bullet:      "•",
		Marker:      "▸",
		Taken:       "●",
		Free:        "○",
		ScrollTrack: "│", ScrollThumb: "█",
		BarFull: "█", BarEmpty: "░",
		BarSteps:  []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"},
		Expanded:  "▾",
		Collapsed: "▸",
		Ascending: "↑", Descending: "↓",
		Spinner: []string{"·", "•", "●", "•"},
	}
}

// ASCII is the set for a terminal that cannot, and for output that is going somewhere
// other than a terminal at all.
//
// It is not a transliteration of the other set. A rounded corner has no ASCII
// equivalent and is drawn square; an ellipsis is three stops, which is wider, and the
// widgets that use it measure rather than assume. The point is that it is readable,
// not that it looks the same.
func ASCII() Glyphs {
	return Glyphs{
		Horizontal: "-", Vertical: "|",
		TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
		RoundTopLeft: "+", RoundTopRight: "+",
		RoundBottomLeft: "+", RoundBottomRight: "+",
		Ellipsis:    "...",
		Bullet:      "*",
		Marker:      ">",
		Taken:       "x",
		Free:        "-",
		ScrollTrack: "|", ScrollThumb: "#",
		BarFull: "#", BarEmpty: "-",
		Expanded: "-", Collapsed: "+",
		Ascending: "^", Descending: "v",
		Spinner: []string{".", "o", "O", "o"},
	}
}

// GlyphsFor picks the set a terminal in this environment can draw.
//
// The test is the locale, because there is no other. A terminal cannot be asked
// whether it will render a box-drawing character, and the one thing that reliably
// decides it is whether the environment says UTF-8: outside it, a multi-byte glyph
// arrives as bytes the terminal draws one at a time.
//
// The lookup is passed in rather than read, for the same reason it is everywhere else
// here — this package is not one that touches the operating system, and a test that
// could not say what the locale was could not check either answer.
func GlyphsFor(getenv func(string) string) Glyphs {
	if getenv == nil {
		return Unicode()
	}
	// The first of these that is set decides, which is the order the C library reads
	// them in: LC_ALL overrides everything, LC_CTYPE decides character handling, and
	// LANG is the fallback for both.
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value := getenv(name); value != "" {
			if utf8Locale(value) {
				return Unicode()
			}
			return ASCII()
		}
	}
	// Nothing said. Modern terminals are UTF-8 whether or not anybody set a variable,
	// and assuming otherwise would give dashes to the overwhelming majority to spare
	// the few — the same bet, and for the same reason, as an unrecognised TERM being
	// treated as truecolor.
	return Unicode()
}

// utf8Locale reports whether a locale names the UTF-8 character set.
func utf8Locale(value string) bool {
	_, charset, found := strings.Cut(value, ".")
	if !found {
		// "C" and "POSIX" name no character set and mean ASCII. Anything else without
		// one is a language with no encoding given, which is not a promise of UTF-8.
		return false
	}
	charset = strings.ToLower(strings.ReplaceAll(charset, "-", ""))
	return strings.HasPrefix(charset, "utf8")
}

// Border is the set of characters a box is drawn with. The zero Border draws no
// lines, which is what a box that only pads its content wants.
type Border struct {
	Top, Bottom, Left, Right string
	TopLeft, TopRight        string
	BottomLeft, BottomRight  string
}

// Rounded and Square are the two line styles worth having, drawn with these glyphs.
// Rounded reads as a panel and Square as a table; anything heavier competes with the
// content.
func (g Glyphs) Rounded() Border {
	return Border{
		Top: g.Horizontal, Bottom: g.Horizontal, Left: g.Vertical, Right: g.Vertical,
		TopLeft: g.RoundTopLeft, TopRight: g.RoundTopRight,
		BottomLeft: g.RoundBottomLeft, BottomRight: g.RoundBottomRight,
	}
}

// Square is the other, for a table or anything that should not read as a floating
// panel.
func (g Glyphs) Square() Border {
	return Border{
		Top: g.Horizontal, Bottom: g.Horizontal, Left: g.Vertical, Right: g.Vertical,
		TopLeft: g.TopLeft, TopRight: g.TopRight,
		BottomLeft: g.BottomLeft, BottomRight: g.BottomRight,
	}
}

// drawn reports whether the border draws anything.
func (b Border) drawn() bool { return b != Border{} }

// There is no package-level rounded or square border, on purpose. One would be built
// from the glyphs this machine happens to be able to draw, and would be the wrong
// answer everywhere else — see [GlyphsFor]. A border comes from the set a widget was
// given, which is why a widget that draws one takes a set.
