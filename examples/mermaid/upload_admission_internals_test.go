package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/extensions/mermaid"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

type cancellingImageHost struct {
	*imageHost
	cancel context.CancelFunc
}

func (h *cancellingImageHost) Transmit(ctx context.Context, data []byte) (<-chan graphics.UploadResult, error) {
	result, err := h.imageHost.Transmit(ctx, data)
	h.cancel()
	return result, err
}

func TestUploadRefusalDoesNotTransmitAnImage(t *testing.T) {
	host := &imageHost{protocol: graphics.Kitty, Host: uitest.New(t, uitest.Config{Width: 50, Height: 10})}
	var screen *diagramScreen
	app, err := program.NewScreen(program.ScreenConfig{
		Host: host, Resources: 1,
		Build: func(rt *program.Runtime) (scene.Element, error) {
			screen = newDiagramScreen(rt, host, nil)
			work, err := rt.NewScope(program.ScopeConfig{})
			if err != nil {
				return scene.Element{}, err
			}
			screen.current = diagram{source: flowchart, work: work}
			screen.rendered(new(mermaid.Image), nil)
			rt.Quit()
			return screen.Element(), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := screen.close(); err != nil {
		t.Fatal(err)
	}
	if screen.current.phase != diagramFailed || !errors.Is(screen.current.failure, program.ErrCapacity) {
		t.Fatalf("refused upload = %v, %v; want a capacity failure", screen.current.phase, screen.current.failure)
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.sent != 0 || host.released != 0 {
		t.Fatalf("refused upload transmitted %d images and required %d compensating releases", host.sent, host.released)
	}
}

func TestUploadCancellationRetainsCleanupAfterAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host := &cancellingImageHost{
		imageHost: &imageHost{protocol: graphics.Kitty, Host: uitest.New(t, uitest.Config{Width: 50, Height: 10})},
		cancel:    cancel,
	}
	if err := runScreen(ctx, host, func(context.Context, string) (*mermaid.Image, error) {
		return new(mermaid.Image), nil
	}); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.sent != 1 || host.released != 1 {
		t.Fatalf("revoked upload transmitted %d images and released %d; want one each", host.sent, host.released)
	}
}
