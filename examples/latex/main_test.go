package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestTheFormulaDrawsDirectlyAndQuits(t *testing.T) {
	host := programtest.New(t, 72, 16)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return newLatex(runtime)
			},
		})
	}()

	host.Shows(t, "LaTeX")
	host.Shows(t, "√")
	host.Shows(t, "±")
	host.Shows(t, "latex.Formula")

	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
