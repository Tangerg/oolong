//go:build linux || darwin || freebsd

package term_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/term"
	"golang.org/x/sys/unix"
)

const suspendTestMode = "OOLONG_SUSPEND_TEST_MODE"

func TestSuspendUsesTheOperatingSystemsJobControl(t *testing.T) {
	for _, mode := range []string{"default", "orphan", "ignored", "handled", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSuspendChildProcess$") //nolint:gosec // G204: this test binary and fixed child-selection arguments are fixture-owned.
			child.Env = append(os.Environ(), suspendTestMode+"="+mode)
			child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if mode == "orphan" {
				child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			}
			reader, writer, pipeErr := os.Pipe()
			if pipeErr != nil {
				t.Fatal(pipeErr)
			}
			defer func() { _ = reader.Close(); _ = writer.Close() }()
			child.ExtraFiles = []*os.File{writer}
			var output strings.Builder
			child.Stdout, child.Stderr = &output, &output
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill() }()
			_ = writer.Close()
			if mode == "default" {
				var status unix.WaitStatus
				pid, waitErr := unix.Wait4(child.Process.Pid, &status, unix.WUNTRACED, nil)
				if waitErr != nil || pid != child.Process.Pid || !status.Stopped() || status.StopSignal() != unix.SIGTSTP {
					_ = child.Wait()
					t.Fatalf("default stop: pid=%d status=%v error=%v output=%s", pid, status, waitErr, &output)
				}
				if err := reader.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				var marker [1]byte
				if n, err := reader.Read(marker[:]); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
					t.Errorf("Suspend returned before continuation: %q, %v", marker[:n], err)
				}
				if err := reader.SetReadDeadline(time.Time{}); err != nil {
					t.Fatal(err)
				}
				if err := child.Process.Signal(unix.SIGCONT); err != nil {
					t.Fatal(err)
				}
			}
			if err := child.Wait(); err != nil {
				t.Fatalf("child: %v, context: %v, output: %s", err, ctx.Err(), &output)
			}
			marker, err := io.ReadAll(reader)
			if err != nil || string(marker) != "returned" {
				t.Errorf("completion = %q, error = %v", marker, err)
			}
		})
	}
}

func TestSuspendChildProcess(t *testing.T) {
	mode := os.Getenv(suspendTestMode)
	if mode == "" {
		return
	}
	var notified chan os.Signal
	switch mode {
	case "ignored":
		signal.Ignore(unix.SIGTSTP)
	case "handled":
		notified = make(chan os.Signal, 1)
		signal.Notify(notified, unix.SIGTSTP)
		defer signal.Stop(notified)
	case "blocked":
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		restore := blockSuspendSignal(t)
		defer restore()
	}
	for range 2 {
		err := term.Suspend()
		if mode == "blocked" {
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Fatalf("blocked Suspend = %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		if notified != nil {
			select {
			case <-notified:
			case <-time.After(time.Second):
				t.Fatal("Suspend replaced the host's signal handler")
			}
		}
		if mode == "default" {
			break
		}
	}
	report := os.NewFile(3, "suspend completion")
	if report == nil {
		t.Fatal("missing completion pipe")
	}
	defer func() { _ = report.Close() }()
	if _, err := fmt.Fprint(report, "returned"); err != nil {
		t.Fatal(err)
	}
}
