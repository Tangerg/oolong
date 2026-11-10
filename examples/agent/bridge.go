package main

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/program"
)

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
	return b.post(ctx, func() error { return b.owner.workflow.Apply(update) })
}

func (b *agentBridge) Tool(ctx context.Context, result toolResult) error {
	return b.post(ctx, func() error { return b.owner.showTool(result) }) //nolint:contextcheck // ctx governs dispatch admission; a claimed UI callback uses the session scope to finish its visible transition.
}

func (b *agentBridge) Review(ctx context.Context, proposal changeProposal) (bool, error) {
	request := newReviewRequest(ctx, proposal)
	if err := b.post(ctx, func() error { return b.owner.openReview(request) }); err != nil { //nolint:contextcheck // Request cancellation governs admission and the reply wait; UI callbacks use the session scope.
		return false, err
	}
	select {
	case <-request.done:
	case <-b.dispatch.Done():
		select {
		case <-request.done:
		default:
			return false, program.ErrStopped
		}
	}
	return request.approved, request.err
}

// post refuses a transition once its run has stopped streaming, so a late backend
// call can neither change a finishing run nor reach the run that replaced it.
func (b *agentBridge) post(ctx context.Context, fn func() error) error {
	return b.dispatch.Invoke(ctx, func() error {
		if !b.owner.streaming(b.run) {
			return context.Canceled
		}
		return fn()
	})
}
