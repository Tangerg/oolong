package ptytest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// recorder catches an assertion's failure so a test can check that it fails when
// it should. An assertion nobody has seen fail is an assertion nobody knows is
// wired up.
type recorder struct{ failures []string }

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, _ ...any) {
	r.failures = append(r.failures, format)
}

func (r *recorder) failed() bool { return len(r.failures) > 0 }

func TestRequireContains(t *testing.T) {
	var ok recorder
	RequireContains(&ok, []byte("hello world"), "hello", "world")
	if ok.failed() {
		t.Fatal("failed on a transcript that has both")
	}

	var bad recorder
	RequireContains(&bad, []byte("hello"), "hello", "world")
	if !bad.failed() {
		t.Fatal("passed on a transcript missing a token")
	}
}

func TestRequireNotContains(t *testing.T) {
	// How the interesting half of a renderer's promises are stated: what an idle
	// frame must not write.
	var ok recorder
	RequireNotContains(&ok, []byte("\x1b[0m"), "\x1b[?2026h")
	if ok.failed() {
		t.Fatal("failed on a transcript that is clean")
	}

	var bad recorder
	RequireNotContains(&bad, []byte("frame \x1b[2J here"), "\x1b[2J")
	if !bad.failed() {
		t.Fatal("passed on a transcript containing what it forbade")
	}
}

func TestRequireOrdered(t *testing.T) {
	var ok recorder
	RequireOrdered(&ok, []byte("first then second then third"), "first", "second", "third")
	if ok.failed() {
		t.Fatal("failed on tokens that are in order")
	}

	var reversed recorder
	RequireOrdered(&reversed, []byte("second then first"), "first", "second")
	if !reversed.failed() {
		t.Fatal("passed on tokens that are out of order")
	}

	var absent recorder
	RequireOrdered(&absent, []byte("only the first"), "first", "second")
	if !absent.failed() {
		t.Fatal("passed on a token that is not there at all")
	}
}

func TestRequireOrderedDoesNotMatchOneTokenTwice(t *testing.T) {
	// "a" then "a" needs two of them, not one found twice.
	var r recorder
	RequireOrdered(&r, []byte("a"), "a", "a")
	if !r.failed() {
		t.Fatal("one occurrence satisfied two tokens")
	}
}

// The modes a session actually turns on, in the order term.modes uses.
var (
	altScreen = Mode{Name: "alternate screen", On: "\x1b[?1049h", Off: "\x1b[?1049l"}
	mouse     = Mode{Name: "mouse", On: "\x1b[?1003h", Off: "\x1b[?1003l"}
)

func TestRequireSymmetricModes(t *testing.T) {
	// On in order, off in reverse: a session that left the alternate screen before
	// it released the mouse released the mouse of a screen that was already gone.
	good := altScreen.On + mouse.On + "frame" + mouse.Off + altScreen.Off

	var ok recorder
	RequireSymmetricModes(&ok, []byte(good), altScreen, mouse)
	if ok.failed() {
		t.Fatalf("failed on a session that unwound properly: %v", ok.failures)
	}
}

func TestRequireSymmetricModesCatchesAModeLeftOn(t *testing.T) {
	// The failure that matters: a terminal left in a mode nobody turned off is a
	// terminal the user has to close.
	leaked := altScreen.On + mouse.On + "frame" + mouse.Off

	var r recorder
	RequireSymmetricModes(&r, []byte(leaked), altScreen, mouse)
	if !r.failed() {
		t.Fatal("passed on a session that never left the alternate screen")
	}
}

func TestRequireSymmetricModesCatchesUnwindingInTheWrongOrder(t *testing.T) {
	wrong := altScreen.On + mouse.On + "frame" + altScreen.Off + mouse.Off

	var r recorder
	RequireSymmetricModes(&r, []byte(wrong), altScreen, mouse)
	if !r.failed() {
		t.Fatal("passed on a session unwound in the order it was set up")
	}
}

func TestRequireSymmetricModesCatchesAModeSetTwice(t *testing.T) {
	twice := altScreen.On + altScreen.On + altScreen.Off

	var r recorder
	RequireSymmetricModes(&r, []byte(twice), altScreen)
	if !r.failed() {
		t.Fatal("passed on a mode turned on twice")
	}
}

func TestRequireSymmetricModesCatchesOffBeforeOn(t *testing.T) {
	backwards := altScreen.Off + "frame" + altScreen.On

	var r recorder
	RequireSymmetricModes(&r, []byte(backwards), altScreen)
	if !r.failed() {
		t.Fatal("passed on a mode turned off before it was turned on")
	}
}

func TestATranscriptIsSafeToReadWhileItGrows(t *testing.T) {
	tr := newTranscript()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 200 {
			tr.append([]byte{byte('a' + i%26)})
		}
	}()
	for range 200 {
		_ = tr.Bytes()
	}
	<-done
	if len(tr.Bytes()) != 200 {
		t.Fatalf("captured %d bytes, want 200", len(tr.Bytes()))
	}
}

func TestWaitForReturnsOnceEveryTokenHasArrived(t *testing.T) {
	tr := newTranscript()
	go func() {
		time.Sleep(10 * time.Millisecond)
		tr.append([]byte("first "))
		time.Sleep(10 * time.Millisecond)
		tr.append([]byte("second"))
	}()
	if err := tr.WaitWithin(2*time.Second, "first", "second"); err != nil {
		t.Fatalf("waiting for both: %v", err)
	}
}

func TestWaitForGivesUpAndSaysWhatItNeverSaw(t *testing.T) {
	tr := newTranscript()
	tr.append([]byte("only this"))

	err := tr.WaitWithin(20*time.Millisecond, "only", "missing")
	if err == nil {
		t.Fatal("waiting for something that never came did not fail")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("the failure was %q, want it to name the token that never came", err)
	}
	if strings.Contains(err.Error(), `"only"`) {
		t.Fatalf("the failure was %q, want it to name only what was missing", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the failure does not unwrap to a deadline: %v", err)
	}
}

func TestSizesThatATerminalCannotReportAreRefused(t *testing.T) {
	// Zero is refused at the door: a terminal collapsed to no size is a state a
	// test never means to ask for and always struggles to diagnose.
	for _, bad := range []Size{{0, 24}, {80, 0}, {-1, 24}, {1 << 20, 24}} {
		if err := bad.check(); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	if err := (Size{80, 24}).check(); err != nil {
		t.Errorf("a normal size was refused: %v", err)
	}
}
