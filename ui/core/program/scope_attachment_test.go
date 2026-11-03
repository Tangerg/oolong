package program_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestScopeAttachmentSeparatesTheSessionFromAnExplicitMount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		child := scene.NewElement(scene.ElementConfig{})
		group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: child}}})
		ready := make(chan *program.Runtime, 1)
		startOn(t, newHost(t), func(runtime *program.Runtime) (scene.Element, error) {
			ready <- runtime
			return group.Element(), nil
		})
		runtime := <-ready
		var mountedTask, sessionTask *program.Task
		var mountedApplied, sessionApplied bool
		runErr := runtime.Dispatcher().Invoke(t.Context(), func() error {
			empty := scene.Attachment{}
			invalid, err := runtime.NewScope(program.ScopeConfig{Attachment: &empty})
			if invalid != nil {
				invalid.Close()
				return errors.New("an explicit empty attachment acquired a scope")
			}
			if !errors.Is(err, program.ErrScopeClosed) {
				return errors.Join(errors.New("empty attachment must return ErrScopeClosed"), err)
			}
			session, err := runtime.NewScope(program.ScopeConfig{})
			if err != nil {
				return err
			}
			lease := child.Attachment()
			mounted, err := runtime.NewScope(program.ScopeConfig{Attachment: &lease})
			if err != nil {
				return err
			}
			lease = scene.Attachment{}
			mountedTask, err = runtime.Start(mounted, program.Work[bool]{
				Run:   func(context.Context) (bool, error) { return true, nil },
				Apply: func(bool, error) { mountedApplied = true },
			})
			if err != nil {
				return fmt.Errorf("changing the caller's lease copy revoked the mount: %w", err)
			}
			if err = group.Set(); err != nil {
				return err
			}
			_, err = runtime.Start(mounted, program.Work[bool]{Run: func(context.Context) (bool, error) { return true, nil }})
			if !errors.Is(err, program.ErrScopeClosed) {
				return errors.Join(errors.New("start after detachment must return ErrScopeClosed"), err)
			}
			sessionTask, err = runtime.Start(session, program.Work[bool]{
				Run:   func(context.Context) (bool, error) { return true, nil },
				Apply: func(value bool, workErr error) { sessionApplied = value && workErr == nil },
			})
			return err
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		<-mountedTask.Done()
		<-mountedTask.Settled()
		<-sessionTask.Settled()
		if status := mountedTask.Snapshot(); mountedApplied || status.State != program.TaskSucceeded || status.Delivery != program.DeliveryRevoked {
			t.Fatalf("detached task = %+v, applied = %v", status, mountedApplied)
		}
		if status := sessionTask.Snapshot(); !sessionApplied || status.Delivery != program.DeliveryApplied {
			t.Fatalf("session task = %+v, applied = %v", status, sessionApplied)
		}
	})
}
