package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestExactAndLongerSequencesBothWork(t *testing.T) {
	host := programtest.New(t, 70, 12)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component { return newKeys(runtime) },
		})
	}()

	host.Shows(t, "waiting for g or gg")
	host.Type("g")
	host.Shows(t, "resolved g")
	host.Type("gg")
	host.Shows(t, "resolved gg")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
}
