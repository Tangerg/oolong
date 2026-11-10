// Package drawn builds the one focusable element a full-screen example paints
// itself into, so each example states only what it draws and which keys it
// handles. The element exists from construction, so an early invalidation is
// never silently lost on an element that has not been made yet.
package drawn

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// View is an example's whole interface: what it draws and the input it answers.
type View interface {
	Draw(view grid.View)
	Handle(event input.Event) bool
}

// Element is a focusable element that paints view and gives it every event.
func Element(key string, view View) scene.Element {
	return scene.NewElement(scene.ElementConfig{
		Key:       key,
		Focusable: func() bool { return true },
		Paint:     func(frame scene.Frame) { view.Draw(frame.View) },
		Handle: func(event scene.Event) scene.EventResult {
			return scene.EventResult{Consumed: view.Handle(event.Input())}
		},
	})
}
