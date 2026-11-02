// Package streamtask keeps producer completion independent of UI admission so
// revoked delivery cannot leave its ingress open.
package streamtask

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/program"
)

// Start closes ingress from task completion because UI delivery may already be
// revoked; the producer's outcome must still follow every accepted byte.
func Start(runtime *program.Runtime, scope *program.Scope, ingress *program.ByteIngress, produce func(context.Context) error) *program.Task {
	task, err := runtime.Start(scope, program.Work[struct{}]{
		Run: func(ctx context.Context) (struct{}, error) { return struct{}{}, produce(ctx) },
	})
	if err != nil {
		_ = ingress.CloseWithError(err)
		return nil
	}
	go func() {
		<-task.Done()
		_ = ingress.CloseWithError(task.Snapshot().Err)
	}()
	return task
}
