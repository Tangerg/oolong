package main

import (
	"errors"
	"testing"
)

func TestRunOutcomeSeparatesFailuresWithinOneStatusLine(t *testing.T) {
	outputFailure := errors.New("output limit exceeded")
	exitFailure := errors.New("exit status 1")
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "success", want: "done"},
		{name: "single", err: outputFailure, want: "failed: output limit exceeded"},
		{name: "output first", err: errors.Join(outputFailure, exitFailure), want: "failed: output limit exceeded; exit status 1"},
		{name: "exit first", err: errors.Join(exitFailure, outputFailure), want: "failed: exit status 1; output limit exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := (runOutcome{err: test.err}).String(); got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
		})
	}
}
