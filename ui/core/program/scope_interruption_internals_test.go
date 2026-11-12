package program

import (
	"runtime"
	"testing"
	"testing/synctest"
	"time"
)

func TestScopeCloseDoesNotBlockReentryFromExternalStop(t *testing.T) {
	owner, rt := taskOwner(t)
	owner.tasks.resourceCapacity = 1
	var scope *Scope
	external := &interruptedScopeContext{deferredCancellation: newDeferredCancellation()}
	external.stop = func() {
		closed := make(chan struct{})
		go func() { scope.Close(); close(closed) }()
		// A mutex wait cannot advance a synctest clock, so this guard uses real time.
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Error("external stop could not close its already closed scope")
		}
	}
	var err error
	scope, err = rt.NewScope(ScopeConfig{Context: external})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	scope.Close()
	replacement, err := rt.NewScope(ScopeConfig{})
	if err != nil {
		t.Fatalf("closed scope retained capacity: %v", err)
	}
	defer replacement.Close()
}

type interruptedScopeContext struct {
	*deferredCancellation
	register, stop func()
}

func (c *interruptedScopeContext) AfterFunc(callback func()) func() bool {
	if c.register != nil {
		c.register()
	}
	stop := c.deferredCancellation.AfterFunc(callback)
	return func() bool {
		stopped := stop()
		if c.stop != nil {
			c.stop()
		}
		return stopped
	}
}

func TestScopeReleasesCapacityWhenExternalObservationExits(t *testing.T) {
	for _, stage := range []string{"registration", "stop"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(stage+"/"+exit, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					owner, rt := taskOwner(t)
					owner.tasks.resourceCapacity = 1
					failure := "external context interrupted"
					interrupt := func() {
						if exit == "Goexit" {
							runtime.Goexit()
						}
						panic(failure)
					}
					external := &interruptedScopeContext{deferredCancellation: newDeferredCancellation()}
					if stage == "registration" {
						external.register = interrupt
					} else {
						external.stop = interrupt
					}
					done := make(chan struct{})
					go func() {
						defer close(done)
						defer func() {
							recovered := recover()
							if exit == "panic" && recovered != failure || exit == "Goexit" && recovered != nil {
								t.Errorf("external context exit = %v", recovered)
							}
						}()
						scope, err := rt.NewScope(ScopeConfig{Context: external})
						if err != nil {
							t.Error(err)
							return
						}
						scope.Close()
						t.Error("external context interruption returned")
					}()
					<-done
					synctest.Wait()
					replacement, err := rt.NewScope(ScopeConfig{})
					if err != nil {
						t.Fatalf("interrupted external observation retained scope capacity: %v", err)
					}
					defer replacement.Close()
				})
			})
		}
	}
}
