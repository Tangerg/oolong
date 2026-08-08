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

// Dark is the default theme: a cool slate, the same family the desktop interface
// uses, so the two do not look like different products.
//
// The greys are cool on purpose. A neutral grey beside the blue accent reads as
// slightly yellow, and the whole interface looks dusty.
func Dark() Theme {
	return themePalette{
		text:     grid.RGBColor(0xE2, 0xE6, 0xEF),
		muted:    grid.RGBColor(0x94, 0x9C, 0xB0),
		subtle:   grid.RGBColor(0x64, 0x6C, 0x80),
		accent:   grid.RGBColor(0x7A, 0xA2, 0xF7),
		green:    grid.RGBColor(0x7A, 0xC8, 0x8E),
		amber:    grid.RGBColor(0xD7, 0xA6, 0x5C),
		red:      grid.RGBColor(0xE8, 0x7D, 0x7D),
		cyan:     grid.RGBColor(0x6C, 0xB6, 0xC4),
		line:     grid.RGBColor(0x3A, 0x41, 0x52),
		surface:  grid.RGBColor(0x16, 0x19, 0x22),
		sunken:   grid.RGBColor(0x1D, 0x21, 0x2C),
		selected: grid.RGBColor(0x25, 0x2B, 0x3A),
		addedBG:  grid.RGBColor(0x18, 0x2C, 0x21),
		goneBG:   grid.RGBColor(0x2E, 0x1C, 0x1F),
		// Body text mixed this far toward black lands almost exactly on Subtle, which
		// is what a covered interface should read as: still legible, plainly not the
		// thing being asked about.
		scrim: Scrim{Color: grid.RGBColor(0, 0, 0), Opacity: 0.55},
	}.theme()
}

// Light is the same palette turned over, for a terminal on a light background.
func Light() Theme {
	return themePalette{
		text:     grid.RGBColor(0x1C, 0x21, 0x2C),
		muted:    grid.RGBColor(0x5C, 0x65, 0x78),
		subtle:   grid.RGBColor(0x8B, 0x93, 0xA5),
		accent:   grid.RGBColor(0x2E, 0x5C, 0xC8),
		green:    grid.RGBColor(0x1F, 0x7A, 0x45),
		amber:    grid.RGBColor(0x92, 0x5F, 0x0E),
		red:      grid.RGBColor(0xB4, 0x2D, 0x2D),
		cyan:     grid.RGBColor(0x1B, 0x6A, 0x78),
		line:     grid.RGBColor(0xD2, 0xD7, 0xE0),
		surface:  grid.RGBColor(0xFA, 0xFB, 0xFD),
		sunken:   grid.RGBColor(0xF0, 0xF2, 0xF6),
		selected: grid.RGBColor(0xE4, 0xE8, 0xF0),
		addedBG:  grid.RGBColor(0xE7, 0xF6, 0xEC),
		goneBG:   grid.RGBColor(0xFB, 0xEA, 0xEA),
		// Less of it than the dark theme takes. A light interface is nearly all
		// background, so the same sheet that dims a dark one turns this one into a
		// grey panel and loses the sense that something is behind the layer.
		scrim: Scrim{Color: grid.RGBColor(0, 0, 0), Opacity: 0.4},
	}.theme()
}
