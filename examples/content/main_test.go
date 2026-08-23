package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestThePeerRenderersComposeAndQuit(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 88, Height: 22})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return newContent(runtime)
			},
		})
	}()

	host.Shows(t, "Three peers, one document")
	host.Shows(t, "SetRenderer")
	host.Shows(t, "√")
	host.Shows(t, "styled text")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
