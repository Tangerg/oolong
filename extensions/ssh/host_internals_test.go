package ssh

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestSourcePreservesReadTimesUnderBackpressure(t *testing.T) {
	for _, eof := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer func() {
				if err := reader.Close(); err != nil {
					t.Error(err)
				}
			}()
			defer func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			}()
			source := newEventSource(t.Context().Done(), reader, nil, nil)
			defer source.Close()
			if _, err := writer.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			first := time.Now()
			if _, err := writer.Write([]byte("\x1b[<0;1;1M")); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			time.Sleep(time.Second)
			second := time.Now()
			if _, err := writer.Write([]byte("\x1b[<0;1;1M")); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			if eof {
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
			}
			<-source.Events()
			a := (<-source.Events()).(input.Mouse)
			b := (<-source.Events()).(input.Mouse)
			if !a.At.Equal(first) || !b.At.Equal(second) || b.At.Sub(a.At) != time.Second {
				t.Fatalf("eof=%v: read times lost: %v, %v", eof, a.At, b.At)
			}
		})
	}
}

func TestHostClipboardTargetsTheClientTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var output strings.Builder
		host := &Host{writer: term.NewWriter(term.WriterConfig{Destination: &output}), clip: &clipboard.Channel{}}
		if !host.Copy("copied remotely") {
			t.Fatal("a small copy was refused")
		}
		result := make(chan error, 1)
		go func() { _, err := host.ReadClipboard(t.Context()); result <- err }()
		synctest.Wait()
		if _, err := host.ReadClipboard(t.Context()); !errors.Is(err, clipboard.ErrPending) {
			t.Fatalf("second read = %v", err)
		}
		host.clip.Answer("c;aGk=", time.Now())
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if err := host.writer.Close(); err != nil {
			t.Fatal(err)
		}
		written := output.String()
		wantCopy, _ := (&clipboard.Channel{}).Copy(clipboard.System, "copied remotely")
		for _, sequence := range []string{wantCopy, "\x1b]52;c;?\x1b\\"} {
			if !strings.Contains(written, sequence) {
				t.Errorf("client clipboard sequence missing: %q", written)
			}
		}
	})
}

func TestHostCannotAcceptClipboardCommandsAfterWriterClose(t *testing.T) {
	h := &Host{writer: term.NewWriter(term.WriterConfig{Destination: io.Discard}), clip: &clipboard.Channel{}}
	if err := h.writer.Close(); err != nil {
		t.Fatal(err)
	}
	if h.Copy("must remain selected") {
		t.Fatal("a closed SSH writer accepted a clipboard command")
	}
	if _, err := h.ReadClipboard(t.Context()); err == nil {
		t.Fatal("a closed SSH writer accepted a paste request")
	}
	if h.clip.Answer("c;dGV4dA==", time.Now()) {
		t.Fatal("a refused paste authorized an unsolicited answer")
	}
}

func TestEventSourceConsumesRequestedClipboardAnswersWithoutInput(t *testing.T) {
	reader := make(chanReader)
	clip := &clipboard.Channel{}
	ready := make(chan struct{})
	result := make(chan string, 1)
	go func() {
		value, err := clip.Read(t.Context(), clipboard.System, func(string) error { close(ready); return nil })
		if err != nil {
			t.Error(err)
		}
		result <- value
	}()
	<-ready
	source := newEventSource(t.Context().Done(), reader, nil, clip)
	t.Cleanup(source.Close)

	answer, _ := (&clipboard.Channel{}).Copy(clipboard.System, "from the client")
	reader <- readResult{data: []byte(answer + "z")}
	if value := <-result; value != "from the client" {
		t.Fatalf("read = %q", value)
	}
	event := receiveEvent(t, source.Events())
	if key, ok := event.(input.Key); !ok || !key.IsRune('z', 0) {
		t.Fatalf("reply became input: %#v", event)
	}
	unasked, _ := (&clipboard.Channel{}).Copy(clipboard.System, "unasked")
	reader <- readResult{data: []byte(unasked)}
	if event := receiveEvent(t, source.Events()); event == nil {
		t.Fatal("unasked clipboard answer disappeared")
	} else if _, ok := event.(input.OSC); !ok {
		t.Fatalf("unasked clipboard answer became %#v", event)
	}
}

func TestHostReadCancellationRetainsWireOwnership(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := &Host{writer: term.NewWriter(term.WriterConfig{Destination: io.Discard}), clip: &clipboard.Channel{}}
		defer func() { _ = host.writer.Close() }()
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() { _, err := host.ReadClipboard(ctx); result <- err }()
		synctest.Wait()
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled read = %v", err)
		}
		if _, err := host.ReadClipboard(t.Context()); !errors.Is(err, clipboard.ErrPending) {
			t.Fatalf("cancelled query lost wire ownership: %v", err)
		}
		if !host.clip.Answer("c;b2xk", time.Now()) {
			t.Fatal("old reply was not absorbed")
		}
	})
}

func TestSessionEnvironmentOwnsTheLastWellFormedValue(t *testing.T) {
	env := newEnvironment([]string{"TERM=dumb", "BROKEN", "=unnamed", "TERM=xterm-256color"})
	if got, ok := env.lookup("TERM"); !ok || got != "xterm-256color" {
		t.Fatalf("TERM = %q, %t", got, ok)
	}
	if _, ok := env.lookup("BROKEN"); ok {
		t.Fatal("malformed entry became an environment value")
	}
}

func TestAWindowUpdateIsAtomicAndZeroKeepsItsAxis(t *testing.T) {
	windows := make(chan charmssh.Window)
	h := &Host{windows: newWindowFeed(charmssh.Window{Width: 80, Height: 24}, windows)}
	defer h.windows.stop()
	windows <- charmssh.Window{Height: 40}
	if got := <-h.windows.events; got != (input.Resize{Width: 80, Height: 40}) {
		t.Fatalf("resize = %v", got)
	}
	width, height, err := h.Size()
	if err != nil || width != 80 || height != 40 {
		t.Fatalf("size = %dx%d, %v", width, height, err)
	}
}

func TestAnInvalidLaterWindowEndsInputWithoutChangingSize(t *testing.T) {
	windows := make(chan charmssh.Window)
	h := &Host{windows: newWindowFeed(charmssh.Window{Width: 80, Height: 24}, windows)}
	defer h.windows.stop()
	windows <- charmssh.Window{Width: grid.MaxTerminalCells, Height: 2}
	if _, ok := <-h.windows.events; ok || !errors.Is(h.windows.result(), ErrWindowSize) {
		t.Fatalf("invalid resize: open=%v err=%v", ok, h.windows.result())
	}
	width, height, _ := h.Size()
	if width != 80 || height != 24 {
		t.Fatalf("invalid update changed size to %dx%d", width, height)
	}
}

func TestEventSourceDecodesBytesThenReportsItsResult(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window)
	feed := newWindowFeed(charmssh.Window{Width: 80, Height: 24}, windows)
	t.Cleanup(feed.stop)
	source := newEventSource(t.Context().Done(), reader, feed, nil)
	t.Cleanup(source.Close)

	reader <- readResult{data: []byte("a")}
	event := receiveEvent(t, source.Events())
	key, ok := event.(input.Key)
	if !ok || key.Rune != 'a' || key.At.IsZero() {
		t.Fatalf("event = %#v", event)
	}

	windows <- charmssh.Window{Width: 100, Height: 30}
	if got := receiveEvent(t, source.Events()); got != (input.Resize{Width: 100, Height: 30}) {
		t.Fatalf("event = %#v", got)
	}

	want := errors.New("connection lost")
	reader <- readResult{err: want}
	if _, ok := <-source.Events(); ok {
		t.Fatal("events remained open after the read failed")
	}
	if !errors.Is(source.Err(), want) {
		t.Fatalf("error = %v, want %v", source.Err(), want)
	}
}

func TestResizeIntakeKeepsTheLatestWindow(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window)
	feed := newWindowFeed(charmssh.Window{Width: 80, Height: 24}, windows)
	t.Cleanup(feed.stop)
	source := newEventSource(t.Context().Done(), reader, feed, nil)
	t.Cleanup(source.Close)

	for width := 81; width <= 100; width++ {
		select {
		case windows <- charmssh.Window{Width: width, Height: 24}:
		case <-time.After(time.Second):
			t.Fatal("window intake stopped behind the event consumer")
		}
	}
	want := input.Resize{Width: 100, Height: 24}
	var last input.Event
	for last != want {
		last = receiveEvent(t, source.Events())
	}
	reader <- readResult{err: io.EOF}
	for event := range source.Events() {
		t.Errorf("unexpected event after EOF: %#v", event)
	}
}

// TestAClosedSourceStopsTakingTheSessionsBytes holds the boundary Close cannot
// enforce by waiting.
//
// One read is already in flight when a source closes and there is no way to recall
// it. Everything after that one is a choice, and offering the chunk in a select
// beside the stop signal makes it the wrong one: with room in the buffer the runtime
// may take either branch, so a closed source goes on consuming the caller's session.
// Each attempt below is an independent coin toss under that behaviour, which is why
// there are several.
func TestAClosedSourceStopsTakingTheSessionsBytes(t *testing.T) {
	for attempt := range 10 {
		reader := make(chanReader)
		source := newEventSource(t.Context().Done(), reader, nil, nil)

		// One byte through the whole source, so the reader is known to be running
		// and waiting on the next read rather than on its first.
		reader <- readResult{data: []byte("a")}
		receiveEvent(t, source.Events())
		source.Close()

		// The read already in flight may still take this. Whether it does is the
		// race Close cannot win, and either answer is allowed.
		select {
		case reader <- readResult{data: []byte("b")}:
		case <-time.After(50 * time.Millisecond):
		}
		// Coming back for another is not a race. It is a closed source reading a
		// session it no longer has any use for.
		select {
		case reader <- readResult{data: []byte("c")}:
			t.Fatalf("attempt %d: the closed source came back for more of the session", attempt)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func receiveEvent(t *testing.T, events <-chan input.Event) input.Event {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("event stream closed while waiting for an event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return nil
	}
}

type chanReader chan readResult

func (r chanReader) Read(p []byte) (int, error) {
	result := <-r
	return copy(p, result.data), result.err
}

var _ io.Reader = chanReader(nil)

func TestSourcePreservesInputEndTimeUnderBackpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader, writer := io.Pipe()
		defer func() {
			if err := reader.Close(); err != nil {
				t.Error(err)
			}
		}()
		defer func() {
			if err := writer.Close(); err != nil {
				t.Error(err)
			}
		}()
		source := newEventSource(t.Context().Done(), reader, nil, nil)
		defer source.Close()
		if _, err := writer.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		ended := time.Now()
		if _, err := writer.Write([]byte("\x1b")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(time.Second)
		<-source.Events()
		key := (<-source.Events()).(input.Key)
		if key.Code != input.Esc || !key.At.Equal(ended) {
			t.Fatalf("end stamped with decoding time: %+v; want %v", key, ended)
		}
	})
}

func TestWindowIntakeBelongsToTheChannelLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		windows := make(chan charmssh.Window)
		feed := newWindowFeed(charmssh.Window{Width: 80, Height: 24}, windows)
		reader := make(chanReader)
		source := newEventSource(t.Context().Done(), reader, feed, nil)
		defer source.Close()
		reader <- readResult{err: io.EOF}
		for range source.Events() {
			t.Fatal("unexpected event")
		}
		windows <- charmssh.Window{Width: 99}
		synctest.Wait()
		if got := feed.size(); got.Width != 99 {
			t.Fatalf("stopped draining before SSH channel teardown: %v", got)
		}
		feed.stop()
		select {
		case windows <- charmssh.Window{Width: 100}:
			t.Fatal("window listener survived channel teardown")
		default:
		}
	})
}

func TestResizeFailureFollowsTheLastAcceptedWindow(t *testing.T) {
	for range 16 {
		synctest.Test(t, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer func() { _ = writer.Close() }()
			defer func() { _ = reader.Close() }()
			windows := make(chan charmssh.Window)
			feed := newWindowFeed(charmssh.Window{Width: 80, Height: 24}, windows)
			defer feed.stop()
			source := newEventSource(t.Context().Done(), reader, feed, nil)
			defer source.Close()
			if _, err := writer.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			windows <- charmssh.Window{Width: 100}
			windows <- charmssh.Window{Height: -1}
			synctest.Wait()

			<-source.Events()
			if got := <-source.Events(); got != (input.Resize{Width: 100, Height: 24}) {
				t.Fatalf("last accepted window lost before resize error: %v", got)
			}
			if _, open := <-source.Events(); open || !errors.Is(source.Err(), ErrWindowSize) {
				t.Fatalf("open=%v err=%v", open, source.Err())
			}
		})
	}
}
