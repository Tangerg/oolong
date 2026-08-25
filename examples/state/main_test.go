package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestOneFieldAPIExpressesEveryStateOwnershipOutcome(t *testing.T) {
	var values answers
	form, local := fields(&values)
	host := programtest.New(t, programtest.Config{Width: 70, Height: 18})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return dress(runtime, form)
			},
		})
	}()

	host.Shows(t, "Who owns this text?")
	host.Type("draft")
	host.Press(input.Tab)
	host.Type("owner")
	host.Press(input.Tab)
	host.Type("mixed")
	host.Shows(t, "MIXED")
	host.Press(input.Tab)
	host.Type("123456789") // the ninth edit is handled but rejected by the owner
	host.Press(input.Enter)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := local.Editor().Text(); got != "draft" {
		t.Fatalf("local value = %q, want draft", got)
	}
	if values.bound != "owner" || values.normalized != "MIXED" || values.guarded != "12345678" {
		t.Fatalf("caller values = %+v", values)
	}
}
