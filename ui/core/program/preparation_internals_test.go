package program

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func preparationOwner(t *testing.T, element scene.Element) (*program, *Runtime, *drainingWriter) {
	t.Helper()
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	writer := &drainingWriter{changes: make(chan struct{}, 1)}
	pipe := &inputPipe{events: make(chan ownerInput, 2), consumed: make(chan int, 2), done: make(chan struct{})}
	var stopInput sync.Once
	p := &program{
		root: root, canvas: grid.NewScreen(20, 4), writer: writer, tasks: newTaskQueue(), inputPipe: pipe,
		cancel: func() { stopInput.Do(func() { close(pipe.done) }) },
	}
	t.Cleanup(p.stop)
	return p, &Runtime{p: p}, writer
}

func TestPendingFramePreparationDoesNotArmAPoll(t *testing.T) {
	p, runtime, _ := preparationOwner(t, scene.NewElement(scene.ElementConfig{}))
	if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	p.runTasks()
	if err := p.draw(); !errors.Is(err, errFrameNotReady) {
		t.Fatalf("unprepared frame = %v", err)
	}
	if _, armed := p.present.DueAt(); armed {
		t.Fatal("a preparation wait kept a frame timer actionable")
	}
}

func TestClaimedRepaintWaitsForPreparationAndPreservesInputOrder(t *testing.T) {
	ready, inputs, replies := false, 0, 0
	p, runtime, writer := preparationOwner(t, scene.NewElement(scene.ElementConfig{
		Paint:  func(frame scene.Frame) { frame.Text(0, 0, "ready", grid.Style{}) },
		Handle: func(scene.Event) scene.EventResult { inputs++; return scene.EventResult{Consumed: true} },
	}))
	if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return ready, nil }); err != nil {
		t.Fatal(err)
	}
	p.runTasks()
	if err := p.answerRepaint(func(w, h int, err error) {
		replies++
		if err != nil || w != 20 || h != 4 || inputs != 0 || writer.Written() != writer.queued || len(writer.frames) != 1 {
			t.Errorf("premature repaint: size=%dx%d inputs=%d written=%d frames=%d err=%v", w, h, inputs, writer.Written(), len(writer.frames), err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if replies != 0 || p.pendingRepaint == nil {
		t.Fatal("unprepared repaint reported completion")
	}
	p.inputPipe.events <- ownerInput{event: p.root.CaptureInput(input.Key{Rune: 'x'}, nil), bytes: 1}
	if err := p.tasks.post(func() { ready = true }); err != nil {
		t.Fatal(err)
	}
	due := deadline.NewTimer()
	defer due.Stop()
	if last, err := p.wait(t.Context(), due); last || err != nil || !ready || inputs != 0 {
		t.Fatalf("readiness wait consumed later input: last=%t ready=%t inputs=%d err=%v", last, ready, inputs, err)
	}
	if err := p.attemptRepaint(); err != nil || replies != 1 || p.pendingRepaint != nil {
		t.Fatalf("completed repaint: replies=%d pending=%t err=%v", replies, p.pendingRepaint != nil, err)
	}
	if last, err := p.wait(t.Context(), due); last || err != nil || inputs != 1 {
		t.Fatalf("repaint did not restore ordered input: last=%t inputs=%d err=%v", last, inputs, err)
	}
}

func TestCancellationSettlesAPreparationBlockedRepaint(t *testing.T) {
	paints, replies := 0, 0
	p, runtime, _ := preparationOwner(t, scene.NewElement(scene.ElementConfig{Paint: func(scene.Frame) { paints++ }}))
	if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	p.runTasks()
	if err := p.answerRepaint(func(w, h int, err error) {
		replies++
		if w != 0 || h != 0 || !errors.Is(err, ErrStopped) {
			t.Errorf("cancelled repaint size=%dx%d err=%v", w, h, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := p.run(ctx); err != nil {
		t.Fatal(err)
	}
	if replies != 1 || paints != 0 || p.pendingRepaint != nil {
		t.Fatalf("replies=%d paints=%d pending=%t", replies, paints, p.pendingRepaint != nil)
	}
}

func TestInputEOFFailsPendingRepaintBeforeDrainingAcceptedInput(t *testing.T) {
	inputs, replies := 0, 0
	p, runtime, _ := preparationOwner(t, scene.NewElement(scene.ElementConfig{Handle: func(scene.Event) scene.EventResult {
		if replies != 1 {
			t.Error("queued input overtook its repaint barrier")
		}
		inputs++
		return scene.EventResult{Consumed: true}
	}}))
	if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	p.runTasks()
	if err := p.answerRepaint(func(w, h int, err error) {
		replies++
		if w != 0 || h != 0 || !errors.Is(err, ErrStopped) {
			t.Errorf("EOF repaint size=%dx%d err=%v", w, h, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	p.inputPipe.events <- ownerInput{event: p.root.CaptureInput(input.Key{Rune: 'x'}, nil), bytes: 1}
	close(p.inputPipe.events)
	p.cancel()
	due := deadline.NewTimer()
	defer due.Stop()
	for {
		last, err := p.wait(t.Context(), due)
		if err != nil {
			t.Fatal(err)
		}
		if last {
			break
		}
	}
	if replies != 1 || inputs != 1 || p.pendingRepaint != nil {
		t.Fatalf("replies=%d inputs=%d pending=%t", replies, inputs, p.pendingRepaint != nil)
	}
}

func TestPendingPreparationFinishesAcceptedInlineOutputInOrder(t *testing.T) {
	for _, resize := range []bool{false, true} {
		paints, value := 0, "visible"
		element := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			paints++
			frame.Text(0, 0, value, grid.Style{})
		}})
		p, runtime, writer := preparationOwner(t, element)
		p.inline = grid.NewInline(40, 6)
		p.canvas = p.inline
		var publisherErr error
		p.publisher, publisherErr = publication.New(publication.Config{Width: 40})
		if publisherErr != nil {
			t.Fatal(publisherErr)
		}
		if _, err := p.compose(true); err != nil {
			t.Fatal(err)
		}
		first, err := p.publisher.TryPublish(permanentLine("permanent-one"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := p.publisher.TryPublish(permanentLine("permanent-two"))
		if err != nil {
			t.Fatal(err)
		}
		if resize {
			if err := p.handleInput(p.root.CaptureInput(input.Resize{Width: 20, Height: 4}, nil)); err != nil {
				t.Fatal(err)
			}
		}
		value = "unprepared"
		element.Invalidate()
		if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); err != nil {
			t.Fatal(err)
		}
		p.stopWork()
		p.publisher.CloseAdmission(ErrStopped)
		if err := p.finish(); err != nil {
			t.Fatal(err)
		}
		var output strings.Builder
		for _, frame := range writer.frames {
			output.Write(frame)
		}
		wire := output.String()
		if paints != 1 || strings.Count(wire, "visible") != 1 || strings.Contains(wire, "unprepared") ||
			strings.Count(wire, "permanent-one") != 1 || strings.Count(wire, "permanent-two") != 1 ||
			strings.Index(wire, "permanent-one") > strings.Index(wire, "permanent-two") {
			t.Fatalf("resize=%t paints=%d output=%q", resize, paints, wire)
		}
		for _, receipt := range []publication.Receipt{first, second} {
			if status := receipt.Snapshot(); status.State != publication.Written || status.Err != nil {
				t.Errorf("accepted permanent output did not settle: %+v", status)
			}
		}
		if items, _ := p.publisher.Pending(); items != 0 {
			t.Fatalf("shutdown left %d pending publications", items)
		}
	}
}

func TestUnpreparedShutdownDoesNotPublishTheRetiredLiveProjectionAgain(t *testing.T) {
	for _, copies := range []int{0, 1, 2} {
		value, paints := "shared-body", 0
		element := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			paints++
			frame.Text(0, 0, value, grid.Style{})
		}})
		p, runtime, writer := preparationOwner(t, element)
		p.inline = grid.NewInline(30, 6)
		p.canvas = p.inline
		var publisherErr error
		p.publisher, publisherErr = publication.New(publication.Config{Width: 30})
		if publisherErr != nil {
			t.Fatal(publisherErr)
		}
		if _, err := p.compose(true); err != nil {
			t.Fatal(err)
		}
		for range copies {
			if _, err := p.publisher.TryPublish(permanentLine(value)); err != nil {
				t.Fatal(err)
			}
		}
		value = "next-unprepared-body"
		element.Invalidate()
		if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); err != nil {
			t.Fatal(err)
		}
		p.stopWork()
		p.publisher.CloseAdmission(ErrStopped)
		if err := p.finish(); err != nil {
			t.Fatal(err)
		}
		shown, screenErr := ptytest.NewScreen(ptytest.Size{Cols: 30, Rows: 6})
		if screenErr != nil {
			t.Fatal(screenErr)
		}
		for _, frame := range writer.frames {
			if err := shown.Apply(frame); err != nil {
				t.Fatal(err)
			}
		}
		if err := shown.Flush(); err != nil {
			t.Fatal(err)
		}
		rows := strings.Join(shown.Rows(), "\n")
		if got := strings.Count(rows, "shared-body"); got != max(1, copies) || paints != 1 || strings.Contains(rows, value) {
			t.Fatalf("publications=%d paints=%d final rows=%q", copies, paints, rows)
		}
	}
}

func TestResumedRepaintFailureEndsTheSessionAfterItsErrorReply(t *testing.T) {
	for _, preparationFailure := range []bool{false, true} {
		ready, replies := false, 0
		p, runtime, _ := preparationOwner(t, scene.NewElement(scene.ElementConfig{}))
		p.writer = &admissionWriter{refuse: !preparationFailure}
		if err := runtime.SetFramePreparation(func(int, int) (bool, error) {
			if ready && preparationFailure {
				return false, term.ErrQueueFull
			}
			return ready, nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := p.answerRepaint(func(w, h int, err error) {
			replies++
			if w != 0 || h != 0 || !errors.Is(err, term.ErrQueueFull) {
				t.Errorf("failed repaint size=%dx%d err=%v", w, h, err)
			}
		}); err != nil {
			t.Fatal(err)
		}
		ready = true
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := p.run(ctx); !errors.Is(err, term.ErrQueueFull) || replies != 1 || p.pendingRepaint != nil {
			t.Fatalf("preparation failure=%t replies=%d pending=%t err=%v", preparationFailure, replies, p.pendingRepaint != nil, err)
		}
	}
}
