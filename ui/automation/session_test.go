package automation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/automation"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestDriverFollowsItsRuntimeThroughShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ready := make(chan *automation.Driver, 1)
	screen, err := program.NewScreen(program.ScreenConfig{Host: uitest.New(t, uitest.Config{Width: 3, Height: 1}), Build: func(rt *program.Runtime) (scene.Element, error) {
		ready <- automation.NewDriver(rt)
		return scene.NewElement(scene.ElementConfig{Key: "session"}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- screen.Run(ctx) }()
	var d *automation.Driver
	select {
	case d = <-ready:
	case err = <-stopped:
		t.Fatal("session stopped during construction", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	nodes, err := d.Observe(ctx, scene.Query{})
	if err != nil || len(nodes.Nodes) != 1 || nodes.Nodes[0].Key != "session" {
		t.Fatal(nodes, err)
	}
	cancel()
	<-stopped
	if _, err = d.Observe(t.Context(), scene.Query{}); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("stale session observation: %v", err)
	}
}
