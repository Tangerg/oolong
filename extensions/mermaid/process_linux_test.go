//go:build linux

package mermaid_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The force-killed launcher cannot reap its browser. A dedicated test process
// owns that orphan instead of depending on the host's PID 1 to reap within the
// renderer's shutdown budget. Subreaper status must not affect unrelated tests.
func runWithProcessReaper(t *testing.T) bool {
	t.Helper()
	const marker = "--mermaid-reaper"
	if slices.Contains(os.Args, marker) {
		if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	//nolint:gosec // G204: reexecute this test binary and one quoted test name.
	cmd := exec.CommandContext(ctx, executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.v", "--", marker)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("process reaper helper: %v\n%s", err, output)
	}
	return true
}

func startBrowserReaper(t *testing.T, pid int) func() {
	t.Helper()
	done := make(chan struct{})
	var reapErr error
	go func() {
		reapErr = reapBrowser(pid)
		close(done)
	}()
	return func() {
		t.Helper()
		<-done
		if reapErr != nil {
			t.Error(reapErr)
		}
	}
}

func reapBrowser(pid int) error {
	for {
		var status syscall.WaitStatus
		waited, err := syscall.Wait4(pid, &status, 0, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.ECHILD) {
			// Until the launcher exits, this known descendant is not yet ours
			// to reap. Never wait for arbitrary children and steal cmd.Wait's PID.
			time.Sleep(time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		if waited != pid || !status.Signaled() || status.Signal() != syscall.SIGKILL {
			return fmt.Errorf("browser reap: pid=%d status=%v, want %d killed", waited, status, pid)
		}
		return nil
	}
}

func TestBrowserTerminationDoesNotDependOnItsExternalReaper(t *testing.T) {
	if runWithProcessReaper(t) {
		return
	}
	call, pid := unresponsiveRender(t)
	call.cancel()
	<-call.done
	// A zombie has terminated; its external reaper must not control Render's
	// outcome. The wait status still proves that group cleanup delivered SIGKILL.
	if err := reapBrowser(pid); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(call.err, context.Canceled) || errors.Is(call.err, syscall.EPERM) ||
		strings.Contains(call.err.Error(), "did not finish shutdown") {
		t.Fatalf("unreaped browser shutdown=%v", call.err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("reaped browser %d still exists: %v", pid, err)
	}
}
