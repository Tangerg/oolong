package headless

import (
	"image"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Slider owns one integer constrained to a range and the interactions that change it.
//
// It is behavior rather than appearance. An appearance calls [Slider.Stage] with the
// track it drew, reads [Slider.Position], and chooses its own glyphs and styles. Keys,
// pointer dragging, controlled state, bounds, and semantics remain one state machine
// regardless of how the track looks.
//
// Construct a slider with [NewSlider]. Its zero value is the inert range [0, 0] with
// a step of one.
type Slider struct {
	minimum, maximum int
	step             int
	value            ownedValue[int]
	label            string

	// Keys maps slider actions. Nil reads through [DefaultSliderKeys].
	Keys *keymap.Map

	focused  bool
	dragging bool
	matcher  keymap.Matcher
	track    Snapshot[image.Rectangle]
}

// SliderConfig is the complete construction state of [Slider].
//
// A nil Value gives the controller local ownership starting at Minimum. An accessor
// gives ownership to the caller and is clamped during construction. Step zero means
// one, matching the useful zero Slider.
type SliderConfig struct {
	// Value is optional caller-owned state. Nil starts local state at Minimum.
	Value Accessor[int]
	// Minimum and Maximum are inclusive and may be equal.
	Minimum, Maximum int
	// Step is the keyboard increment. Zero means one.
	Step int
	// Label names the value in semantics and any appearance.
	Label string
	// Keys maps slider actions. Nil uses [DefaultSliderKeys].
	Keys *keymap.Map
}

// NewSlider constructs one slider from config.
//
// With Value set, later owner-written values are applied with [Slider.Sync], matching
// the explicit controlled-state rule used by dialogs and tabs.
func NewSlider(config SliderConfig) *Slider {
	s := &Slider{value: newOwnedValue(config.Minimum, config.Value), Keys: config.Keys}
	s.SetBounds(config.Minimum, config.Maximum)
	if config.Step != 0 {
		s.SetStep(config.Step)
	}
	s.SetLabel(config.Label)
	return s
}

// Bounds returns the inclusive minimum and maximum.
func (s *Slider) Bounds() (minimum, maximum int) {
	if s == nil {
		return 0, 0
	}
	return s.minimum, s.maximum
}

// SetBounds changes the inclusive range and clamps the current value.
//
// A reversed range or one whose span cannot be represented by int is a programmer
// error. The latter cannot be mapped onto a finite terminal extent without giving the
// same position two incompatible integer meanings.
func (s *Slider) SetBounds(minimum, maximum int) {
	if s == nil {
		return
	}
	if maximum < minimum || maximum-minimum < 0 {
		panic("headless: slider bounds have an invalid span")
	}
	s.minimum, s.maximum = minimum, maximum
	s.Sync()
}

// Step returns how much one increase or decrease changes the value. The zero Slider
// uses one.
func (s *Slider) Step() int {
	if s == nil || s.step <= 0 {
		return 1
	}
	return s.step
}

// SetStep changes the keyboard increment. A non-positive step is a programmer error.
func (s *Slider) SetStep(step int) {
	if step <= 0 {
		panic("headless: slider step must be positive")
	}
	if s != nil {
		s.step = step
	}
}

// Value returns the current value, clamped to the slider's bounds.
func (s *Slider) Value() int {
	if s == nil {
		return 0
	}
	return min(max(s.value.get(), s.minimum), s.maximum)
}

// Set changes the value, clamped to the slider's bounds, and reports whether the
// stored value changed.
func (s *Slider) Set(value int) bool {
	if s == nil {
		return false
	}
	value = min(max(value, s.minimum), s.maximum)
	if s.value.get() == value {
		return false
	}
	s.value.set(value)
	return true
}

// Sync clamps a caller-written controlled value. It is harmless for an uncontrolled
// slider.
func (s *Slider) Sync() bool {
	if s == nil {
		return false
	}
	return s.Set(s.value.get())
}

// Move changes the value by a number of steps, saturating at either bound.
func (s *Slider) Move(steps int) bool {
	if s == nil || steps == 0 {
		return false
	}
	value, step := s.Value(), s.Step()
	if steps > 0 {
		available := (s.maximum - value) / step
		if steps > available {
			return s.Set(s.maximum)
		}
	} else {
		available := (value - s.minimum) / step
		if steps < -available {
			return s.Set(s.minimum)
		}
	}
	return s.Set(value + steps*step)
}

// Label returns the control's semantic label.
func (s *Slider) Label() string {
	if s == nil {
		return ""
	}
	return s.label
}

// SetLabel changes the control's semantic label.
func (s *Slider) SetLabel(label string) {
	if s != nil {
		s.label = strings.Clone(label)
	}
}

// Position maps the current value onto cells positions from zero through cells-1.
func (s *Slider) Position(cells int) int {
	if s == nil || cells <= 1 || s.maximum == s.minimum {
		return 0
	}
	return layout.Scale(cells-1, s.Value()-s.minimum, s.maximum-s.minimum)
}

// Stage publishes the track rectangle an appearance drew, in the slider widget's
// local coordinates. Pointer events are routed against this rectangle only after the
// complete root frame commits.
func (s *Slider) Stage(frame Frame, track image.Rectangle) {
	if s == nil {
		return
	}
	s.track.Stage(frame, track.Intersect(frame.Bounds()))
}

// Handle applies bound keys and a left-button drag to the value.
func (s *Slider) Handle(event input.Event) bool {
	if s == nil {
		return false
	}
	if mouse, ok := event.(input.Mouse); ok {
		return s.mouse(mouse)
	}
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	_, handled := s.matcher.Handle(s.keys(), key, s.Do)
	return handled
}

// Do applies a slider action by name.
func (s *Slider) Do(action keymap.Action) bool {
	switch action {
	case Decrease:
		s.Move(-1)
	case Increase:
		s.Move(1)
	case ToMinimum:
		s.Set(s.minimum)
	case ToMaximum:
		s.Set(s.maximum)
	default:
		return false
	}
	return true
}

func (s *Slider) mouse(mouse input.Mouse) bool {
	track := s.track.Value()
	switch mouse.Action {
	case input.MouseDown:
		if mouse.Button != input.ButtonLeft || track.Empty() || !mouse.Pos.In(track) {
			return false
		}
		s.dragging = true
		s.setAt(mouse.Pos.X, track)
		return true
	case input.MouseDrag:
		if !s.dragging {
			return false
		}
		s.setAt(mouse.Pos.X, track)
		return true
	case input.MouseUp:
		if !s.dragging {
			return false
		}
		s.dragging = false
		s.setAt(mouse.Pos.X, track)
		return true
	default:
		return false
	}
}

func (s *Slider) setAt(x int, track image.Rectangle) {
	positions := track.Dx()
	if positions <= 1 || s.maximum == s.minimum {
		s.Set(s.minimum)
		return
	}
	at := min(max(x-track.Min.X, 0), positions-1)
	s.Set(s.minimum + layout.Scale(s.maximum-s.minimum, at, positions-1))
}

// Focus takes or gives up keyboard ownership.
func (s *Slider) Focus(has bool) {
	if s != nil {
		if !has {
			s.matcher.Clear()
		}
		s.focused = has
	}
}

// Focused reports whether the slider owns the keyboard.
func (s *Slider) Focused() bool { return s != nil && s.focused }

// Semantics describes the slider independently of its track appearance.
func (s *Slider) Semantics() SemanticNode {
	if s == nil {
		return SemanticNode{Role: RoleSlider}
	}
	state := SemanticState(0)
	if s.focused {
		state |= StateFocused
	}
	return SemanticNode{
		Role: RoleSlider, Label: s.label, Value: strconv.Itoa(s.Value()), State: state,
	}
}

func (s *Slider) keys() *keymap.Map {
	if s.Keys != nil {
		return s.Keys
	}
	return sliderKeys()
}
