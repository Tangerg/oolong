// Package streamtask joins bounded byte delivery with a scoped producer in the examples.
package streamtask

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/program"
)

// Start transfers the producer's outcome to its ingress after every accepted byte.
// Delivery to the UI may be revoked, so closure follows task completion independently.
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
