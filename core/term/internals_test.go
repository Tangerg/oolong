package term

// Tests of what this package keeps to itself: which escape sequences a session
// writes to take a terminal over and the order it puts them back in, and the pump
// that turns bytes into events. None of it has a public name, and ptytest asserts
// the same properties end to end against a real terminal.

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestOpenWithoutATerminal(t *testing.T) {
	// Under a test runner standard input is not a terminal, which is exactly the
	// case a caller has to handle rather than force: a program whose output is piped
	// wants text, not frames.
	if _, err := Open(Options{}); !errors.Is(err, ErrNotTerminal) {
		t.Fatalf("Open error = %v, want ErrNotTerminal", err)
	}
}

// driver runs a pump over channels a test controls.
type driver struct {
	raw     chan []byte
	readErr chan error
	resized chan struct{}
	stop    chan struct{}
	events  chan input.Event
	size    func() (int, int, error)
	result  chan error
	done    chan struct{}
}

func newDriver(grace time.Duration) *driver {
	d := &driver{
		raw:     make(chan []byte, 4),
		readErr: make(chan error, 1),
		resized: make(chan struct{}, 1),
		stop:    make(chan struct{}),
		events:  make(chan input.Event, 64),
		size:    func() (int, int, error) { return 80, 24, nil },
		result:  make(chan error, 1),
		done:    make(chan struct{}),
	}
	p := &pump{
		raw: d.raw, readErr: d.readErr, resized: d.resized, stop: d.stop,
		out: d.events, size: func() (int, int, error) { return d.size() }, grace: grace,
	}
	go func() {
		defer close(d.done)
		d.result <- p.run()
		close(d.events)
	}()
	return d
}

// next waits for one event.
func (d *driver) next(t *testing.T) input.Event {
	t.Helper()
	select {
	case ev, ok := <-d.events:
		if !ok {
			t.Fatal("the event channel closed while an event was expected")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
		return nil
	}
}

// silent asserts that nothing arrives within d.
func (d *driver) silent(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case ev := <-d.events:
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(within):
	}
}

func TestPumpDecodesBytesIntoEvents(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	defer close(d.stop)

	d.raw <- []byte("a\x1b[B")
	if got := d.next(t).(input.Key); !got.IsRune('a', 0) {
		t.Fatalf("first = %+v", got)
	}
	if got := d.next(t).(input.Key).Code; got != input.Down {
		t.Fatalf("second = %v", got)
	}
}

func TestPumpHoldsALoneEscapeThenDeliversIt(t *testing.T) {
	const grace = 40 * time.Millisecond
	d := newDriver(grace)
	defer close(d.stop)

	d.raw <- []byte("\x1b")
	// Held: it could still become a sequence, and guessing early would turn every
	// arrow key into an Escape followed by a letter.
	d.silent(t, grace/2)
	if got := d.next(t).(input.Key).Code; got != input.Esc {
		t.Fatalf("event = %v, want the Escape key once the wait was over", got)
	}
}

func TestPumpDisarmsTheWaitWhenTheSequenceArrives(t *testing.T) {
	d := newDriver(40 * time.Millisecond)
	defer close(d.stop)

	// The rest of the sequence arrives in a second read, which is normal.
	d.raw <- []byte("\x1b")
	d.raw <- []byte("[A")
	if got := d.next(t).(input.Key).Code; got != input.Up {
		t.Fatalf("event = %v, want the arrow key rather than an Escape", got)
	}
	d.silent(t, 60*time.Millisecond)
}

func TestPumpReportsResizes(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	defer close(d.stop)

	d.size = func() (int, int, error) { return 120, 40, nil }
	d.resized <- struct{}{}
	got := d.next(t).(input.Resize)
	if got.Width != 120 || got.Height != 40 {
		t.Fatalf("resize = %+v, want 120x40", got)
	}
}

func TestPumpIgnoresAResizeItCannotMeasure(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	defer close(d.stop)

	d.size = func() (int, int, error) { return 0, 0, errors.New("no size") }
	d.resized <- struct{}{}
	// Reporting a size of zero would have every widget lay out into nothing.
	d.silent(t, 40*time.Millisecond)
}

func TestEveryModeTurnedOnIsTurnedBackOff(t *testing.T) {
	// A mode left on outlives the process: mouse reporting still on means the shell
	// prints escape sequences when the user moves the mouse, and the alternate
	// screen still on means whatever was on screen before is gone.
	all := modes{altScreen: true, mouse: true, focus: true, keyboard: true}
	enter, leave := all.enter(), all.leave()

	for _, pair := range all.sequence() {
		if !strings.Contains(enter, pair.on) {
			t.Errorf("entering does not turn on %q", pair.on)
		}
		if !strings.Contains(leave, pair.off) {
			t.Errorf("leaving does not turn off %q", pair.on)
		}
	}
}

func TestModesAreUndoneInTheOppositeOrder(t *testing.T) {
	// The alternate screen is entered first and has to be left last, or the modes
	// underneath are put back onto a screen that is about to be discarded.
	all := modes{altScreen: true, mouse: true, focus: true, keyboard: true}
	enter, leave := all.enter(), all.leave()

	seq := all.sequence()
	for i := range seq {
		for j := i + 1; j < len(seq); j++ {
			if strings.Index(enter, seq[i].on) > strings.Index(enter, seq[j].on) {
				t.Fatalf("%q is turned on after %q", seq[i].on, seq[j].on)
			}
			if strings.Index(leave, seq[i].off) < strings.Index(leave, seq[j].off) {
				t.Fatalf("%q is turned off before %q", seq[i].off, seq[j].off)
			}
		}
	}
}

func TestAModeNotAskedForIsNeverTouched(t *testing.T) {
	none := modes{}
	enter, leave := none.enter(), none.leave()
	for _, unwanted := range []string{altScreenOn, mouseOn, focusOn, keyboardOn} {
		if strings.Contains(enter, unwanted) {
			t.Errorf("entering turned on %q without being asked", unwanted)
		}
	}
	for _, unwanted := range []string{altScreenOff, mouseOff, focusOff, keyboardOff} {
		if strings.Contains(leave, unwanted) {
			t.Errorf("leaving turned off %q that was never on", unwanted)
		}
	}
	// Bracketed paste is not optional: without it a pasted block arrives as
	// keystrokes, and pasted code runs.
	if !strings.Contains(enter, pasteOn) || !strings.Contains(leave, pasteOff) {
		t.Error("bracketed paste is not handled unconditionally")
	}
}

func TestLeavingAlwaysShowsTheCursor(t *testing.T) {
	// A frame may have hidden it, and a hidden cursor in the shell afterwards looks
	// like a hung terminal.
	if !strings.HasSuffix(modes{}.leave(), cursorShow) {
		t.Error("leaving does not end by showing the cursor")
	}
}

func TestPumpDeliversTheLastKeystrokeWhenInputEnds(t *testing.T) {
	d := newDriver(time.Second)
	// Bytes and the end of input arrive on separate channels, so both are waiting when
	// the pump comes round and a select cannot be told to prefer one. Neither the
	// keystroke that had not been decoded yet nor the one still held may be lost.
	d.raw <- []byte("ab\x1b")
	d.readErr <- errors.New("end of input")

	for _, want := range []rune{'a', 'b'} {
		if got := d.next(t).(input.Key); !got.IsRune(want, 0) {
			t.Fatalf("event = %+v, want %q", got, want)
		}
	}
	if got := d.next(t).(input.Key).Code; got != input.Esc {
		t.Fatalf("event = %v, want the held Escape", got)
	}
	select {
	case _, ok := <-d.events:
		if ok {
			t.Fatal("more events arrived after the input ended")
		}
	case <-time.After(time.Second):
		t.Fatal("the event channel never closed after the input ended")
	}
}

func TestPumpPreservesTheInputFailure(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	cause := errors.New("terminal read failed")
	d.readErr <- cause

	select {
	case _, ok := <-d.events:
		if ok {
			t.Fatal("an event arrived after a read failure")
		}
	case <-time.After(time.Second):
		t.Fatal("the event channel did not close after a read failure")
	}
	if got := <-d.result; !errors.Is(got, cause) {
		t.Fatalf("pump result = %v, want read failure", got)
	}
}

func TestPumpTreatsEOFAsACleanEnd(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	d.readErr <- io.EOF

	select {
	case _, ok := <-d.events:
		if ok {
			t.Fatal("an event arrived after EOF")
		}
	case <-time.After(time.Second):
		t.Fatal("the event channel did not close after EOF")
	}
	if got := <-d.result; got != nil {
		t.Fatalf("pump result = %v, want a clean end", got)
	}
}

func TestPumpClosesItsChannelWhenAskedToStop(t *testing.T) {
	d := newDriver(10 * time.Millisecond)
	close(d.stop)
	select {
	case <-d.done:
	case <-time.After(time.Second):
		t.Fatal("the pump did not return")
	}
	if _, ok := <-d.events; ok {
		t.Fatal("the event channel was not closed")
	}
}

func TestParseXParseColor(t *testing.T) {
	// The digit count is the precision, not padding. That is the whole of what
	// this form gets wrong when it is read as hexadecimal and nothing else: "8"
	// is eight fifteenths of full brightness, and "0008" is very nearly black.
	for _, tc := range []struct {
		spec string
		want grid.RGB
	}{
		{"rgb:1a1a/1b1b/2626", grid.RGB{R: 0x1a, G: 0x1b, B: 0x26}},
		{"rgb:ffff/ffff/ffff", grid.RGB{R: 255, G: 255, B: 255}},
		{"rgb:0000/0000/0000", grid.RGB{}},
		{"rgb:f/f/f", grid.RGB{R: 255, G: 255, B: 255}},
		{"rgb:ff/ff/ff", grid.RGB{R: 255, G: 255, B: 255}},
		{"rgb:8/8/8", grid.RGB{R: 136, G: 136, B: 136}},
		{"rgb:0008/0008/0008", grid.RGB{}},
		{"rgb:FDFD/F6F6/E3E3", grid.RGB{R: 0xfd, G: 0xf6, B: 0xe3}},
		{"rgb:0/8/f", grid.RGB{R: 0, G: 136, B: 255}},
	} {
		got, ok := parseXParseColor(tc.spec)
		if !ok {
			t.Errorf("%q was refused", tc.spec)
			continue
		}
		if got != tc.want {
			t.Errorf("%q = %+v, want %+v", tc.spec, got, tc.want)
		}
	}
}

func TestParseXParseColorRefusesWhatItCannotRead(t *testing.T) {
	// A colour invented for a malformed answer is worse than no colour: it decides
	// a whole theme, and nobody can tell it apart from one the terminal gave.
	for _, spec := range []string{
		"",
		"rgb:",
		"1a1a/1b1b/2626",
		"rgba:1/1/1/1",
		"rgb:1a1a/1b1b",
		"rgb:1a/1b/26/ff",
		"rgb:11111/1/1",
		"rgb://",
		"rgb:1a1a/1b1b/zzzz",
		"rgb:-1/0/0",
		"#1a1b26",
	} {
		if got, ok := parseXParseColor(spec); ok {
			t.Errorf("%q was read as %+v", spec, got)
		}
	}
}

func FuzzParseXParseColorNeverPanicsAndStaysInRange(f *testing.F) {
	// The colour a terminal answers with decides a whole theme, and it arrives on the
	// same input a hostile process could be writing to.
	for _, seed := range []string{
		"", "rgb:", "rgb:0/0/0", "rgb:ffff/ffff/ffff", "rgb:1a1a/1b1b/2626",
		"rgb:f/f/f", "rgb:0008/0008/0008", "rgba:1/1/1/1", "#1a1b26",
		"rgb://///", "rgb:zzzz/0/0", "rgb:99999999999/0/0",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, spec string) {
		got, ok := parseXParseColor(spec)
		if !ok {
			return
		}
		// A refusal is free; an acceptance has to mean something. Every channel is
		// already the eight bits a cell holds, so the only way this can be wrong is
		// by having accepted a specification that says nothing.
		if !strings.HasPrefix(spec, "rgb:") {
			t.Fatalf("accepted %q as the colour %+v", spec, got)
		}
	})
}
