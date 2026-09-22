//go:build windows

package mermaid

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	ready := os.Args[index+2]
	if err := os.WriteFile(ready+".tmp", []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil { //nolint:gosec // G703: the test parent supplies its own temporary readiness path.
		os.Exit(4)
	}
	if err := os.Rename(ready+".tmp", ready); err != nil { //nolint:gosec // G703: both readiness paths are owned by the test parent.
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
			ready := filepath.Join(t.TempDir(), "child.pid")
			command := exec.CommandContext(ctx, executable, "-test.run=^TestJobTreeProcess$", "--", "--job-tree", mode, ready) //nolint:gosec // G204: the test executable and fixed arguments are parent-owned.
			command.Dir = t.TempDir()
			command.Stdin = strings.NewReader("")
			command.Stdout = new(bytes.Buffer)
			done := make(chan struct{})
			var runErr error
			go func() { runErr = run(ctx, command); close(done) }()
			t.Cleanup(func() { cancel(); <-done })
			var pid uint64
			for pid == 0 {
				data, readErr := os.ReadFile(ready) //nolint:gosec // G304: readiness file is owned by this test.
				if readErr == nil {
					pid, err = strconv.ParseUint(string(data), 10, 32)
					if err != nil {
						t.Fatal(err)
					}
					break
				}
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal(readErr)
				}
				select {
				case <-done:
					if runErr != nil {
						t.Fatalf("process ended before readiness: %v", runErr)
					}
					// Normal parent exit can win the poll after publishing its PID.
					if _, readyErr := os.Stat(ready); readyErr != nil {
						t.Fatalf("process exited without readiness: %v", readyErr)
					}
				case <-ctx.Done():
					t.Fatal(context.Cause(ctx))
				case <-time.After(time.Millisecond):
				}
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
