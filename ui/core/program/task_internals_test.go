package program

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func taskOwner(t *testing.T) (*program, *Runtime) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	p := &program{ctx: ctx, cancel: cancel, tasks: newTaskQueue()}
	root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{})})
	if err != nil {
		t.Fatal(err)
	}
	p.root = root
	t.Cleanup(p.stop)
	return p, &Runtime{p: p}
}

func settleTask(t *testing.T, p *program, task *Task) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-task.Settled():
			return
		case <-p.tasks.wake:
			p.runTasks()
		case <-deadline.C:
			t.Fatal("accepted task did not settle")
		}
	}
}

func TestTaskCompletionHasReservedCapacity(t *testing.T) {
	p, rt := taskOwner(t)
	p.tasks.capacity = 1
	scope, err := rt.NewScope(ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	release := make(chan struct{})
	var got int
	task, err := rt.Start(scope, Work[int]{
		Run: func(context.Context) (int, error) { <-release; return 42, nil },
		Apply: func(value int, err error) {
			if err != nil {
				t.Error(err)
			}
			got = value
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Dispatcher().Post(func() {}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Dispatcher().Post(func() { t.Error("rejected action ran") }); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("saturated Post = %v", err)
	}
	close(release)
	settleTask(t, p, task)
	status := task.Snapshot()
	if got != 42 || status.State != TaskSucceeded || status.Delivery != DeliveryApplied {
		t.Fatalf("value=%d status=%+v", got, status)
	}
}

func TestTaskCancellationDoesNotInventOutcomeOrRevokeDelivery(t *testing.T) {
	for _, honors := range []bool{false, true} {
		name := "worker succeeds after cancellation"
		if honors {
			name = "worker reports cancellation"
		}
		t.Run(name, func(t *testing.T) {
			p, runtime := taskOwner(t)
			scope, err := runtime.NewScope(ScopeConfig{})
			if err != nil {
				t.Fatal(err)
			}
			defer scope.Close()
			var applied bool
			task, err := runtime.Start(scope, Work[int]{
				Run: func(ctx context.Context) (int, error) {
					<-ctx.Done()
					if honors {
						return 0, ctx.Err()
					}
					return 42, nil
				},
				Apply: func(value int, err error) {
					applied = true
					if honors {
						if !errors.Is(err, context.Canceled) {
							t.Errorf("outcome = %v", err)
						}
					} else if value != 42 || err != nil {
						t.Errorf("successful result = %d, %v", value, err)
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !task.Cancel() || task.Cancel() {
				t.Fatal("cancellation must be requested exactly once")
			}
			settleTask(t, p, task)
			want := TaskSucceeded
			if honors {
				want = TaskCancelled
			}
			status := task.Snapshot()
			if !status.CancelRequested || status.State != want || status.Delivery != DeliveryApplied || !applied {
				t.Fatalf("cancellation conflated independent facts: %+v, applied=%v", status, applied)
			}
			if scope.Context().Err() != nil || task.Cancel() {
				t.Fatal("task cancellation escaped its lifetime or changed a settled outcome")
			}
		})
	}
}

func TestScopeRevokesDeliveryWithoutInventingWorkerFailure(t *testing.T) {
	p, rt := taskOwner(t)
	scope, err := rt.NewScope(ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	task, err := rt.Start(scope, Work[int]{
		Run:   func(context.Context) (int, error) { <-release; return 7, nil },
		Apply: func(int, error) { t.Error("closed scope delivered a result") },
	})
	if err != nil {
		t.Fatal(err)
	}
	scope.Close()
	close(release)
	settleTask(t, p, task)
	<-task.Done()
	status := task.Snapshot()
	if status.State != TaskSucceeded || status.Delivery != DeliveryRevoked || !status.CancelRequested {
		t.Fatalf("cancellation conflated with result: %+v", status)
	}
}

func TestDeliveryRevocationDoesNotWaitForANoncooperativeWorker(t *testing.T) {
	_, rt := taskOwner(t)
	scope, err := rt.NewScope(ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	defer close(release)
	task, err := rt.Start(scope, Work[int]{Run: func(context.Context) (int, error) { <-release; return 1, nil }})
	if err != nil {
		t.Fatal(err)
	}
	scope.Close()
	select {
	case <-task.Settled():
	case <-time.After(5 * time.Second):
		t.Fatal("revocation waited for the worker")
	}
	status := task.Snapshot()
	if status.State != TaskRunning || status.Delivery != DeliveryRevoked {
		t.Fatalf("revocation invented completion: %+v", status)
	}
}

func TestTaskAdmissionRejectsBeforeLaunching(t *testing.T) {
	p, rt := taskOwner(t)
	p.tasks.resourceCapacity = 1
	scope, err := rt.NewScope(ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	if _, err := rt.Start(scope, Work[int]{Run: func(context.Context) (int, error) { t.Error("rejected worker ran"); return 0, nil }}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity = %v", err)
	}
	p.tasks.resourceCapacity = 2
	p.nextTask = math.MaxUint64
	for range 2 {
		if _, err := rt.Start(scope, Work[int]{Run: func(context.Context) (int, error) { t.Error("exhausted identity launched"); return 0, nil }}); err == nil {
			t.Fatal("task identity wrapped")
		}
	}
}

func TestInspectDoesNotAdvanceModelOrRequestAFrame(t *testing.T) {
	p, rt := taskOwner(t)
	before := p.root.Progress()
	done := make(chan error, 1)
	go func() { done <- rt.Dispatcher().Inspect(t.Context(), func() error { return nil }) }()
	select {
	case <-p.tasks.wake:
		p.runTasks()
	case <-time.After(5 * time.Second):
		t.Fatal("inspect was not admitted")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := p.root.Progress(); got != before {
		t.Fatalf("read advanced model: %+v -> %+v", before, got)
	}
	drawn, err := p.present.Present(time.Now(), func(bool) (uint64, error) { t.Error("read scheduled a frame"); return 0, nil })
	if drawn || err != nil {
		t.Fatalf("read changed presenter: %v, %v", drawn, err)
	}
}
