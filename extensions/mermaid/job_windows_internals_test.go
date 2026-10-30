//go:build windows

package mermaid

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestJobTreeProcess(_ *testing.T) {
	index := slices.Index(os.Args, "--job-tree")
	if index < 0 {
		return
	}
	if os.Args[index+1] == "child" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	child := exec.Command(executable, "-test.run=^TestJobTreeProcess$", "--", "--job-tree", "child") //nolint:noctx,gosec // G204: this test executable deliberately spawns a child whose lifetime must be controlled by its job.
	if err := child.Start(); err != nil {
		os.Exit(3)
	}
	if _, err := fmt.Fprintln(os.Stdout, child.Process.Pid); err != nil {
		os.Exit(4)
	}
	if os.Args[index+1] == "exit" {
		os.Exit(0)
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestJobOwnsDescendantsOnCancellationAndParentExit(t *testing.T) {
	for _, mode := range []string{"wait", "exit"} {
		t.Run(mode, func(t *testing.T) {
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestJobTreeProcess$", "--", "--job-tree", mode) //nolint:gosec // G204: the test executable and fixed arguments are parent-owned.
			command.Dir = t.TempDir()
			command.Stdin = strings.NewReader("")
			output, writer := io.Pipe()
			command.Stdout = writer
			ready := make(chan struct{})
			var pidText string
			var readErr error
			go func() {
				pidText, readErr = bufio.NewReader(output).ReadString('\n')
				_ = output.Close()
				close(ready)
			}()
			done := make(chan struct{})
			var runErr error
			go func() {
				runErr = run(ctx, command)
				_ = writer.CloseWithError(runErr)
				close(done)
			}()
			t.Cleanup(func() { cancel(); <-done; <-ready })
			select {
			case <-ready:
				if readErr != nil {
					t.Fatalf("process ended before readiness: %v", readErr)
				}
			case <-ctx.Done():
				t.Fatal(context.Cause(ctx))
			}
			pid, err := strconv.ParseUint(strings.TrimSpace(pidText), 10, 32)
			if err != nil || pid == 0 {
				t.Fatalf("invalid child PID %q: %v", pidText, err)
			}
			handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
			if openErr != nil && mode != "exit" {
				t.Fatal(openErr)
			}
			if openErr == nil {
				defer func() {
					if closeErr := windows.CloseHandle(handle); closeErr != nil {
						t.Error(closeErr)
					}
				}()
			}
			if mode == "wait" {
				cancel()
			}
			<-done
			err = runErr
			if mode == "wait" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if mode == "exit" && err != nil {
				t.Fatal(err)
			}
			if openErr == nil {
				status, err := windows.WaitForSingleObject(handle, 0)
				if err != nil || status != windows.WAIT_OBJECT_0 {
					t.Fatalf("child remains alive: status=%d error=%v", status, err)
				}
			}
		})
	}
}
