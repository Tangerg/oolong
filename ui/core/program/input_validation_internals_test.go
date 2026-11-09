package program

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestHostRejectsInvalidKeyTransition(t *testing.T) {
	source := &pipeSource{events: make(chan input.Event, 1)}
	source.events <- input.Key{Code: input.Enter, Transition: input.Transition(255)}
	close(source.events)
	pipe := newInputPipe(t.Context(), nil, source, source.Events(), nil, Config{})
	<-pipe.done
	for item := range pipe.events {
		key, _ := item.event.Input().(input.Key)
		t.Errorf("invalid transition crossed host boundary: %+v Down=%v", key, key.Down())
	}
	if err := pipe.takeResult(); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("invalid host input ended as success: %v", err)
	}
}

func TestInvalidKeyTransitionCannotActivateButton(t *testing.T) {
	activations := 0
	button := base.NewButton(base.ButtonConfig{Label: "Run", Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		activations++
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	root, err := scene.NewRoot(scene.RootConfig{Element: button.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	root.FlushNotifications()
	frame, err := root.Prepare(grid.NewSurface(10, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	frame.Commit(1)
	root.Wrote(1)
	consumed := root.Handle(input.Key{Code: input.Enter, Transition: input.Transition(255)})
	if consumed || activations != 0 {
		t.Fatalf("invalid transition activated button: consumed=%v activations=%d", consumed, activations)
	}
}
