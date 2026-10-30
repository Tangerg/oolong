//go:build unix

package apiledger

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOSRunnerBoundsInheritedOutputAfterExitAndCancellation(t *testing.T) {
	for _, ending := range []string{"exit", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pidFile := filepath.Join(t.TempDir(), "holder")
			script := `sleep 30 & printf '%d' "$!" > "$OOLONG_TEST_HOLDER.tmp"; mv "$OOLONG_TEST_HOLDER.tmp" "$OOLONG_TEST_HOLDER"; printf 'owned output\n'; `
			if ending == "cancel" {
				script += "wait"
			}
			returned := make(chan error, 1)
			go func() {
				_, err := (osRunner{}).run(ctx, command{
					name: "sh", args: []string{"-c", script},
					env: map[string]string{"OOLONG_TEST_HOLDER": pidFile},
				})
				returned <- err
			}()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			deadline := time.After(5 * time.Second)
			var pid int
			for pid == 0 {
				// #nosec G304 -- the controlled helper publishes its PID in this test's temporary directory.
				data, err := os.ReadFile(pidFile)
				if err == nil {
					pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
					if err != nil {
						t.Fatal(err)
					}
					break
				}
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				select {
				case <-ticker.C:
				case <-deadline:
					t.Fatal("tool did not publish its output pipe holder")
				}
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
			})
			if ending == "cancel" {
				cancel()
			}
			select {
			case err := <-returned:
				if err == nil || ending == "exit" && !errors.Is(err, exec.ErrWaitDelay) {
					t.Fatalf("incomplete tool output = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("tool output kept the runner alive after process exit or cancellation")
			}
		})
	}
}
