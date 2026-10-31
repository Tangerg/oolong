package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestCommandOutputDoesNotOutliveItsProcessOwner(t *testing.T) {
	for _, ending := range []string{"exit", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			host := uitest.New(t, uitest.Config{Width: 40, Height: 4})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ready := make(chan *program.Task, 1)
			programDone := make(chan error, 1)
			go func() {
				app, err := program.NewInline(program.InlineConfig{Host: host, Build: func(runtime *program.InlineRuntime) (scene.Element, error) { //nolint:contextcheck // Run(ctx) parents this scope; cancellation through the runtime is under test.
					r := &runner{
						runtime: runtime,
						command: []string{os.Args[0], "-test.run=^TestCommandPipeHolder$", "--", ending},
					}
					scope, err := runtime.NewScope(program.ScopeConfig{})
					if err != nil {
						return scene.Element{}, err
					}
					task, err := r.start(scope)
					ready <- task
					if err != nil {
						return scene.Element{}, err
					}
					return r.Element(), nil
				}})
				if err == nil {
					err = app.Run(ctx)
				}
				programDone <- err
			}()
			t.Cleanup(func() {
				cancel()
				if err := <-programDone; err != nil {
					t.Errorf("run output program: %v", err)
				}
			})
			task := <-ready
			sourceDone := task.Done()
			var pid int
			host.Until(t, "child to announce its pipe holder", func() bool {
				_, tail, found := strings.Cut(host.Frames(), "retained-stdout:")
				if !found {
					return false
				}
				_, err := fmt.Sscanf(tail, "%d", &pid)
				return err == nil
			})
			child, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := child.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					t.Errorf("stop the test pipe holder: %v", err)
				}
				if err := child.Release(); err != nil {
					t.Errorf("release the test pipe holder: %v", err)
				}
				select {
				case <-sourceDone:
				case <-time.After(5 * time.Second):
					t.Error("command source did not stop after pipe-holder cleanup")
				}
			})
			if ending == "cancel" {
				cancel()
			}
			select {
			case <-sourceDone:
			case <-time.After(3 * time.Second):
				t.Fatal("command source kept waiting for a pipe its process no longer owns")
			}
			if ending == "exit" {
				if err := task.Snapshot().Err; !errors.Is(err, exec.ErrWaitDelay) {
					t.Fatalf("incomplete output = %v, want exec.ErrWaitDelay", err)
				}
			}
		})
	}
}

func TestCommandPipeHolder(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "hold" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	//nolint:gosec // G204: this controlled test subprocess retains the inherited pipe.
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCommandPipeHolder$", "--", "hold")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("retained-stdout:%d\n", child.Process.Pid)
	if mode == "cancel" {
		_ = child.Wait()
	}
	os.Exit(0)
}
