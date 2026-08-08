package kit

import (
	"strconv"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Slider is the polished one-row appearance of a [headless.Slider].
//
// Its headless controller owns the value and every interaction. This layer formats
// the label and value, paints the track, and stages exactly the rectangle it painted
// so pointer routing and appearance cannot disagree.
type Slider struct {
	// Theme and Glyphs choose the appearance of the track and thumb.
	Theme  Theme
	Glyphs Glyphs
	// Format turns the integer value into its right-hand label. Nil uses strconv.Itoa;
	// returning an empty string omits the value and leaves that room to the track.
	Format func(int) string

	controller *headless.Slider
}

// NewSlider composes an uncontrolled slider with the kit appearance.
func NewSlider(theme Theme, glyphs Glyphs, label string, minimum, maximum int) *Slider {
	of := headless.NewSlider(minimum, maximum)
	of.SetLabel(label)
	return &Slider{controller: of, Theme: theme, Glyphs: glyphs}
}

// NewControlledSlider composes a slider around caller-owned value storage.
func NewControlledSlider(
	theme Theme,
	glyphs Glyphs,
	binding headless.Accessor[int],
	label string,
	minimum, maximum int,
) *Slider {
	of := headless.NewControlledSlider(binding, minimum, maximum)
	of.SetLabel(label)
	return &Slider{controller: of, Theme: theme, Glyphs: glyphs}
}

// Controller returns the headless slider that owns value and interaction state.
func (s *Slider) Controller() *headless.Slider {
	if s == nil {
		return nil
	}
	return s.controller
}

// Measure is one row whenever a controller is present.
func (s *Slider) Measure(int) int {
	if s == nil || s.controller == nil {
		return 0
	}
	return 1
}

// Draw paints the label, track, thumb, and formatted value.
func (s *Slider) Draw(frame headless.Frame) {
	if s == nil || s.controller == nil {
		return
	}
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		s.controller.Stage(frame, grid.Rect(0, 0, 0, 0))
		return
	}
	value := s.format(s.controller.Value())
	boxes := layoutMeter(width, text.Width(s.controller.Label()), text.Width(value))
	s.controller.Stage(frame, boxes.track)
	if label := s.controller.Label(); label != "" {
		Label{Text: label, Style: s.Theme.Muted, Ellipsis: s.Glyphs.Ellipsis}.
			Draw(frame.Sub(boxes.label).View)
	}
	if value != "" {
		Label{Text: value, Style: s.Theme.Muted, Align: layout.End, Ellipsis: s.Glyphs.Ellipsis}.
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
		glyph, style := s.Glyphs.SliderTrack, s.Theme.Subtle
		if x < position {
			style = s.Theme.Accent
		}
		if x == position {
			glyph, style = s.Glyphs.SliderThumb, s.Theme.Accent
			if s.controller.Focused() {
				style = style.Merge(s.Theme.Selection)
			}
		}
		view.Text(x, 0, glyph, style)
	}
}

// Handle forwards input to the controller.
func (s *Slider) Handle(event input.Event) bool {
	return s != nil && s.controller != nil && s.controller.Handle(event)
}

// Focus forwards keyboard ownership to the controller.
func (s *Slider) Focus(has bool) {
	if s != nil && s.controller != nil {
		s.controller.Focus(has)
	}
}

// Semantics forwards the controller's structural meaning.
func (s *Slider) Semantics() headless.SemanticNode {
	if s == nil || s.controller == nil {
		return headless.SemanticNode{Role: headless.RoleSlider}
	}
	return s.controller.Semantics()
}

func (s *Slider) format(value int) string {
	if s.Format != nil {
		return s.Format(value)
	}
	return strconv.Itoa(value)
}
