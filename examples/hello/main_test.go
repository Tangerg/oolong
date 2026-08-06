package main

import (
	"context"
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/fake"
)

func TestItDrawsAndAnswersAndStops(t *testing.T) {
	host := fake.New(60, 12)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(context.Background(), program.Config{
			Host: host,
			Root: func(loop program.Loop) program.Component { return &hello{loop: loop} },
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
