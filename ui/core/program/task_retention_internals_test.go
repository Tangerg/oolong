package program

import (
	"context"
	"runtime"
	"testing"
	"testing/synctest"
	"weak"
)

func TestRevokedTaskReleasesItsQueuedResultWithoutAnOwnerTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner, rt := taskOwner(t)
		scope, err := rt.NewScope(ScopeConfig{})
		if err != nil {
			t.Fatal(err)
		}
		defer scope.Close()
		task, result := func() (*Task, weak.Pointer[[64]byte]) {
			value := new([64]byte)
			reference := weak.Make(value)
			task, startErr := rt.Start(scope, Work[*[64]byte]{
				Run: func(context.Context) (*[64]byte, error) { return value, nil },
				Apply: func(*[64]byte, error) {
					t.Error("revoked task delivered its result")
				},
			})
			if startErr != nil {
				t.Fatal(startErr)
			}
			return task, reference
		}()
		synctest.Wait()
		owner.tasks.mu.Lock()
		queued := owner.tasks.tasks.Len()
		owner.tasks.mu.Unlock()
		if queued != 1 {
			t.Fatalf("queued completions = %d, want 1", queued)
		}
		scope.Close()
		synctest.Wait()
		if status := task.Snapshot(); status.State != TaskSucceeded || status.Delivery != DeliveryRevoked || status.Err != nil || status.CancelRequested {
			t.Fatalf("revocation changed the completed worker outcome: %+v", status)
		}
		runtime.GC()
		retained := result.Value() != nil
		runtime.KeepAlive(task)
		runtime.KeepAlive(owner)
		if retained {
			t.Fatal("revoked completion retained its result until another owner turn")
		}
		owner.runTasks()
	})
}
