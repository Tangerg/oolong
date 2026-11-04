package program

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"testing/synctest"
)

type exitingPanicValue struct{}

func (exitingPanicValue) String() string {
	runtime.Goexit()
	return ""
}

func TestWorkerReceiptSurvivesPanicFormattingExit(t *testing.T) {
	for _, delivery := range []struct {
		name  string
		apply bool
	}{
		{name: "without apply"},
		{name: "with apply", apply: true},
	} {
		t.Run(delivery.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, owner := taskOwner(t)
				p.tasks.resourceCapacity = 2
				scope, err := owner.NewScope(ScopeConfig{})
				if err != nil {
					t.Fatal(err)
				}
				defer scope.Close()
				work := Work[int]{Run: func(context.Context) (int, error) { panic(exitingPanicValue{}) }}
				var applied bool
				if delivery.apply {
					work.Apply = func(_ int, err error) {
						applied = true
						if !errors.Is(err, ErrTaskAborted) {
							t.Errorf("interrupted worker delivered %v", err)
						}
					}
				}
				task, err := owner.Start(scope, work)
				if err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				p.runTasks()
				synctest.Wait()
				select {
				case <-task.Done():
				default:
					t.Error("exited worker left its accepted receipt running")
				}
				select {
				case <-task.Settled():
				default:
					t.Error("exited worker left its delivery unresolved")
				}
				status := task.Snapshot()
				wantDelivery := DeliveryRevoked
				if delivery.apply {
					wantDelivery = DeliveryDelivered
				}
				if status.State != TaskFailed || status.Delivery != wantDelivery || !errors.Is(status.Err, ErrTaskAborted) || applied != delivery.apply {
					t.Errorf("interrupted worker outcome=%+v applied=%v", status, applied)
				}
				if _, err := owner.Start(scope, Work[int]{Run: func(context.Context) (int, error) { return 0, nil }}); err != nil {
					t.Errorf("exited worker retained its reserved capacity: %v", err)
				}
				synctest.Wait()
			})
		})
	}
}
