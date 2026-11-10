package main

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/text"
)

// mockChunkBytes is deliberately not a Markdown boundary, so the transcript must
// revise partial syntax exactly as it would for a network transport.
const mockChunkBytes = 19

type mockBackend struct{ delay time.Duration }

func (m mockBackend) Run(ctx context.Context, request runRequest, output agentOutput) error {
	run := &script{backend: m, output: output}
	run.step(ctx, stepUnderstand, stepRunning)
	run.say(ctx, "# Working plan\n\nI’ll inspect the parser boundary for **"+clip(request.prompt)+"**.\n\n")
	run.step(ctx, stepUnderstand, stepDone)

	if request.planningMode == planningFast {
		run.step(ctx, stepInspect, stepSkipped)
	} else {
		run.step(ctx, stepInspect, stepRunning)
		run.say(ctx, "The behavior is correct, but the validation and state transition are split across two functions. I can make the parser own that invariant.\n\n")
		run.step(ctx, stepInspect, stepDone)
	}

	proposal := parserProposal()
	run.step(ctx, stepReview, stepRunning)
	if !run.review(ctx, proposal) {
		run.step(ctx, stepReview, stepSkipped)
		run.step(ctx, stepVerify, stepSkipped)
		run.say(ctx, "I left the workspace unchanged. The review decision is a domain result, not a transport failure.\n")
		return run.err
	}

	run.tool(ctx, toolResult{
		Name: "apply_patch", Summary: "updated internal/parser.go", Change: proposal,
	})
	run.step(ctx, stepReview, stepDone)
	run.step(ctx, stepVerify, stepRunning)
	run.say(ctx, "## Verification\n\nThe focused tests and race detector pass. The parser now owns validation and callers have one obvious path.\n\nAll checks pass.\n")
	run.step(ctx, stepVerify, stepDone)
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

func (s *script) step(ctx context.Context, step stepID, state stepState) {
	if s.err == nil {
		s.err = s.output.Step(ctx, stepUpdate{Step: step, State: state})
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
	for from := 0; from < len(body); from += mockChunkBytes {
		if err := pause(ctx, m.delay); err != nil {
			return err
		}
		to := min(from+mockChunkBytes, len(body))
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

// pause waits for delay, or only observes cancellation when there is none, so a
// zero-delay script still stops at the next chunk once its run is cancelled.
func pause(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return context.Cause(ctx)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

func clip(s string) string { return text.Truncate(strings.TrimSpace(s), promptPreviewColumns, "…") }
