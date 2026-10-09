package program_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

type exitingOutput struct{}

func (exitingOutput) Write([]byte) (int, error) {
	runtime.Goexit()
	return 0, nil
}

func TestRunReportsAWriterExitWithoutWaitingForInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		writer := term.NewWriter(exitingOutput{})
		t.Cleanup(func() { _ = writer.Close() })
		host := &minimalHost{events: make(chan input.Event), writer: writer, w: 4, h: 2}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- program.Run(ctx, program.Config{
				Host: host,
				Root: func(*program.Runtime) program.Component { return &component{} },
			})
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
