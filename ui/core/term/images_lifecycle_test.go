package term_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/term"
)

func pendingUpload(t *testing.T, tty *term.Terminal, primary *os.File) (<-chan graphics.UploadResult, string) {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	result, err := tty.Transmit(t.Context(), data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if drainErr := tty.Writer().Drain(time.Second); drainErr != nil {
		t.Fatal(drainErr)
	}
	upload := read(t, primary, 50*time.Millisecond)
	_, tail, found := strings.Cut(upload, "a=t,f=100,I=")
	number, _, separated := strings.Cut(tail, ",")
	if !found || !separated {
		t.Fatalf("terminal did not receive the complete upload: %q", upload)
	}
	return result, number
}

func acknowledgeUpload(t *testing.T, primary *os.File, number string, id uint32) {
	t.Helper()
	if _, err := fmt.Fprintf(primary, "\x1b_Gi=%d,I=%s;OK\x1b\\", id, number); err != nil {
		t.Fatal(err)
	}
}

func TestPendingImageRefusesHandoverWithoutLosingTerminalOwnership(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")
	tty, primary := open(t, term.Config{AltScreen: true})
	result, number := pendingUpload(t, tty, primary)
	called := false
	err := tty.Hand(nil, func() error { called = true; return nil })
	if called || !errors.Is(err, term.ErrImageCleanupIncomplete) {
		t.Fatalf("unacknowledged allocation handed off: child=%t err=%v", called, err)
	}
	select {
	case outcome := <-result:
		t.Fatalf("refused handover revoked the outstanding request: %+v", outcome)
	default:
	}
	if _, queueErr := tty.Writer().Queue([]byte("still-owned")); queueErr != nil {
		t.Fatal("handover refusal did not restore ordinary admission:", queueErr)
	}
	if output := read(t, primary, 50*time.Millisecond); !strings.Contains(output, "still-owned") || strings.Contains(output, "\x1b[?1049l") {
		t.Fatalf("refused handover changed terminal ownership: %q", output)
	}
	acknowledgeUpload(t, primary, number, 77)
	outcome := <-result
	if outcome.Err != nil || !outcome.Resource.Live() {
		t.Fatalf("remaining reader did not settle the allocation: %+v", outcome)
	}
	if handErr := tty.Hand(nil, func() error {
		output := read(t, primary, 50*time.Millisecond)
		deletion := "a=d,d=I,i=77,"
		if strings.Count(output, deletion) != 1 || strings.Index(output, deletion) > strings.Index(output, "\x1b[?1049l") {
			t.Errorf("child did not receive completed exact cleanup: %q", output)
		}
		return nil
	}); handErr != nil {
		t.Fatal(handErr)
	}
	if outcome.Resource.Live() {
		t.Fatal("handed-over allocation remained live")
	}
}

func TestCloseDeletesAnImageAcknowledgedDuringShutdown(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")
	tty, primary := open(t, term.Config{})
	result, number := pendingUpload(t, tty, primary)
	closed := make(chan error, 1)
	go func() { closed <- tty.Close() }()
	if outcome := <-result; !errors.Is(outcome.Err, term.ErrClosed) {
		t.Fatalf("shutdown did not cancel the upload result: %+v", outcome)
	}
	acknowledgeUpload(t, primary, number, 77)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	output := read(t, primary, 50*time.Millisecond)
	if strings.Count(output, "a=d,d=I,i=77,") != 1 || strings.Count(output, "a=d") != 1 {
		t.Fatalf("shutdown lost or guessed an allocation: %q", output)
	}
}

func TestUnconfirmedImageCleanupStillRestoresTerminalOnClose(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")
	primary, replica := pty(t)
	fd := int(replica.Fd())
	original, err := xterm.GetState(fd)
	if err != nil {
		t.Fatal(err)
	}
	tty, err := term.OpenOn(replica, replica, term.Config{AltScreen: true}, os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	result, _ := pendingUpload(t, tty, primary)
	closeErr := tty.Close()
	if !errors.Is(closeErr, term.ErrImageCleanupIncomplete) {
		t.Fatalf("unconfirmed cleanup was reported as complete: %v", closeErr)
	}
	if outcome := <-result; !errors.Is(outcome.Err, term.ErrClosed) {
		t.Fatal(outcome.Err)
	}
	state, err := xterm.GetState(fd)
	if err != nil || !reflect.DeepEqual(state, original) {
		t.Fatalf("cleanup uncertainty prevented raw-mode restoration: state=%v err=%v", state, err)
	}
	if output := read(t, primary, 50*time.Millisecond); !strings.Contains(output, "\x1b[?1049l") || strings.Contains(output, "a=d") {
		t.Fatalf("shutdown guessed an image identity or skipped mode restoration: %q", output)
	}
	if next := tty.Close(); !errors.Is(next, closeErr) {
		t.Fatalf("repeated Close changed its outcome: %v, %v", closeErr, next)
	}
}

func TestCloseSupersedesAHandoverWaitingForAnImage(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "1")
	tty, primary := open(t, term.Config{})
	result, number := pendingUpload(t, tty, primary)
	prepared := make(chan struct{})
	handed := make(chan error, 1)
	called := false
	go func() {
		handed <- tty.Hand(func() error { close(prepared); return nil }, func() error { called = true; return nil })
	}()
	<-prepared
	closed := make(chan error, 1)
	go func() { closed <- tty.Close() }()
	if outcome := <-result; !errors.Is(outcome.Err, term.ErrClosed) {
		t.Fatal(outcome.Err)
	}
	acknowledgeUpload(t, primary, number, 77)
	if closeErr := <-closed; closeErr != nil {
		t.Fatal(closeErr)
	}
	if handErr := <-handed; called || !errors.Is(handErr, term.ErrClosed) {
		t.Fatalf("closing terminal started a child: called=%t err=%v", called, handErr)
	}
	if output := read(t, primary, 50*time.Millisecond); strings.Count(output, "a=d,d=I,i=77,") != 1 {
		t.Fatalf("concurrent cleanup did not delete the allocation once: %q", output)
	}
}
