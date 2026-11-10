package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/oolong/examples/internal/streamtask"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

const elapsedFormat = "%4.1fs"

// runPhase orders one backend run. Only a streaming run accepts backend output; a
// finishing run has closed its source and waits for final preparation to settle.
type runPhase uint8

const (
	runStreaming runPhase = iota
	runFinishing
)

// agentRun owns every fact that exists only while one backend run is current. Its
// identity also tells late callbacks from an earlier run apart from this one.
type agentRun struct {
	phase   runPhase
	task    *program.Task
	clock   *program.Timer
	started time.Time
	status  kit.Status
}

func (r *agentRun) tick() {
	r.status.Tick()
	r.status.Elapsed = fmt.Sprintf(elapsedFormat, time.Since(r.started).Seconds())
}

// streaming reports whether run is the current run and still accepts backend output.
func (a *agent) streaming(run *agentRun) bool {
	return run != nil && a.run == run && run.phase == runStreaming
}

func (a *agent) startRun(prompt string) {
	run := &agentRun{started: time.Now(), status: kit.Status{Theme: a.theme, Glyphs: a.glyphs}}
	a.run = run
	a.outcome = sessionOutcome{}
	if err := a.conversation.User(a.scope.Context(), prompt); err != nil {
		a.finishRun(err)
		return
	}
	a.composer.Controller().Clear()
	a.completion.Dismiss()
	a.workflow.Reset()
	request := runRequest{prompt: prompt, planningMode: a.planningMode}
	a.inform("planning with " + string(request.planningMode))
	a.setProgress(term.Progress{State: term.ProgressIndeterminate})

	var err error
	if run.clock, err = a.runtime.Every(a.scope, statusTickInterval, run.tick); err != nil {
		a.finishRun(err)
		return
	}
	ingress, err := program.NewByteIngress(program.ByteIngressConfig{
		Dispatcher: a.runtime.Dispatcher(), Limit: ingressSize,
		Consume: func(batch program.ByteBatch) { a.accept(run, batch) },
	})
	if err != nil {
		a.finishRun(err)
		return
	}
	output := &agentBridge{ingress: ingress, dispatch: a.runtime.Dispatcher(), owner: a, run: run}
	run.task = streamtask.Start(a.runtime.Runtime, a.scope, ingress, func(ctx context.Context) error {
		return a.backend.Run(ctx, request, output)
	})
}

// accept ignores batches from any run but the streaming one: closing an ingress
// does not withdraw a delivery already queued on the owner.
func (a *agent) accept(run *agentRun, batch program.ByteBatch) {
	if !a.streaming(run) {
		return
	}
	if len(batch.Data) > 0 {
		if err := a.conversation.Markdown(a.scope.Context(), string(batch.Data)); err != nil {
			a.finishRun(errors.Join(batch.Err, err))
			return
		}
	}
	if batch.Final {
		a.finishRun(batch.Err)
	}
}

func (a *agent) finishRun(err error) {
	run := a.run
	if !a.streaming(run) {
		return
	}
	run.phase = runFinishing
	if endErr := a.conversation.FinishMarkdown(a.scope.Context(), err); endErr != nil {
		err = errors.Join(err, endErr)
	}
	a.withdrawReview(context.Canceled)
	run.task.Cancel()
	a.inform("preparing final answer")
	settleErr := a.conversation.Settled(func(prepareErr error) {
		if a.run != run {
			return
		}
		if prepareErr != nil {
			err = errors.Join(err, prepareErr)
		}
		a.settleRun(err)
	})
	if settleErr != nil {
		a.settleRun(errors.Join(err, settleErr))
	}
}

func (a *agent) settleRun(err error) {
	a.run.clock.Stop()
	a.run = nil
	outcome := finishedOutcome(err)
	if outcome.kind == outcomeFailed {
		if appendErr := a.conversation.Append(a.scope.Context(), &kit.Entry{Theme: a.theme, Label: "runtime", Body: err.Error()}); appendErr != nil {
			err = errors.Join(err, appendErr)
			outcome = finishedOutcome(err)
		}
	}
	if progressErr := a.settledProgress(outcome.progress()); progressErr != nil {
		outcome = finishedOutcome(errors.Join(err, progressErr))
		a.setProgress(outcome.progress())
	}
	a.outcome = outcome
	if outcome.kind == outcomeCompleted {
		_ = a.runtime.Session().Notify("mock agent completed")
	}
}

func (a *agent) stopSource() {
	run := a.run
	if !a.streaming(run) || !run.task.Cancel() {
		return
	}
	a.inform("cancelling")
	a.withdrawReview(context.Canceled)
}

// Native progress is optional host chrome: an unsupported or failed indicator
// must not change the run it describes.
func (a *agent) setProgress(progress term.Progress) {
	a.progressReset.Stop()
	a.progressReset = nil
	_ = a.runtime.Session().SetProgress(progress)
}

// settledProgress shows a settled outcome briefly, then clears it so an idle
// session does not keep advertising a finished run.
func (a *agent) settledProgress(progress term.Progress) error {
	a.setProgress(progress)
	timer, err := a.runtime.After(a.scope, settledProgressDuration, func() {
		a.progressReset = nil
		_ = a.runtime.Session().SetProgress(term.Progress{})
	})
	if err != nil {
		return err
	}
	a.progressReset = timer
	return nil
}
