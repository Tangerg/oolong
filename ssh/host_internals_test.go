package ssh

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	charmssh "charm.land/ssh"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

func TestHostClipboardTargetsTheClientTerminal(t *testing.T) {
	var output strings.Builder
	host := &host{
		writer: term.NewWriter(&output),
		clip:   &clipboard.Channel{},
	}
	if !host.Copy("copied remotely") {
		t.Fatal("a small copy was refused")
	}
	if !host.Paste() {
		t.Fatal("the first paste request was refused")
	}
	if host.Paste() {
		t.Fatal("a second unidentified paste request was accepted")
	}
	if err := host.writer.Close(); err != nil {
		t.Fatal(err)
	}
	written := output.String()
	wantCopy, _ := (&clipboard.Channel{}).Copy(clipboard.System, "copied remotely")
	wantPaste, _ := (&clipboard.Channel{}).Request(clipboard.System)
	for name, sequence := range map[string]string{"copy": wantCopy, "paste": wantPaste} {
		if !strings.Contains(written, sequence) {
			t.Errorf("client %s sequence was not written: %q", name, written)
		}
	}
}

func TestEventSourceTurnsOnlyTheRequestedClipboardAnswerIntoPaste(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window)
	clip := &clipboard.Channel{}
	if _, ok := clip.Request(clipboard.System); !ok {
		t.Fatal("clipboard request was refused")
	}
	source := newEventSource(t.Context().Done(), reader, windows,
		func(window charmssh.Window) (input.Resize, bool, error) {
			return input.Resize{Width: window.Width, Height: window.Height}, true, nil
		}, clip)
	t.Cleanup(source.Close)

	answer, _ := (&clipboard.Channel{}).Copy(clipboard.System, "from the client")
	reader <- readResult{data: []byte(answer)}
	event := receiveEvent(t, source.Events())
	paste, ok := event.(input.Paste)
	if !ok || paste.Text != "from the client" {
		t.Fatalf("clipboard answer = %#v, want client paste", event)
	}

	unasked, _ := (&clipboard.Channel{}).Copy(clipboard.System, "unasked")
	reader <- readResult{data: []byte(unasked)}
	if event := receiveEvent(t, source.Events()); event == nil {
		t.Fatal("unasked clipboard answer disappeared")
	} else if _, ok := event.(input.OSC); !ok {
		t.Fatalf("unasked clipboard answer became %#v", event)
	}
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
	h := &host{window: charmssh.Window{Width: 80, Height: 24}}
	resized, changed, err := h.resize(charmssh.Window{Height: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || resized != (input.Resize{Width: 80, Height: 40}) {
		t.Fatalf("resize = %#v, %t", resized, changed)
	}
	width, height, err := h.Size()
	if err != nil || width != 80 || height != 40 {
		t.Fatalf("size = %dx%d, %v", width, height, err)
	}
}

func TestAnInvalidLaterWindowEndsInputWithoutChangingSize(t *testing.T) {
	h := &host{window: charmssh.Window{Width: 80, Height: 24}}
	_, changed, err := h.resize(charmssh.Window{Width: program.MaxCells, Height: 2})
	if !errors.Is(err, ErrWindowSize) || changed {
		t.Fatalf("resize = changed %t, error %v", changed, err)
	}
	width, height, _ := h.Size()
	if width != 80 || height != 24 {
		t.Fatalf("invalid update changed size to %dx%d", width, height)
	}
}

func TestEventSourceDecodesBytesThenReportsItsResult(t *testing.T) {
	reader := make(chanReader)
	windows := make(chan charmssh.Window)
	source := newEventSource(t.Context().Done(), reader, windows,
		func(window charmssh.Window) (input.Resize, bool, error) {
			return input.Resize{Width: window.Width, Height: window.Height}, true, nil
		}, nil)
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
	windows := make(chan charmssh.Window, 32)
	observed := make(chan struct{}, 32)
	source := newEventSource(t.Context().Done(), reader, windows,
		func(window charmssh.Window) (input.Resize, bool, error) {
			observed <- struct{}{}
			return input.Resize{Width: window.Width, Height: window.Height}, true, nil
		}, nil)
	t.Cleanup(source.Close)

	for width := 81; width <= 100; width++ {
		windows <- charmssh.Window{Width: width, Height: 24}
	}
	for range 20 {
		select {
		case <-observed:
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
		source := newEventSource(t.Context().Done(), reader, make(chan charmssh.Window),
			func(window charmssh.Window) (input.Resize, bool, error) {
				return input.Resize{Width: window.Width, Height: window.Height}, true, nil
			}, nil)

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

type readResult struct {
	data []byte
	err  error
}

type chanReader chan readResult

func (r chanReader) Read(p []byte) (int, error) {
	result := <-r
	return copy(p, result.data), result.err
}

var _ io.Reader = chanReader(nil)
