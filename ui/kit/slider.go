package kit

import (
	"strconv"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Slider is the polished one-row appearance of a [base.Slider].
//
// Its base controller owns the value and every interaction. This layer formats
// the label and value, paints the track, and stages exactly the rectangle it painted
// so pointer routing and appearance cannot disagree.
// A Slider must not be copied after first use: its controller retains this appearance.
type Slider struct {
	noCopy     noCopy
	appearance appearance
	glyphs     Glyphs
	// Format turns the integer value into its right-hand label. Nil uses strconv.Itoa;
	// returning an empty string omits the value and leaves that room to the track. A
	// non-empty value is likewise omitted when it cannot fit in full, because clipped
	// digits can read as another value. Format runs during drawing and must be an
	// observationally pure projection.
	format func(int) string

	controller *base.Slider
}

type SliderConfig struct {
	Behavior   base.SliderConfig
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	// Format runs during drawing and must be an observationally pure projection.
	// Nil uses strconv.Itoa; an empty result reserves no room for a value label.
	Format func(int) string
}

// NewSlider constructs the kit appearance and its sole base controller.
// It panics when Theme and Scope both declare palette sources, or when its base configuration is invalid.
func NewSlider(config SliderConfig) *Slider {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	slider := &Slider{
		controller: base.NewSlider(config.Behavior),
		appearance: look,
		glyphs:     config.Glyphs.clone(),
		format:     config.Format,
	}
	if err := slider.controller.Element().SetPaint(slider.Draw); err != nil {
		panic(err)
	}
	slider.appearance.bind(slider.Element())
	return slider
}

func (s *Slider) Controller() *base.Slider {
	if s == nil {
		return nil
	}
	return s.controller
}

func (s *Slider) HeightForWidth(int) int {
	if s == nil || s.controller == nil {
		return 0
	}
	return 1
}

func (s *Slider) Draw(frame scene.Frame) {
	if s == nil || s.controller == nil {
		return
	}
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		s.controller.Stage(frame, grid.Area(0, 0, 0, 0))
		return
	}
	value := s.formatValue(s.controller.Value())
	boxes := layoutMeter(width, text.Width(s.controller.Label()), text.Width(value))
	s.controller.Stage(frame, boxes.track)
	if label := s.controller.Label(); label != "" {
		Label{Text: label, Style: s.appearance.palette().Muted, Ellipsis: s.glyphs.Ellipsis}.
			Draw(frame.Sub(boxes.label).View)
	}
	if value != "" {
		Label{Text: value, Style: s.appearance.palette().Muted, Align: layout.End, Ellipsis: s.glyphs.Ellipsis}.
			Draw(frame.Sub(boxes.value).View)
	}
	s.track(frame.Sub(boxes.track).View)
}

func (s *Slider) track(view grid.View) {
	width, _ := view.Size()
	if width <= 0 {
		return
	}
	position := s.controller.Position(width)
	visible := view.Visible()
	if visible.Min.Y != 0 || visible.Empty() {
		return
	}
	palette := s.appearance.palette()
	for x := visible.Min.X; x < visible.Max.X; x++ {
		glyph, style := s.glyphs.SliderTrack, palette.Subtle
		if x < position {
			style = palette.Accent
		}
		if x == position {
			glyph, style = s.glyphs.SliderThumb, palette.Accent
			if s.controller.Focused() {
				style = style.Merge(palette.Selection)
			}
		}
		view.Text(x, 0, glyph.value, style)
	}
}

func (s *Slider) formatValue(value int) string {
	if s.format != nil {
		return s.format(value)
	}
	return strconv.Itoa(value)
}

func (s *Slider) Element() scene.Element {
	if s == nil || s.controller == nil {
		return scene.Element{}
	}
	return s.controller.Element()
}

func (s *Slider) Glyphs() Glyphs { return s.glyphs.clone() }

// SetGlyphs panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (s *Slider) SetGlyphs(value Glyphs) {
	element := s.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	s.glyphs = value.clone()
	element.Invalidate()
}

func (s *Slider) Format() func(int) string { return s.format }

// SetFormat panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (s *Slider) SetFormat(value func(int) string) {
	element := s.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if s.format == nil && value == nil {
		return
	}
	s.format = value
	element.Invalidate()
}
