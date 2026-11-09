//go:build unix

package term

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestProbeStartupFailureJoinsWorkersAndRestoresRawMode(t *testing.T) {
	const childEnv = "OOLONG_TEST_PROBE_STARTUP_FAILURE"
	if os.Getenv(childEnv) != "1" {
		if !ptytest.Supported() {
			t.Skip("no pty on this platform")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		session, err := ptytest.Start(ctx, ptytest.Config{Env: append(os.Environ(), childEnv+"=1")}, os.Args[0], "-test.run=^TestProbeStartupFailureJoinsWorkersAndRestoresRawMode$")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = session.Close() }()
		waitErr := session.Wait(ctx)
		drainErr := session.Drain(ctx)
		if waitErr != nil || drainErr != nil {
			t.Fatalf("failed probe startup: wait=%v drain=%v\n%s", waitErr, drainErr, session.Transcript().String())
		}
		return
	}

	cfg := Config{Features: Features{Probe: true}}
	tty := newTerminal(os.Stdin, os.Stdout, cfg, nil)
	var err error
	tty.inFD, err = checkedTerminalFD(os.Stdin, "input")
	if err != nil {
		t.Fatal(err)
	}
	tty.outFD, err = checkedTerminalFD(os.Stdout, "output")
	if err != nil {
		t.Fatal(err)
	}
	tty.oldState, err = xterm.MakeRaw(tty.inFD)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = xterm.Restore(tty.inFD, tty.oldState) }()
	if takeoverErr := tty.takeOver(); takeoverErr != nil {
		t.Fatal(takeoverErr)
	}

	// Fail the session's duplicate without closing the caller's PTY. The real
	// reader, writer and resize workers still need a completed shutdown.
	closeErr := tty.output.Close()
	startErr := tty.start(cfg)
	defer func() { _ = tty.Close() }()
	if closeErr != nil {
		t.Fatalf("close output duplicate: %v", closeErr)
	}
	if !errors.Is(startErr, os.ErrClosed) {
		t.Fatalf("startup lost its closed-output failure: %v", startErr)
	}
	for _, worker := range []struct {
		name string
		done <-chan struct{}
	}{
		{"input", tty.readerDone},
		{"resize", tty.resizeDone},
		{"pump", tty.pumpDone},
		{"task", tty.task.done},
		{"writer", tty.writer.loopDone},
	} {
		select {
		case <-worker.done:
		default:
			t.Errorf("startup failure returned before %s worker ended", worker.name)
		}
	}
	restored, stateErr := xterm.GetState(tty.inFD)
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	// Kernels may add transient flags on restoration, such as Darwin's PENDIN.
	// Capture the session's result before obtaining the platform's restored state.
	if restoreErr := xterm.Restore(tty.inFD, tty.oldState); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	wantState, err := xterm.GetState(tty.inFD)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, wantState) {
		t.Errorf("failed startup retained raw mode: state=%v want=%v", restored, wantState)
	}
	before := tty.Writer().Queued()
	if _, err := tty.Writer().Queue([]byte("after failed startup")); !errors.Is(err, ErrClosed) {
		t.Errorf("failed session accepted a frame: %v", err)
	}
	if tty.Writer().Queued() != before {
		t.Error("refused frame advanced writer admission")
	}
}
