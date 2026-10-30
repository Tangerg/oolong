// Package base supplies unstyled behavior composed from scene, text and layout.
// Controllers own semantic state; scene owns their actual attachment and input route.
// Mutating methods run on the UI owner sequence. Mounted controllers reject
// mutation during scene presentation, observation or a pending frame. Raw Handle
// methods expose local behaviour; applications route physical input through scene
// so focus, capture and written-frame coordinates remain under its ownership.
package base

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Block is passive content that can leave the live scene for a transcript.
type Block interface{ grid.Drawable }

// RowGutter paints decorations without taking ownership of text or its positions.
type RowGutter interface {
	Width(lines int) int
	Draw(view grid.View, rows []text.Row)
}

// Doer consumes a symbolic input command. A consumed command is not a task result.
type Doer interface {
	Do(action keymap.Action) bool
}

// eventGeometry only borrows the immutable projection selected by scene for one
// input delivery. It never publishes geometry or retains a second routing owner.
type eventGeometry struct{ event *scene.Event }

func (g *eventGeometry) read[T any](snapshot *scene.Snapshot[T]) T {
	if g.event != nil {
		value, _ := g.event.Read(snapshot)
		return value
	}
	return snapshot.Value()
}

func requireMutable(elements ...scene.Element) {
	for _, element := range elements {
		if err := element.CheckMutation(); err != nil {
			panic(err)
		}
	}
}

func partEvent(event scene.Event, handle func(input.Event) bool, projections ...*eventGeometry) scene.EventResult {
	previous := make([]*scene.Event, len(projections))
	for i, projection := range projections {
		previous[i], projection.event = projection.event, &event
	}
	defer func() {
		for i, projection := range projections {
			projection.event = previous[i]
		}
	}()

	consumed := handle(event.Input)
	result := scene.EventResult{Consumed: consumed}
	if mouse, ok := event.Input.(input.Mouse); ok && consumed && mouse.Action == input.MouseDown && mouse.Button == input.ButtonLeft {
		result.Focus, result.Capture = true, true
	}
	return result
}
