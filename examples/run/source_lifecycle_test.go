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

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func TestCommandOutputDoesNotOutliveItsProcessOwner(t *testing.T) {
	for _, ending := range []string{"exit", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			host := programtest.New(t, programtest.Config{Width: 40, Height: 4})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ready := make(chan *runner, 1)
			programDone := make(chan error, 1)
			childPID := make(chan int, 1)
			final := make(chan error, 1)
			go func() {
				programDone <- program.Run(ctx, program.Config{
					Host: host,
					Inline: func(runtime *program.InlineRuntime) program.Component {
						r := &runner{
							runtime: runtime,
							command: []string{os.Args[0], "-test.run=^TestCommandPipeHolder$", "--", ending},
						}
						var output strings.Builder
						announced := false
						var err error
						r.ingress, err = program.NewByteIngress(program.ByteIngressConfig{
							Dispatcher: runtime.Dispatcher(), Limit: 64 << 10,
							Consume: func(batch program.ByteBatch) {
								output.Write(batch.Data)
								if !announced && strings.Contains(output.String(), "\n") {
									var pid int
									if _, parseErr := fmt.Sscanf(output.String(), "retained-stdout:%d\n", &pid); parseErr == nil {
										announced = true
										childPID <- pid
									}
								}
								if batch.Final {
									final <- batch.Err
								}
							},
						})
						if err != nil {
							panic(err)
						}
						ready <- r
						return r
					},
				})
			}()
			t.Cleanup(func() {
				cancel()
				if err := <-programDone; err != nil {
					t.Errorf("run output program: %v", err)
				}
			})
			r := <-ready
			sourceDone := make(chan struct{})
			go func() {
				r.start()
				close(sourceDone)
			}()
			var pid int
			select {
			case pid = <-childPID:
			case <-time.After(5 * time.Second):
				t.Fatal("command did not announce its pipe holder")
			}
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
				if err := <-final; !errors.Is(err, exec.ErrWaitDelay) {
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
