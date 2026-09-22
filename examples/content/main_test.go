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
	host.Shows(t, "explicitly bind")
	host.Shows(t, "√")
	host.Shows(t, "drawable contract")
	host.Type("2")
	host.Shows(t, "println")
	host.Type("3")
	host.Shows(t, "√")
	host.Type("1")
	host.Shows(t, "Three peers, one document")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
