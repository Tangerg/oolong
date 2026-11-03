package term

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/input"
)

func uploadPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func uploadTerminal(t *testing.T, destination io.Writer) *Terminal {
	t.Helper()
	terminal := &Terminal{writer: NewWriter(WriterConfig{Destination: destination}), imageProtocol: graphics.Kitty}
	t.Cleanup(func() {
		terminal.cancelUploads(ErrClosed)
		terminal.modeMu.Lock()
		terminal.revokeUploads()
		terminal.modeMu.Unlock()
		_ = terminal.writer.Close()
	})
	return terminal
}

func uploadOutcome(t *testing.T, result <-chan graphics.UploadResult, want error) graphics.Resource {
	t.Helper()
	outcome, ok := <-result
	if !ok || !errors.Is(outcome.Err, want) {
		t.Fatalf("outcome = %+v, open = %t; want %v", outcome, ok, want)
	}
	if _, open := <-result; open {
		t.Fatal("upload settled more than once")
	}
	return outcome.Resource
}

func TestInlineUploadRequestsAFreePhysicalIdentity(t *testing.T) {
	var wire bytes.Buffer
	data := uploadPNG(t)
	foreign, err := graphics.Transmit(&wire, 1, data)
	if err != nil {
		t.Fatal(err)
	}
	if paintErr := foreign.Placement(1).Paint(&wire, image.Pt(1, 1), -1); paintErr != nil {
		t.Fatal(paintErr)
	}
	prior := wire.Len()
	terminal := uploadTerminal(t, &wire)
	result, err := terminal.Transmit(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminal.writer.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	sequence := wire.String()[prior:]
	if !strings.Contains(sequence, "a=t,f=100,I=") || strings.Contains(sequence, "i=1") || strings.Contains(sequence, "q=2") {
		t.Fatalf("upload overwrites an existing physical image or hides its reply: %q", sequence)
	}
	select {
	case outcome := <-result:
		t.Fatalf("writer admission claimed terminal creation: %+v", outcome)
	default:
	}
	if consumed, err := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=99,I=%d;OK", pendingImageNumber(t, terminal))}); !consumed || err != nil {
		t.Fatal("creation response not consumed")
	}
	resource := uploadOutcome(t, result, nil)
	if resource.ID() != 99 || !resource.Live() || resource.Size() != image.Pt(1, 1) {
		t.Fatalf("resource did not use terminal allocation: %+v", resource)
	}
	if err := terminal.ReleaseImage(resource); err != nil {
		t.Fatal(err)
	}
	if err := terminal.writer.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	if output := wire.String()[prior:]; strings.Contains(output, "a=d,d=I,i=1,") || !strings.Contains(output, "a=d,d=I,i=99,") {
		t.Fatalf("cleanup addressed the foreign image: %q", output)
	}
}

func TestImageCreationAdmissionCannotWaitForAnInputConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		terminal := uploadTerminal(t, io.Discard)
		raw := make(chan readResult, 2)
		out := make(chan input.Event)
		stop := make(chan struct{})
		pumpDone := make(chan struct{})
		p := &pump{raw: raw, out: out, stop: stop, stream: &input.Stream{}, consume: terminal.takeImageReply}
		go func() { _ = p.run(); close(pumpDone) }()
		defer func() { close(stop); <-pumpDone }()
		raw <- readResult{data: []byte("x"), at: time.Now()}
		synctest.Wait()
		result, err := terminal.Transmit(t.Context(), uploadPNG(t))
		if err != nil {
			t.Fatal(err)
		}
		raw <- readResult{data: []byte(fmt.Sprintf("\x1b_Gi=42,I=%d;OK\x1b\\", pendingImageNumber(t, terminal))), at: time.Now()}
		<-out
		if resource := uploadOutcome(t, result, nil); resource.ID() != 42 {
			t.Fatal("pump failed to settle terminal-created image")
		}
	})
}

func TestImageRequestsFailExplicitlyAndRetainNoInventedResource(t *testing.T) {
	for _, test := range []struct {
		body string
		want error
	}{
		{"Gi=0,I=1;EINVAL:decoder refused PNG", graphics.ErrRejected},
		{"Gi=broken,I=1;OK", graphics.ErrMalformedReply},
		{"Gi=42,I=1;", graphics.ErrMalformedReply},
		{"Gi=42,I=1,p=3;OK", graphics.ErrMalformedReply},
	} {
		t.Run(test.body, func(t *testing.T) {
			terminal := uploadTerminal(t, io.Discard)
			result, err := terminal.Transmit(t.Context(), uploadPNG(t))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := terminal.takeImageReply(input.APC{Body: strings.Replace(test.body, "I=1", fmt.Sprintf("I=%d", pendingImageNumber(t, terminal)), 1)}); err != nil {
				t.Fatal(err)
			}
			if resource := uploadOutcome(t, result, test.want); resource.Live() {
				t.Fatal("failed image became a live resource")
			}
		})
	}
}

func TestCancelledUploadsRemainBoundedAndCleanUpLateAllocations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var wire bytes.Buffer
		terminal := uploadTerminal(t, &wire)
		data := uploadPNG(t)
		var firstNumber uint32
		for range maxImageUploads {
			ctx, cancel := context.WithCancel(t.Context())
			result, err := terminal.Transmit(ctx, data)
			if err != nil {
				t.Fatal(err)
			}
			if firstNumber == 0 {
				firstNumber = pendingImageNumber(t, terminal)
			}
			cancel()
			uploadOutcome(t, result, context.Canceled)
			if err := terminal.writer.Drain(time.Second); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := terminal.Transmit(t.Context(), data); !errors.Is(err, ErrImageUploadCapacity) {
			t.Fatalf("unanswered retention escaped its bound: %v", err)
		}
		if _, err := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=77,I=%d;OK", firstNumber)}); err != nil {
			t.Fatal(err)
		}
		if err := terminal.writer.Drain(time.Second); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(wire.String(), "a=d,d=I,i=77,") || terminal.imageOwner.Contains(77) {
			t.Fatal("cancelled image was not deleted by its acknowledged physical ID")
		}
		result, err := terminal.Transmit(t.Context(), data)
		if err != nil {
			t.Fatal("settled correlation did not release capacity:", err)
		}
		terminal.cancelUploads(ErrClosed)
		uploadOutcome(t, result, ErrClosed)
	})
}

func TestSilentImageRequestsTimeOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		terminal := uploadTerminal(t, io.Discard)
		result, err := terminal.Transmit(t.Context(), uploadPNG(t))
		if err != nil {
			t.Fatal(err)
		}
		uploadOutcome(t, result, ErrImageResponseTimeout)
	})
}

func TestCancelledImageRetainsItsCleanupObligation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, malformed := range []bool{false, true} {
			var wire bytes.Buffer
			terminal := uploadTerminal(t, &wire)
			result, err := terminal.Transmit(t.Context(), uploadPNG(t))
			if err != nil {
				t.Fatal(err)
			}
			number := pendingImageNumber(t, terminal)
			cause := ErrClosed
			if malformed {
				cause = graphics.ErrMalformedReply
				if _, replyErr := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=broken,I=%d;OK", number)}); replyErr != nil {
					t.Fatal(replyErr)
				}
			} else {
				terminal.cancelUploads(cause)
			}
			uploadOutcome(t, result, cause)
			terminal.writer.CloseAdmission()
			if waitErr := terminal.awaitUploads(); !errors.Is(waitErr, ErrImageCleanupIncomplete) {
				t.Fatalf("unconfirmed cleanup = %v", waitErr)
			}
			if pendingImageNumber(t, terminal) != number {
				t.Fatal("cancellation lost the allocation obligation")
			}
			if consumed, replyErr := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=77,I=%d;OK", number)}); !consumed || replyErr != nil {
				t.Fatalf("late allocation = %t, %v", consumed, replyErr)
			}
			if waitErr := terminal.awaitUploads(); waitErr != nil {
				t.Fatal(waitErr)
			}
			if drainErr := terminal.writer.Drain(time.Second); drainErr != nil {
				t.Fatal(drainErr)
			}
			if strings.Count(wire.String(), "a=d,d=I,i=77,") != 1 {
				t.Fatalf("cleanup did not delete the exact allocation once: %q", wire.String())
			}
		}
	})
}

func TestImageCleanupWaitsForAnAllocationWhileInputRemainsOwned(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var wire bytes.Buffer
		terminal := uploadTerminal(t, &wire)
		result, err := terminal.Transmit(t.Context(), uploadPNG(t))
		if err != nil {
			t.Fatal(err)
		}
		number := pendingImageNumber(t, terminal)
		terminal.writer.CloseAdmission()
		terminal.cancelUploads(ErrClosed)
		uploadOutcome(t, result, ErrClosed)
		if !terminal.handed.hold() {
			t.Fatal("cannot claim input")
		}
		defer terminal.handed.release()
		replied := make(chan error, 1)
		go func() {
			time.Sleep(DrainGrace / 2)
			terminal.handed.park(terminal.stop)
			_, replyErr := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=77,I=%d;OK", number)})
			replied <- replyErr
		}()
		if waitErr := terminal.awaitUploads(); waitErr != nil {
			t.Fatal(waitErr)
		}
		if replyErr := <-replied; replyErr != nil {
			t.Fatal(replyErr)
		}
		if drainErr := terminal.writer.Drain(time.Second); drainErr != nil {
			t.Fatal(drainErr)
		}
		if strings.Count(wire.String(), "a=d,d=I,i=77,") != 1 {
			t.Fatal("closed ordinary admission prevented reserved image cleanup")
		}
	})
}

func TestImageWaitersObserveWriterFailure(t *testing.T) {
	failure := errors.New("output disconnected")
	terminal := uploadTerminal(t, imageFailureWriter{failure})
	result, err := terminal.Transmit(t.Context(), uploadPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	uploadOutcome(t, result, failure)
}

type imageFailureWriter struct{ err error }

func (w imageFailureWriter) Write([]byte) (int, error) { return 0, w.err }

func TestImageRequestNumberDoesNotOwnAnotherRequestsReply(t *testing.T) {
	terminal := uploadTerminal(t, io.Discard)
	result, err := terminal.Transmit(t.Context(), uploadPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if consumed, err := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=99,I=%d;OK", pendingImageNumber(t, terminal)+1)}); consumed || err != nil {
		t.Fatal("foreign request claimed an upload")
	}
	if _, err := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=99,I=%d;OK", pendingImageNumber(t, terminal))}); err != nil {
		t.Fatal(err)
	}
	uploadOutcome(t, result, nil)
}

func TestNewTerminalCannotClaimThePreviousInstancesLateReply(t *testing.T) {
	var wire bytes.Buffer
	first := uploadTerminal(t, &wire)
	data := uploadPNG(t)
	oldResult, err := first.Transmit(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	oldNumber := pendingImageNumber(t, first)
	first.cancelUploads(ErrClosed)
	uploadOutcome(t, oldResult, ErrClosed)
	if closeErr := first.writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	second := uploadTerminal(t, &wire)
	newResult, err := second.Transmit(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	newNumber := pendingImageNumber(t, second)
	if oldNumber == newNumber {
		t.Fatal("new terminal reused an earlier request number")
	}
	if consumed, err := second.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=12,I=%d;OK", oldNumber)}); consumed || err != nil {
		t.Fatal("previous terminal's reply acquired a new owner")
	}
	if _, err := second.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=44,I=%d;OK", newNumber)}); err != nil {
		t.Fatal(err)
	}
	if resource := uploadOutcome(t, newResult, nil); resource.ID() != 44 {
		t.Fatal("new terminal bound the stale physical ID")
	}
	if err := second.writer.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wire.String(), "a=d,d=I,i=12,") {
		t.Fatal("new terminal deleted the previous foreground's image")
	}
}

func TestCancelledImageChecksItsLifetimeBeforeTheWatcherRuns(t *testing.T) {
	var wire bytes.Buffer
	terminal := uploadTerminal(t, &wire)
	ctx := &delayedImageCancellation{done: make(chan struct{})}
	result, err := terminal.Transmit(ctx, uploadPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	number := pendingImageNumber(t, terminal)
	ctx.cancelled.Store(true)
	defer close(ctx.done)
	if _, err := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=71,I=%d;OK", number)}); err != nil {
		t.Fatal(err)
	}
	if resource := uploadOutcome(t, result, context.Canceled); resource.Live() {
		t.Fatal("ended lifetime acquired a live image before its watcher ran")
	}
	if err := terminal.writer.Drain(time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wire.String(), "a=d,d=I,i=71,") {
		t.Fatal("late successful allocation was not retired")
	}
}

type delayedImageCancellation struct {
	cancelled atomic.Bool
	done      chan struct{}
}

func (*delayedImageCancellation) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *delayedImageCancellation) Done() <-chan struct{}     { return c.done }
func (*delayedImageCancellation) Value(any) any               { return nil }
func (c *delayedImageCancellation) Err() error {
	if c.cancelled.Load() {
		return context.Canceled
	}
	return nil
}

func TestCancelledImageDeletionRetriesBoundedOutputBackpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		var wire bytes.Buffer
		terminal := uploadTerminal(t, imageGatedWriter{release: release, destination: &wire})
		data := uploadPNG(t)
		const count = 8
		for index := range count {
			ctx, cancel := context.WithCancel(t.Context())
			result, err := terminal.Transmit(ctx, data)
			if err != nil {
				t.Fatal(err)
			}
			number := pendingImageNumber(t, terminal)
			cancel()
			uploadOutcome(t, result, context.Canceled)
			if _, err := terminal.takeImageReply(input.APC{Body: fmt.Sprintf("Gi=%d,I=%d;OK", 100+index, number)}); err != nil {
				t.Fatal(err)
			}
		}
		terminal.writer.CloseAdmission()
		if err := terminal.awaitUploads(); !errors.Is(err, ErrImageCleanupIncomplete) {
			t.Fatalf("blocked deletion lost its cleanup obligation: %v", err)
		}
		terminal.modeMu.Lock()
		retained := len(terminal.uploads)
		terminal.modeMu.Unlock()
		if retained == 0 || retained > count {
			t.Fatalf("blocked cleanup lost bounded ownership: %d", retained)
		}
		close(release)
		released = true
		if err := terminal.awaitUploads(); err != nil {
			t.Fatal("sealed admission prevented exact cleanup:", err)
		}
		if err := terminal.writer.Drain(time.Second); err != nil {
			t.Fatal(err)
		}
		terminal.modeMu.Lock()
		retained = len(terminal.uploads)
		terminal.modeMu.Unlock()
		if retained != 0 {
			t.Fatalf("cleanup retained %d completed requests", retained)
		}
		for index := range count {
			sequence := fmt.Sprintf("a=d,d=I,i=%d,", 100+index)
			if strings.Count(wire.String(), sequence) != 1 {
				t.Fatalf("physical image %d was not deleted exactly once", 100+index)
			}
		}
	})
}

type imageGatedWriter struct {
	release     <-chan struct{}
	destination io.Writer
}

func (w imageGatedWriter) Write(data []byte) (int, error) {
	<-w.release
	return w.destination.Write(data)
}
