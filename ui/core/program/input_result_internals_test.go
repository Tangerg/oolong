package program

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestRunSettlesTheSourceResultOnceAcrossEOFAndShutdown(t *testing.T) {
	sourceErr := errors.New("input transport failed")
	frameErr := errors.New("frame preparation failed")
	for _, ending := range []string{"input EOF", "quit", "frame failure"} {
		t.Run(ending, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			queries := 0
			source := &sourceWithResult{events: make(chan input.Event), result: func() error {
				queries++
				return sourceErr
			}}
			close(source.events)
			owner, runtime, _ := preparationOwner(t, scene.NewElement(scene.ElementConfig{}))
			owner.cancel = cancel
			owner.inputPipe = newInputPipe(ctx, owner.root, source, source.Events(), nil, Config{})
			<-owner.inputPipe.done
			switch ending {
			case "quit":
				runtime.Quit()
			case "frame failure":
				if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, frameErr }); err != nil {
					t.Fatal(err)
				}
			}
			err := owner.run(ctx)
			if queries != 1 || !errors.Is(err, sourceErr) || ending == "frame failure" && !errors.Is(err, frameErr) {
				t.Fatalf("Run=%v source result queried %d times", err, queries)
			}
			if err := owner.inputPipe.takeResult(); err != nil || queries != 1 {
				t.Fatalf("settled source result replayed: err=%v queries=%d", err, queries)
			}
		})
	}
}
