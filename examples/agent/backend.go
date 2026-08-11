package main

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/Tangerg/oolong/core/program"
)

type stepState uint8

const (
	stepWaiting stepState = iota
	stepRunning
	stepDone
	stepSkipped
)

type stepUpdate struct {
	Index int
	State stepState
}

type changeProposal struct {
	Path          string
	Summary       string
	Before, After []string
}

type toolResult struct {
	Name, Summary string
	Change        changeProposal
}

// agentBackend is the application's model boundary. It knows prompts, plans and tool
// review, but nothing about terminal frames, components or the owner goroutine.
type agentBackend interface {
	Run(ctx context.Context, prompt string, output agentOutput) error
}

// agentOutput is the complete conversation one backend run may have with its owner.
// Ordered text uses io.Writer; the few discrete domain transitions have named methods
// because they are not bytes and must never be silently coalesced with them.
type agentOutput interface {
	io.Writer
	Step(ctx context.Context, update stepUpdate) error
	Review(ctx context.Context, proposal changeProposal) (bool, error)
	Tool(ctx context.Context, result toolResult) error
}

type mockBackend struct{ delay time.Duration }

func (m mockBackend) Run(ctx context.Context, prompt string, output agentOutput) error {
	if err := output.Step(ctx, stepUpdate{Index: 0, State: stepRunning}); err != nil {
		return err
	}
	if err := m.write(ctx, output, "# Working plan\n\nI’ll inspect the parser boundary for **"+clip(prompt)+"**.\n\n"); err != nil {
		return err
	}
	if err := output.Step(ctx, stepUpdate{Index: 0, State: stepDone}); err != nil {
		return err
	}
	if err := output.Step(ctx, stepUpdate{Index: 1, State: stepRunning}); err != nil {
		return err
	}
	if err := m.write(ctx, output, "The behavior is correct, but the validation and state transition are split across two functions. I can make the parser own that invariant.\n\n"); err != nil {
		return err
	}
	if err := output.Step(ctx, stepUpdate{Index: 1, State: stepDone}); err != nil {
		return err
	}

	proposal := parserProposal()
	if err := output.Step(ctx, stepUpdate{Index: 2, State: stepRunning}); err != nil {
		return err
	}
	approved, err := output.Review(ctx, proposal)
	if err != nil {
		return err
	}
	if !approved {
		if err := output.Step(ctx, stepUpdate{Index: 2, State: stepSkipped}); err != nil {
			return err
		}
		if err := output.Step(ctx, stepUpdate{Index: 3, State: stepSkipped}); err != nil {
			return err
		}
		return m.write(ctx, output, "I left the workspace unchanged. The review decision is a domain result, not a transport failure.\n")
	}
	if err := output.Tool(ctx, toolResult{
		Name: "apply_patch", Summary: "updated internal/parser.go", Change: proposal,
	}); err != nil {
		return err
	}
	if err := output.Step(ctx, stepUpdate{Index: 2, State: stepDone}); err != nil {
		return err
	}
	if err := output.Step(ctx, stepUpdate{Index: 3, State: stepRunning}); err != nil {
		return err
	}
	if err := m.write(ctx, output, "## Verification\n\nThe focused tests and race detector pass. The parser now owns validation and callers have one obvious path.\n\nAll checks pass.\n"); err != nil {
		return err
	}
	return output.Step(ctx, stepUpdate{Index: 3, State: stepDone})
}

func parserProposal() changeProposal {
	return changeProposal{
		Path:    "internal/parser.go",
		Summary: "move validation into parser.advance",
		Before: []string{
			"func parse(token string) error {",
			"    if token == \"\" { return errEmpty }",
			"    return state.advance(token)",
			"}",
		},
		After: []string{
			"func parse(token string) error {",
			"    return state.advance(token)",
			"}",
			"",
			"func (s *parser) advance(token string) error {",
			"    if token == \"\" { return errEmpty }",
			"    // transition remains owned here",
			"    return nil",
			"}",
		},
	}
}

func (m mockBackend) write(ctx context.Context, dst io.Writer, body string) error {
	const chunk = 19
	for from := 0; from < len(body); from += chunk {
		if m.delay > 0 {
			timer := time.NewTimer(m.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Cause(ctx)
			case <-timer.C:
			}
		} else {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			default:
			}
		}
		to := min(from+chunk, len(body))
		n, err := io.WriteString(dst, body[from:to])
		if err != nil {
			return err
		}
		if n != to-from {
			return io.ErrShortWrite
		}
	}
	return nil
}

// agentBridge is the run-owned adapter from background domain work into the two
// ownership channels. Bytes use bounded ingress. Sparse transitions use Dispatcher
// and wait for the owner, preserving their order relative to earlier byte drains.
type agentBridge struct {
	ingress  *program.ByteIngress
	dispatch program.Dispatcher
	owner    *agent
	run      *agentRun
}

func (b *agentBridge) Write(p []byte) (int, error) { return b.ingress.Write(p) }

func (b *agentBridge) Step(ctx context.Context, update stepUpdate) error {
	return b.post(ctx, func() { b.owner.workflow.Apply(update) })
}

func (b *agentBridge) Tool(ctx context.Context, result toolResult) error {
	return b.post(ctx, func() { b.owner.showTool(result) })
}

func (b *agentBridge) Review(ctx context.Context, proposal changeProposal) (bool, error) {
	request := &reviewRequest{proposal: proposal, answer: make(chan bool, 1)}
	if err := b.post(ctx, func() { b.owner.openReview(request) }); err != nil {
		return false, err
	}
	select {
	case answer := <-request.answer:
		return answer, nil
	case <-ctx.Done():
		return false, context.Cause(ctx)
	case <-b.dispatch.Done():
		return false, program.ErrStopped
	}
}

func (b *agentBridge) post(ctx context.Context, fn func()) error {
	applied := make(chan error, 1)
	b.dispatch.Post(func() {
		if b.owner.run != b.run || ctx.Err() != nil {
			err := context.Cause(ctx)
			if err == nil {
				err = context.Canceled
			}
			applied <- err
			return
		}
		fn()
		applied <- nil
	})
	select {
	case err := <-applied:
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-b.dispatch.Done():
		return program.ErrStopped
	}
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
