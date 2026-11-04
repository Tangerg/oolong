package program_test

import (
	"context"
	"runtime"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTaskReceiptSurvivesApplyInterruption(t *testing.T) {
	failure := &struct{ message string }{message: "apply interrupted"}
	for _, exit := range []string{"return", "panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var task *program.Task
				var effect, unwound bool
				var recovered any
				screen, err := program.NewScreen(program.ScreenConfig{
					Host: newHost(t),
					Build: func(rt *program.Runtime) (scene.Element, error) {
						scope, err := rt.NewScope(program.ScopeConfig{})
						if err != nil {
							return scene.Element{}, err
						}
						task, err = rt.Start(scope, program.Work[bool]{
							Run: func(context.Context) (bool, error) { return true, nil },
							Apply: func(value bool, _ error) {
								defer scope.Close()
								defer func() { unwound = true }()
								effect = value
								select {
								case <-task.Settled():
									t.Error("delivery settled before Apply unwound")
								default:
								}
								switch exit {
								case "panic":
									panic(failure)
								case "Goexit":
									runtime.Goexit()
								default:
									rt.Quit()
								}
							},
						})
						return scene.NewElement(scene.ElementConfig{}), err
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				stopped := make(chan struct{})
				go func() {
					defer close(stopped)
					defer func() { recovered = recover() }()
					if err := screen.Run(t.Context()); err != nil {
						panic(err)
					}
				}()
				<-stopped
				if task == nil {
					t.Fatal("task was not accepted")
				}
				<-task.Settled()
				status := task.Snapshot()
				if !effect || !unwound || status.State != program.TaskSucceeded || status.Err != nil || status.Delivery != program.DeliveryDelivered {
					t.Fatalf("effect=%v, unwound=%v, receipt=%+v", effect, unwound, status)
				}
				if exit == "panic" && recovered != failure || exit != "panic" && recovered != nil {
					t.Fatalf("panic = %v", recovered)
				}
			})
		})
	}
}
