package kit

import (
	"strconv"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Slider is the polished one-row appearance of a [base.Slider].
//
// Its base controller owns the value and every interaction. This layer formats
// the label and value, paints the track, and stages exactly the rectangle it painted
// so pointer routing and appearance cannot disagree.
type Slider struct {
	appearance appearance
	Glyphs     Glyphs
	// Format turns the integer value into its right-hand label. Nil uses strconv.Itoa;
	// returning an empty string omits the value and leaves that room to the track. A
	// non-empty value is likewise omitted when it cannot fit in full, because clipped
	// digits can read as another value. Format runs during drawing and must be an
	// observationally pure projection.
	Format func(int) string

	controller *base.Slider
}

// SliderConfig is the complete construction state of [Slider].
//
// Value chooses state ownership without selecting a second constructor. Nil starts a
// locally owned value at Minimum. Format is optional and has the same zero meaning as
// [Slider.Format].
type SliderConfig struct {
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	// Value is optional caller-owned state. Nil starts local state at Minimum.
	Initial int
	// Minimum and Maximum are inclusive and may be equal.
	Minimum, Maximum int
	// Step is the keyboard increment. Zero means one.
	Step  int
	Label string
	// Keys maps slider actions. Nil uses the base defaults.
	Keys *keymap.Map
	// Format renders the value at the right. Nil uses strconv.Itoa. It runs during
	// drawing and must be observationally pure.
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
		controller: base.NewSlider(base.SliderConfig{
			Initial: config.Initial, Minimum: config.Minimum, Maximum: config.Maximum,
			Step: config.Step, Label: config.Label, Keys: config.Keys,
		}),
		appearance: look,
		Glyphs:     config.Glyphs,
		Format:     config.Format,
	}
	if err := slider.controller.Element().SetPaint(slider.Draw); err != nil {
		panic(err)
	}
	slider.appearance.bind(slider.Element())
	return slider
}

// Controller returns the base slider that owns value and interaction state.
func (s *Slider) Controller() *base.Slider {
	if s == nil {
		return nil
	}
	return s.controller
}

// HeightForWidth is one row whenever a controller is present.
func (s *Slider) HeightForWidth(int) int {
	if s == nil || s.controller == nil {
		return 0
	}
	return 1
}

// Draw paints the label, track, thumb, and formatted value.
func (s *Slider) Draw(frame scene.Frame) {
	if s == nil || s.controller == nil {
		return
	}
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		s.controller.Stage(frame, grid.Area(0, 0, 0, 0))
		return
	}
	value := s.format(s.controller.Value())
	boxes := layoutMeter(width, text.Width(s.controller.Label()), text.Width(value))
	s.controller.Stage(frame, boxes.track)
	if label := s.controller.Label(); label != "" {
		Label{Text: label, Style: s.appearance.palette().Muted, Ellipsis: s.Glyphs.Ellipsis}.
			Draw(frame.Sub(boxes.label).View)
	}
	if value != "" {
		Label{Text: value, Style: s.appearance.palette().Muted, Align: layout.End, Ellipsis: s.Glyphs.Ellipsis}.
			Draw(frame.Sub(boxes.value).View)
	}
	s.track(frame.Sub(boxes.track).View)
}

func (s *Slider) track(view grid.View) {
	width, _ := view.Size()
	if width <= 0 || s.Glyphs.SliderTrack == "" || s.Glyphs.SliderThumb == "" {
		return
	}
	position := s.controller.Position(width)
	for x := range width {
		glyph, style := s.Glyphs.SliderTrack, s.appearance.palette().Subtle
		if x < position {
			style = s.appearance.palette().Accent
		}
		if x == position {
			glyph, style = s.Glyphs.SliderThumb, s.appearance.palette().Accent
			if s.controller.Focused() {
				style = style.Merge(s.appearance.palette().Selection)
			}
		}
		view.Text(x, 0, glyph, style)
	}
}

// Handle forwards input to the controller.

// Focus forwards keyboard ownership to the controller.

// Semantics forwards the controller's structural meaning.

func (s *Slider) format(value int) string {
	if s.Format != nil {
		return s.Format(value)
	}
	return strconv.Itoa(value)
}

// Element returns the controller's one scene identity with its kit appearance installed.
func (s *Slider) Element() scene.Element {
	if s == nil || s.controller == nil {
		return scene.Element{}
	}
	return s.controller.Element()
}
