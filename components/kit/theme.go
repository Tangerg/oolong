package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
)

// Theme is the palette an interface is drawn with.
//
// The fields are styles rather than colours because a role often carries more than
// a colour: muted text is dim as well as grey, and a heading is bold. A widget that
// had to remember to add the dimming would be a widget that sometimes forgot.
type Theme struct {
	// Text is body text, and Muted is text present for reference rather than for
	// reading — timestamps, paths, counts.
	Text  grid.Style
	Muted grid.Style
	// Subtle is quieter than muted: structure the eye should skip unless it is
	// looking for it.
	Subtle grid.Style
	// Strong is emphasis within body text.
	Strong grid.Style
	// Heading titles a pane or a section.
	Heading grid.Style

	// Accent marks the thing the interface is about: the active item, the key in a
	// hint, the prompt marker.
	Accent grid.Style
	// Success, Warning, Danger and Info are outcomes. They are the only colours in
	// the interface that carry meaning on their own, which is why there are exactly
	// four of them.
	Success grid.Style
	Warning grid.Style
	Danger  grid.Style
	Info    grid.Style

	// Border draws a frame, and Divider a line between things inside one.
	Border  grid.Style
	Divider grid.Style
	// Selection is the row under the cursor.
	Selection grid.Style
	// Surface is a pane's background, and Sunken a well inside one — a tool's output,
	// a code block.
	Surface grid.Style
	Sunken  grid.Style

	// Added and Removed are the two halves of a diff. Nothing else in the interface
	// uses green and red on a background, so a diff is recognisable at a glance.
	Added   grid.Style
	Removed grid.Style
	// Context is a diff line that did not change.
	Context grid.Style

	// Scrim is what a layer paints over what it covers, so the eye goes to the layer
	// and it is obvious that what is behind it is not the thing to act on.
	//
	// It is the one entry that is not a style, because receding is not a colour: it
	// is mixing with whatever is already there. A style could only have set one
	// colour over everything, which erases the interface underneath instead of
	// dimming it.
	Scrim Scrim
}

// Scrim is a translucent sheet of colour, painted over a region to make it recede.
//
// The zero value covers nothing, so a widget that was given no theme dims nothing —
// which is the right answer, because dimming toward a colour nobody chose is a
// guess about what the interface looks like.
type Scrim struct {
	// Color is what the sheet is made of, and Opacity how much of it shows, from 0
	// for nothing to 1 for paint.
	Color   grid.Color
	Opacity float64
}

// Over paints the scrim across everything the view covers.
//
// Where a cell's colour is the terminal's own it is resolved first, which needs the
// terminal to have said what that is — see [grid.View.Blend]. Nothing else here has
// to know that, which is why this is one line and lives at the bottom of the layer
// rather than in every widget that floats something.
func (s Scrim) Over(v grid.View) { v.Blend(v.Bounds(), s.Color, s.Opacity) }

// Look is what a headless widget that draws itself is dressed with — see
// [headless.Look] — built from this palette and a glyph set.
//
// It is here rather than in each widget that needs one because it is one mapping:
// the answer is a heading, a label is strong, a hint is subtle, the row under the
// keyboard is the selection. A second copy of it somewhere else would be a second
// place for a form and the editor inside it to disagree about what a placeholder
// looks like.
func (t Theme) Look(g Glyphs) headless.Look {
	return headless.Look{
		Text:      t.Text,
		Label:     t.Strong,
		Subtle:    t.Subtle,
		Selection: t.Selection,
		Accent:    t.Accent,
		Danger:    t.Danger,
		Taken:     g.Taken,
		Free:      g.Free,
	}
}

// Suited chooses a theme from the surrounding terminal colours.
//
// Only the background decides, because that is what everything is read against. A
// terminal that said nothing gets the dark theme, because dark is the commoner
// choice and light is the one that becomes unreadable when it is guessed wrong:
// grey on white is faint, and grey on black is invisible.
func Suited(g grid.Ground) Theme {
	if !g.BG.Default() && !g.BG.RGB().Dark() {
		return Light()
	}
	return Dark()
}

// themePalette is the raw colour vocabulary shared by the built-in themes. Turning
// it into semantic styles is one mapping: dark and light differ in colours, not in
// what a border, selection or outcome means.
type themePalette struct {
	text, muted, subtle, accent grid.Color
	green, amber, red, cyan     grid.Color
	line, surface, sunken       grid.Color
	selected, addedBG, goneBG   grid.Color
	scrim                       Scrim
}

func (p themePalette) theme() Theme {
	return Theme{
		Text:      grid.Style{FG: p.text},
		Muted:     grid.Style{FG: p.muted},
		Subtle:    grid.Style{FG: p.subtle},
		Strong:    grid.Style{FG: p.text, Attr: grid.Bold},
		Heading:   grid.Style{FG: p.text, Attr: grid.Bold},
		Accent:    grid.Style{FG: p.accent},
		Success:   grid.Style{FG: p.green},
		Warning:   grid.Style{FG: p.amber},
		Danger:    grid.Style{FG: p.red},
		Info:      grid.Style{FG: p.cyan},
		Border:    grid.Style{FG: p.line},
		Divider:   grid.Style{FG: p.line},
		Selection: grid.Style{BG: p.selected},
		Surface:   grid.Style{BG: p.surface},
		Sunken:    grid.Style{BG: p.sunken},
		Added:     grid.Style{FG: p.green, BG: p.addedBG},
		Removed:   grid.Style{FG: p.red, BG: p.goneBG},
		Context:   grid.Style{FG: p.muted},
		Scrim:     p.scrim,
	}
}

// Dark is the default theme: neutral charcoal surfaces with bright accents used
// sparingly. Keeping the foundation achromatic lets status colours carry meaning
// without tinting every piece of surrounding chrome.
//
//nolint:dupl // Dark and Light are parallel palette data; extracting field assignment would hide the complete palette each function audits.
func Dark() Theme {
	return themePalette{
		text:     grid.RGBColor(0xE1, 0xE1, 0xE1),
		muted:    grid.RGBColor(0x6C, 0x6C, 0x6C),
		subtle:   grid.RGBColor(0x58, 0x58, 0x58),
		accent:   grid.RGBColor(0x7A, 0xA2, 0xF7),
		green:    grid.RGBColor(0x9E, 0xCE, 0x6A),
		amber:    grid.RGBColor(0xE0, 0xAF, 0x68),
		red:      grid.RGBColor(0xF7, 0x76, 0x8E),
		cyan:     grid.RGBColor(0x7D, 0xCF, 0xFF),
		line:     grid.RGBColor(0x32, 0x32, 0x37),
		surface:  grid.RGBColor(0x14, 0x14, 0x14),
		sunken:   grid.RGBColor(0x1C, 0x1C, 0x1C),
		selected: grid.RGBColor(0x36, 0x36, 0x36),
		addedBG:  grid.RGBColor(0x06, 0x38, 0x06),
		goneBG:   grid.RGBColor(0x42, 0x0E, 0x14),
		scrim:    Scrim{Color: grid.RGBColor(0, 0, 0), Opacity: 0.5},
	}.theme()
}

// Light turns the same hierarchy over for a light background. Its accents are
// deepened rather than merely reused so their contrast and relative emphasis stay
// aligned with the dark theme.
//
//nolint:dupl // Dark and Light are parallel palette data; extracting field assignment would hide the complete palette each function audits.
func Light() Theme {
	return themePalette{
		text:     grid.RGBColor(0x26, 0x26, 0x26),
		muted:    grid.RGBColor(0x76, 0x76, 0x76),
		subtle:   grid.RGBColor(0xA5, 0xA5, 0xA5),
		accent:   grid.RGBColor(0x2F, 0x64, 0xD2),
		green:    grid.RGBColor(0x37, 0x8E, 0x23),
		amber:    grid.RGBColor(0xA2, 0x76, 0x12),
		red:      grid.RGBColor(0xCD, 0x30, 0x48),
		cyan:     grid.RGBColor(0x00, 0x82, 0xAA),
		line:     grid.RGBColor(0xC8, 0xC8, 0xCD),
		surface:  grid.RGBColor(0xEE, 0xEE, 0xEE),
		sunken:   grid.RGBColor(0xE4, 0xE4, 0xE4),
		selected: grid.RGBColor(0xC6, 0xC6, 0xC6),
		addedBG:  grid.RGBColor(0xDA, 0xF2, 0xDC),
		goneBG:   grid.RGBColor(0xF5, 0xDA, 0xDE),
		scrim:    Scrim{Color: grid.RGBColor(0, 0, 0), Opacity: 0.5},
	}.theme()
}
