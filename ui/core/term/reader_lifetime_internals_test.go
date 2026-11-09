package term

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestCloseJoinsInputBeforeRestoringTheTerminal(t *testing.T) {
	const childEnv = "OOLONG_TEST_INPUT_LIFETIME"
	if os.Getenv(childEnv) != "1" {
		if !ptytest.Supported() {
			t.Skip("no pty on this platform")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		session, err := ptytest.Start(ctx, ptytest.Config{
			Env: append(os.Environ(), childEnv+"=1"),
		}, os.Args[0], "-test.run=^TestCloseJoinsInputBeforeRestoringTheTerminal$")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = session.Close() }()
		waitErr := session.Wait(ctx)
		drainErr := session.Drain(ctx)
		if waitErr != nil || drainErr != nil {
			t.Fatalf("input lifetime: wait=%v drain=%v\n%s", waitErr, drainErr, session.Transcript().String())
		}
		return
	}

	tty := newTerminal(os.Stdin, os.Stdout, Config{}, nil)
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
	if takeoverErr := tty.takeOver(); takeoverErr != nil {
		t.Fatal(takeoverErr)
	}
	rawState, err := xterm.GetState(tty.inFD)
	if err != nil {
		t.Fatal(err)
	}

	// Hold the real input claim before starting the reader. Shutdown must wait
	// for that owner; a wake request alone cannot transfer its descriptor or modes.
	tty.handed.mu.Lock()
	if err := tty.start(Config{}); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- tty.Close() }()
	<-tty.stop
	var premature bool
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close = %v", err)
		}
		premature = true
	case <-time.After(100 * time.Millisecond):
	}
	state, stateErr := xterm.GetState(tty.inFD)
	tty.handed.mu.Unlock()
	if !premature {
		if err := <-closed; err != nil {
			t.Errorf("Close = %v", err)
		}
	}
	if premature {
		t.Error("Close returned while its input reader still owned the descriptor")
	}
	if stateErr != nil || !reflect.DeepEqual(state, rawState) {
		t.Errorf("modes restored before input ended: state=%v err=%v", state, stateErr)
	}
	select {
	case <-tty.readerDone:
	default:
		t.Error("Close did not join the input reader")
	}
}
