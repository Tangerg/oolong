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
	noCopy    noCopy
	label     string
	element   scene.Element
	action    *scene.Action[struct{}, struct{}]
	pointer   Pointer
	lastError error
	keys      *keymap.Map
	matcher   keymap.Matcher
}

// NewButton constructs one action owner. Nil Activate creates a disabled button with an explanatory availability reason.
func NewButton(config ButtonConfig) *Button {
	b := &Button{label: strings.Clone(config.Label), keys: config.Keys}
	b.action = scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
		ID: scene.ActionActivate, Label: "Activate",
		Availability: func(struct{}) scene.Availability {
			if config.Activate == nil {
				return scene.Availability{Disabled: true, Reason: "no action"}
			}
			if config.Availability != nil {
				return config.Availability()
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
				b.matcher.Clear()
				b.pointer.Left()
			}
		},
		Handle: b.handleEvent, Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleButton, Name: b.label} }, Actions: []scene.ActionBinding{b.action.Binding()},
	})
	return b
}

func (b *Button) handleEvent(event scene.Event) scene.EventResult {
	if mouse, ok := event.Input.(input.Mouse); ok {
		return partEvent(event, func(input.Event) bool {
			accepted := b.pointer.Handle(mouse)
			if !accepted || b.pointer.button != input.ButtonLeft {
				return false
			}
			if mouse.Action == input.MouseUp && b.pointer.Clicked(input.ButtonLeft) {
				_, b.lastError = b.Press(context.Background())
			}
			return mouse.Action == input.MouseDown || mouse.Action == input.MouseDrag || mouse.Action == input.MouseUp
		}, &b.pointer.geometry)
	}
	key, ok := event.Input.(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	bindings := b.keys
	if bindings == nil {
		bindings = activationKeys()
	}
	_, handled := b.matcher.Handle(bindings, key, b.Do)
	return scene.EventResult{Consumed: handled}
}

// Element returns the single scene identity shared by drawing, input and actions.
func (b *Button) Element() scene.Element {
	if b == nil {
		return scene.Element{}
	}
	return b.element
}

// Label returns the semantic name used by the default appearance.
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
	requireMutable(b.element)
	result, err := b.action.Invoke(ctx, struct{}{})
	if err == nil && result.State != scene.Unchanged {
		b.element.Invalidate()
	}
	return result, err
}

// Error is the last activation error delivered by keyboard or pointer input.
// Programmatic and agent activation return their error directly.
func (b *Button) Error() error { return b.lastError }

// HeightForWidth reserves one row for the default button appearance.
func (b *Button) HeightForWidth(int) int { return 1 }

// Draw stages the pointer extent and paints the current label without changing action state.
func (b *Button) Draw(frame scene.Frame) { b.Stage(frame); frame.Text(0, 0, b.label, Look{}.Text) }

// Stage derives this candidate frame's pointer extent for an alternate appearance.
func (b *Button) Stage(frame scene.Frame) { b.pointer.Stage(frame, frame.Bounds()) }

// Do executes a named activation through Press and records its UI error.
func (b *Button) Do(action keymap.Action) bool {
	if action != Activate {
		return false
	}
	_, b.lastError = b.Press(context.Background())
	return true
}
