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
	run := &script{backend: m, output: output}
	run.step(ctx, 0, stepRunning)
	run.say(ctx, "# Working plan\n\nI\u2019ll inspect the parser boundary for **"+clip(prompt)+"**.\n\n")
	run.step(ctx, 0, stepDone)

	run.step(ctx, 1, stepRunning)
	run.say(ctx, "The behavior is correct, but the validation and state transition are split across two functions. I can make the parser own that invariant.\n\n")
	run.step(ctx, 1, stepDone)

	proposal := parserProposal()
	run.step(ctx, 2, stepRunning)
	if !run.review(ctx, proposal) {
		run.step(ctx, 2, stepSkipped)
		run.step(ctx, 3, stepSkipped)
		run.say(ctx, "I left the workspace unchanged. The review decision is a domain result, not a transport failure.\n")
		return run.err
	}

	run.tool(ctx, toolResult{
		Name: "apply_patch", Summary: "updated internal/parser.go", Change: proposal,
	})
	run.step(ctx, 2, stepDone)
	run.step(ctx, 3, stepRunning)
	run.say(ctx, "## Verification\n\nThe focused tests and race detector pass. The parser now owns validation and callers have one obvious path.\n\nAll checks pass.\n")
	run.step(ctx, 3, stepDone)
	return run.err
}

// script carries the first failure through a scripted answer, so the answer reads as
// the sequence it is rather than as an error check between every line of it. Once one
// step has failed the rest do nothing, which is what returning early did.
type script struct {
	backend mockBackend
	output  agentOutput
	err     error
}

func (s *script) step(ctx context.Context, index int, state stepState) {
	if s.err == nil {
		s.err = s.output.Step(ctx, stepUpdate{Index: index, State: state})
	}
}

func (s *script) say(ctx context.Context, text string) {
	if s.err == nil {
		s.err = s.backend.write(ctx, s.output, text)
	}
}

func (s *script) tool(ctx context.Context, result toolResult) {
	if s.err == nil {
		s.err = s.output.Tool(ctx, result)
	}
}

// review reports approval, which a failure is not.
func (s *script) review(ctx context.Context, proposal changeProposal) bool {
	if s.err != nil {
		return false
	}
	approved, err := s.output.Review(ctx, proposal)
	s.err = err
	return err == nil && approved
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
