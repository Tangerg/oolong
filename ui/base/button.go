package base

import (
	"context"
	"strings"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// ButtonConfig supplies one semantic label, a pure availability query and the event-side action. Activate returning Started means work was accepted, not completed.
type ButtonConfig struct {
	Key, Label   string
	Keys         *keymap.Map
	Availability func() scene.Availability
	Activate     func(context.Context) (scene.ActionResult[struct{}], error)
}

// Button connects every activation source to one typed action. It keeps no
// independent enabled flag: appearance and activation query the same policy.
// Button must not be copied after first use; its collaborators share one live identity.
type Button struct {
	noCopy   noCopy
	label    string
	element  scene.Element
	action   *scene.Action[struct{}, struct{}]
	pointer  Pointer
	feedback requestFeedback
	keys     keyBinding
	matcher  keymap.Matcher
}

// NewButton constructs one action owner. Nil Activate creates a disabled button with an explanatory availability reason.
func NewButton(config ButtonConfig) *Button {
	b := &Button{label: strings.Clone(config.Label), keys: keyBinding{current: config.Keys}}
	b.action = scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
		ID: scene.ActionActivate, Label: "Activate",
		Availability: func(struct{}) scene.Availability {
			if config.Activate == nil {
				return scene.Availability{Disabled: true, Reason: "no action"}
			}
			if config.Availability != nil {
				return readProjection(b.element, config.Availability)
			}
			return scene.Availability{}
		},
		Run: func(ctx context.Context, _ struct{}) (scene.ActionResult[struct{}], error) {
			return config.Activate(ctx)
		},
	})
	b.element = scene.NewElement(scene.ElementConfig{
		Key: config.Key, Focusable: func() bool { return true }, Enabled: func() bool { return !b.action.Availability(struct{}{}).Disabled },
		Measure: func(axis layout.Axis, _ int) int {
			if axis == layout.Down {
				return 1
			}
			return text.Width(b.label)
		},
		Paint: b.Draw,
		FocusChanged: func(has bool) {
			if !has {
				b.pointer.Left()
				b.matcher.Clear()
			}
		},
		Handle: b.handleEvent, Semantics: func() scene.Semantic {
			return withFeedback(scene.Semantic{Role: scene.RoleButton, Name: b.label}, b.Error())
		}, Actions: []scene.ActionBinding{b.action.Binding()},
	})
	return b
}

func (b *Button) handleEvent(event scene.Event) scene.EventResult {
	if mouse, ok := event.Input().(input.Mouse); ok {
		return partEvent(event, func(input.Event) bool {
			accepted := b.pointer.Handle(mouse)
			if !accepted || b.pointer.pressedWith() != input.ButtonLeft {
				return false
			}
			if mouse.Action == input.MouseUp && b.pointer.Clicked(input.ButtonLeft) {
				b.Do(Activate)
			}
			return mouse.Action == input.MouseDown || mouse.Action == input.MouseDrag || mouse.Action == input.MouseUp
		}, &b.pointer.geometry)
	}
	key, ok := event.Input().(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	_, handled := b.matcher.Handle(b.keys.resolve(activationKeys), key, keyboardReceipt(&b.element), b.Do)
	return scene.EventResult{Consumed: handled}
}

func (b *Button) Element() scene.Element {
	if b == nil {
		return scene.Element{}
	}
	b.keys.attach(b.element, &b.matcher)
	return b.element
}

func (b *Button) Label() string { return b.label }

// SetLabel replaces the owned name and wakes the scene. It panics during presentation or observation.
func (b *Button) SetLabel(label string) {
	requireMutable(b.element)
	b.label = strings.Clone(label)
	b.element.Invalidate()
}

// Availability reads the same pure policy checked before every activation.
func (b *Button) Availability() scene.Availability { return b.action.Availability(struct{}{}) }

// Press checks the current action policy and invokes its owner. It panics during presentation or observation; action failures are returned unchanged.
func (b *Button) Press(ctx context.Context) (scene.ActionResult[struct{}], error) {
	return b.press(ctx, false)
}

func (b *Button) press(ctx context.Context, recordError bool) (scene.ActionResult[struct{}], error) {
	requireMutable(b.element)
	var turn uint64
	if recordError {
		turn = b.feedback.begin()
	}
	result, err := b.action.Invoke(ctx, struct{}{})
	changed := result.State != scene.Unchanged
	if recordError {
		changed = b.feedback.finish(turn, err) || changed
	}
	if changed {
		b.element.Invalidate()
	}
	return result, err
}

// Error is the last activation error delivered by keyboard or pointer input.
// Programmatic and agent activation return their error directly.
func (b *Button) Error() error { return b.feedback.err }

func (b *Button) HeightForWidth(int) int { return 1 }

func (b *Button) Draw(frame scene.Frame) { b.Stage(frame); frame.Text(0, 0, b.label, Look{}.Text) }

// Stage derives this candidate frame's pointer extent for an alternate appearance.
// It panics unless the button is mounted in the frame's scene outside read-only projection.
func (b *Button) Stage(frame scene.Frame) {
	requireProjection(b.element, frame)
	b.pointer.Stage(frame, frame.Bounds())
}

func (b *Button) Do(action keymap.Action) bool {
	if action != Activate {
		return false
	}
	_, _ = b.press(context.Background(), true)
	return true
}
