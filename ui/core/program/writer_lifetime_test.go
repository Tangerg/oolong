package program_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type exitingOutput struct{}

func (exitingOutput) Write([]byte) (int, error) {
	runtime.Goexit()
	return 0, nil
}

func TestRunReportsAWriterExitWithoutWaitingForInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		writer := term.NewWriter(term.WriterConfig{Destination: exitingOutput{}})
		t.Cleanup(func() { _ = writer.Close() })
		host := &minimalHost{events: make(chan input.Event), writer: writer, w: 4, h: 2}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- runFixture(ctx, program.ScreenConfig{Host: host, Build: func(*program.Runtime) (scene.Element, error) { return fixtureElement(&component{}), nil }})
		}()
		select {
		case err := <-done:
			if err == nil || !errors.Is(err, writer.Err()) {
				t.Fatalf("Run = %v, want the writer's interruption error %v", err, writer.Err())
			}
		case <-time.After(time.Second):
			cancel()
			<-done
			t.Fatal("Run stayed alive after its writer exited")
		}
	})
}
