package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestTheDocumentReflowsAndQuits(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 72, Height: 20})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return newMarkdown(runtime)
			},
		})
	}()

	host.Shows(t, "Structured text stays structured")
	host.Shows(t, "immutable blocks")
	host.Shows(t, "final grid")
	host.Resize(44, 20)
	host.Shows(t, "Parsing, measuring, drawing")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
