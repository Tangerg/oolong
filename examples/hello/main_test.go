package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestItDrawsAndAnswersAndStops(t *testing.T) {
	host := programtest.New(t, 60, 12)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component { return &hello{runtime: runtime} },
		})
	}()

	host.Shows(t, "press a key")
	host.Type("ab")
	host.Shows(t, "2 keys so far")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
