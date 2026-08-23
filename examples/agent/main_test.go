package main

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestParseCommandKeepsProductSyntaxInTheApplication(t *testing.T) {
	for _, tc := range []struct {
		line      string
		name, arg string
		ok        bool
	}{
		{line: "/clear", name: "clear", ok: true},
		{line: "/model gpt-5", name: "model", arg: "gpt-5", ok: true},
		{line: "/model   spaced  out  ", name: "model", arg: "spaced  out", ok: true},
		{line: "/", ok: true},
		{line: "not a command", ok: false},
		{line: "", ok: false},
		{line: " /clear", ok: false},
	} {
		name, arg, ok := parseCommand(tc.line)
		if ok != tc.ok || name != tc.name || arg != tc.arg {
			t.Errorf("%q = (%q, %q, %v), want (%q, %q, %v)", tc.line, name, arg, ok, tc.name, tc.arg, tc.ok)
		}
	}
}

func runAgent(t *testing.T, backend agentBackend) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, programtest.Config{Width: 90, Height: 24})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- program.Run(ctx, program.Config{
			Host: host,
			Inline: func(runtime *program.InlineRuntime) program.Component {
				return headless.NewRoot(newAgent(runtime, backend))
			},
		})
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("agent stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func TestMockAgentStreamsReviewsAndVerifiesOneRun(t *testing.T) {
	host, stop := runAgent(t, mockBackend{})
	host.Shows(t, "Ask the mock agent")

	host.Type("move validation into its owner")
	host.Press(input.Enter)
	host.Shows(t, "Review tool call")
	host.Shows(t, "internal/parser.go")
	host.Shows(t, "Allow this tool call?")

	host.Press(input.Enter)
	// The tool label may already belong to terminal history by the time the run
	// completes. Assert the resulting diff instead of requiring committed output
	// to remain in the application's current frame.
	host.Shows(t, "transition remains owned here")
	host.Shows(t, "All checks pass")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestDenyingAToolCallIsAProductResult(t *testing.T) {
	host, stop := runAgent(t, mockBackend{})
	host.Shows(t, "Ask the mock agent")
	host.Type("make a risky change")
	host.Press(input.Enter)
	host.Shows(t, "Review tool call")

	host.Press(input.Esc)
	host.Shows(t, "denied")
	host.Shows(t, "left the workspace unchanged")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSlashCompletionRunsTheRegisteredCommand(t *testing.T) {
	host, stop := runAgent(t, mockBackend{})
	host.Shows(t, "Ask the mock agent")
	host.Type("/he")
	host.Shows(t, "show the commands")

	// The first Enter accepts the completion; the second executes the completed line.
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "/clear")
	host.Shows(t, "/model <fast|careful>")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

type waitingBackend struct{}

func (waitingBackend) Run(ctx context.Context, _ string, output agentOutput) error {
	if err := output.Step(ctx, stepUpdate{Index: 0, State: stepRunning}); err != nil {
		return err
	}
	if _, err := io.WriteString(output, "Waiting for a cancellable source.\n"); err != nil {
		return err
	}
	<-ctx.Done()
	return context.Cause(ctx)
}

func TestCancellingSettlesTheBackgroundRun(t *testing.T) {
	host, stop := runAgent(t, waitingBackend{})
	host.Shows(t, "Ask the mock agent")
	host.Type("wait")
	host.Press(input.Enter)
	host.Shows(t, "Waiting for a cancellable source")

	host.Send(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl})
	host.Shows(t, "cancelled")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}
