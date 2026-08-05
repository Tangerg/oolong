// Package term owns the terminal itself: taking it over, giving it back, reading
// what it sends, and writing frames to it without blocking whoever drew them.
//
// It is the only package in the TUI that touches the operating system. Everything
// above it works in cells, events and frames, and can be tested without a terminal
// at all — which is the point of putting the parts that need one here and nowhere
// else.
package term

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	xterm "golang.org/x/term"

	"github.com/Tangerg/oolong/core/clipboard"
	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// ErrNotTerminal is reported by [Open] when the process is not attached to a
// terminal — piped, redirected, or running under something that gave it no tty.
var ErrNotTerminal = errors.New("term: not a terminal")

// Options says which of the terminal's optional behaviours a session wants.
//
// The zero value asks for none of them, which is a legitimate choice for a
// session that only wants raw keys, and is why each is named for what turning it
// on gets you rather than for turning it off.
type Options struct {
	// AltScreen draws on a screen of its own, leaving the user's scrollback as it
	// was. Without it, frames are drawn in place among whatever else is on screen.
	AltScreen bool
	// Mouse asks for mouse reporting, including movement and not only clicks.
	Mouse bool
	// Focus asks to be told when the terminal window gains or loses focus, which is
	// what lets a UI stop animating while nobody is looking.
	Focus bool
	// Keyboard asks for the Kitty keyboard protocol: unambiguous key codes, key
	// releases and repeats, and the text a key produced. Terminals that do not
	// implement it ignore the request.
	Keyboard bool
	// Probe asks the terminal about itself while [Open] is still running: the
	// colour it draws on, and the extensions it claims. See [Terminal.Background]
	// and [Terminal.Attributes].
	//
	// It is the only option that costs anything — one round trip to the terminal,
	// normally a millisecond and bounded either way — and the only one whose answer
	// a session cannot get any other way. A theme that has to be told whether the
	// terminal is light is a theme that is wrong for half the people who run it.
	Probe bool
}

// Terminal is a terminal taken over for a session.
//
// It is the whole boundary: raw mode, the modes it turned on, the goroutines
// reading input, and the writer frames go through. [Terminal.Close] gives all of
// it back, in the order that leaves the terminal as it was found, and is safe to
// call more than once — including from a deferred call on a path that already
// failed.
type Terminal struct {
	in, out  *os.File
	modes    modes
	oldState *xterm.State

	events chan input.Event
	writer *Writer
	// waker ends the reader's wait when this process, rather than the terminal, has
	// something to say about it.
	waker *waker
	// handed is the latch the reader parks on while the terminal is somebody else's
	// — see [Terminal.Hand].
	handed handover
	// said is what the terminal was willing to say about itself, when it was asked.
	// It is written once during Open and only read afterwards.
	said answers
	// pasting is set between asking for the clipboard and the answer arriving. It is
	// atomic because Paste may be called from anywhere and the pump reads it.
	pasting atomic.Bool

	winch    chan os.Signal
	resized  chan struct{}
	stop     chan struct{}
	pumpDone chan struct{}
	fanDone  chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// Open takes over the terminal on standard input and output.
//
// It reports [ErrNotTerminal] when there is no terminal to take over, which is the
// case a caller has to handle rather than force: a program whose output is being
// piped wants to write text, not frames.
func Open(opts Options) (*Terminal, error) {
	return OpenOn(os.Stdin, os.Stdout, opts)
}

// OpenOn takes over a terminal that is not this process's own.
//
// Standard input and output are the ordinary answer and [Open] is the ordinary
// call. This exists because they are not the only answer: a program serving a
// session over a pty holds one at each end, and everything below this line worked
// on whatever files it was handed long before anything could hand it any.
//
// It is also what makes this package testable at all. A terminal that could only
// ever be the process's own is one whose lifecycle — raw mode, the modes it turns
// on, the order it puts them back in — could be checked only by running a second
// program and reading what came out of it.
func OpenOn(in, out *os.File, opts Options) (*Terminal, error) {
	fd := int(in.Fd())
	if !xterm.IsTerminal(fd) {
		return nil, fmt.Errorf("%w: standard input is fd %d", ErrNotTerminal, fd)
	}

	// Raw mode first: it is what stops the terminal from interpreting keys on the
	// program's behalf, and everything below assumes it.
	oldState, err := xterm.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("term: enter raw mode: %w", err)
	}

	t := &Terminal{
		in:  in,
		out: out,
		modes: modes{
			altScreen: opts.AltScreen,
			mouse:     opts.Mouse,
			focus:     opts.Focus,
			keyboard:  opts.Keyboard,
		},
		oldState: oldState,
		events:   make(chan input.Event, 64),
		winch:    make(chan os.Signal, 1),
		resized:  make(chan struct{}, 1),
		stop:     make(chan struct{}),
		pumpDone: make(chan struct{}),
		fanDone:  make(chan struct{}),
	}

	if _, err := out.WriteString(t.modes.enter()); err != nil {
		_ = xterm.Restore(fd, oldState)
		return nil, fmt.Errorf("term: take over the terminal: %w", err)
	}

	t.writer = NewWriter(out)
	waker, wakeErr := newWaker(fd)
	if wakeErr != nil {
		_, _ = out.WriteString(t.modes.leave())
		_ = xterm.Restore(fd, oldState)
		return nil, wakeErr
	}
	t.waker = waker
	notifyResize(t.winch)

	// The size is delivered as an event rather than left to be asked for, so a
	// session learns its size the same way it learns about every later change.
	if w, h, err := t.Size(); err == nil {
		t.events <- input.Resize{Width: w, Height: h}
	}

	raw := make(chan []byte, 4)
	readErr := make(chan error, 1)
	go t.read(raw, readErr)

	// Asking has to happen here, between the reader starting and the pump starting.
	// A terminal has exactly one reader; asking any earlier means nothing is
	// listening for the answer, and any later means two readers race for it.
	parser := &input.Parser{}
	var early []input.Event
	if opts.Probe {
		pr := &probe{raw: raw, out: out, parser: parser}
		t.said = pr.run()
		early = pr.early
	}

	p := &pump{
		raw: raw, readErr: readErr, resized: t.resized, stop: t.stop,
		out: t.events, parser: parser, early: early, pasting: &t.pasting, size: t.Size,
	}
	go t.fanResize()
	go func() {
		defer close(t.pumpDone)
		p.run()
	}()
	return t, nil
}

// Events is the terminal's input, closed when the input ends or the session does.
func (t *Terminal) Events() <-chan input.Event { return t.events }

// Writer is where frames go.
func (t *Terminal) Writer() *Writer { return t.writer }

// Ground is what the terminal's own two colours are, and whether it said.
//
// It answers two questions that used to be one. Which theme suits is decided by the
// background alone — [grid.RGB.Dark] turns it into a yes or no. What a translucent
// layer mixes with needs both, because a cell left at the terminal's own colours has
// no numbers of its own until this says what they are; that is what [grid.Ground] is
// for and why a frame is given one.
//
// A colour the terminal was not asked for, or would not give, comes back as the
// default — see [grid.Color.Default]. A session that gets one has to choose for
// itself, because there is no safe guess: dark is the commoner choice and light is
// the one that becomes unreadable when guessed wrong.
func (t *Terminal) Ground() grid.Ground {
	var g grid.Ground
	if t.said.hasFg {
		g.FG = grid.RGBColor(t.said.foreground.R, t.said.foreground.G, t.said.foreground.B)
	}
	if t.said.hasBg {
		g.BG = grid.RGBColor(t.said.background.R, t.said.background.G, t.said.background.B)
	}
	return g
}

// Attributes is what the terminal said it was, and whether it said.
//
// The class is of little use. What the extensions carry is worth having: sixel
// graphics is claimed here and nowhere else, so this is how a program learns it
// can draw pixels on a terminal that does not speak Kitty's protocol.
func (t *Terminal) Attributes() (input.DeviceAttributes, bool) {
	return t.said.attributes, t.said.hasAttrs
}

// clipboardCommand is the operating system command that carries a clipboard.
const clipboardCommand = 52

// Copy asks the terminal to put text on the system clipboard, reporting false for
// text too large to carry — see [clipboard.Limit].
//
// The terminal does it rather than the process, because the terminal is the only
// part of this on the user's side of the connection. Over ssh, in a container, or
// through a multiplexer running elsewhere, shelling out to pbcopy fills a clipboard
// nobody can paste from.
//
// The sequence is queued beside the frames, so it lands between two of them and
// never inside one. A terminal is free to refuse and says nothing when it does, so
// true means asked for rather than done.
func (t *Terminal) Copy(text string) bool {
	seq, ok := clipboard.Copy(clipboard.System, text)
	if !ok {
		return false
	}
	t.writer.Queue([]byte(seq))
	return true
}

// Paste asks the terminal what is on the system clipboard.
//
// The answer arrives on [Terminal.Events] as an ordinary [input.Paste], because
// that is what it is: a component that already inserts what the user pasted needs
// nothing further to insert what they copied somewhere else.
//
// Most terminals refuse to answer, because a program that can read the clipboard
// can read what the user copied out of a password manager. A refusal has no reply,
// so nothing should wait on one.
func (t *Terminal) Paste() {
	t.pasting.Store(true)
	t.writer.Queue([]byte(clipboard.Request(clipboard.System)))
}

// Name is what the terminal called itself when asked, and whether it answered.
//
// It is worth more than anything the environment says, which is why everything here
// that identifies a terminal prefers it. Environment variables do not survive ssh, do
// not exist in a container, and are rewritten by a multiplexer; an answer to this came
// from the terminal that is actually drawing.
//
// The form is whatever the terminal chose — "kitty(0.32.2)", "WezTerm 20240203" — so
// it is matched against and not parsed.
func (t *Terminal) Name() (string, bool) { return t.said.name, t.said.hasName }

// Version is the number a terminal gives when it will not give a name, and whether it
// gave one. See [input.DeviceVersion] for why the numbers are not interpreted here.
func (t *Terminal) Version() (input.DeviceVersion, bool) {
	return t.said.version, t.said.hasVersion
}

// Keyboard is which of the Kitty keyboard protocol's enhancements the terminal
// actually turned on, and whether it said.
//
// Asking for them is not the same as getting them, and that difference is invisible in
// the events themselves. A terminal can accept the request for unambiguous key codes
// and give nothing for key releases: the protocol is live, Shift+Enter works, the
// teardown still owes a pop — and every key is held forever as far as this program can
// tell. A component that waits for a key to be let go would wait for ever, and nothing
// would say why.
//
// A session that did not ask for the protocol, or a terminal that does not implement
// it, reports false.
func (t *Terminal) Keyboard() (input.KeyboardFlags, bool) {
	return t.said.keyboard, t.said.hasKeyboard
}

// identity is what to match a terminal against: what it called itself when asked, and
// nothing when it did not.
func (t *Terminal) identity() string {
	if t.said.hasName {
		return t.said.name
	}
	return ""
}

// Wheel is what this terminal's wheel reports are worth.
//
// It prefers what the terminal said it was over what the environment claims, for the
// reason [Terminal.Name] gives.
func (t *Terminal) Wheel() input.Wheel {
	return input.WheelFor(os.Getenv, t.identity())
}

// Graphics is the richest way this terminal will take an image.
//
// It is the environment and the terminal's own claims together, which is what it
// takes: the environment names the terminal, and only the terminal names sixel. A
// session that did not ask — see [Options.Probe] — gets the answer the environment
// alone supports, which is the same as [DetectGraphics].
func (t *Terminal) Graphics() graphics.Protocol {
	sixel := t.said.hasAttrs && t.said.attributes.Has(sixelAttribute)
	return graphics.DetectIn(os.Getenv, t.identity(), sixel)
}

// ReportDirectory tells the terminal which directory the program is working in.
//
// It is the other half of leaving relative paths alone. A terminal finds paths in its
// own output and offers to open them, and it can only resolve "src/main.go" against a
// directory it knows — which, once a program has changed its own, is not the one the
// shell started in. Without this the terminal's own path handling quietly stops
// working for exactly the output a program produces, and the reason a program declines
// to make a relative path a hyperlink is that the terminal would do it better.
//
// The path is made absolute, because a relative one tells the terminal nothing it did
// not already have. An empty path reports the process's own working directory.
//
// It is written beside the frames, so it lands between two of them and never inside
// one. A terminal that does not implement it ignores it.
func (t *Terminal) ReportDirectory(path string) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("term: report the directory: %w", err)
		}
		path = cwd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("term: report the directory: %w", err)
	}
	// The host is part of the form and is what tells a terminal on the other end of an
	// ssh connection that the path is not one of its own. A machine that cannot name
	// itself reports none, which is the form's own way of saying "wherever this is".
	host, _ := os.Hostname()
	t.writer.Queue([]byte("\x1b]7;file://" + host + pathEscape(abs) + "\x1b\\"))
	return nil
}

// pathEscape percent-encodes the bytes of a path that cannot appear in a URL.
//
// url.PathEscape is not it: that escapes the separators too, and a path whose slashes
// are %2F is a path no terminal will resolve. What has to go is the small set that
// would end the sequence or be read as part of the URL's own syntax.
func pathEscape(path string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(path) {
		switch c := path[i]; {
		case c > 0x20 && c < 0x7f && c != '%' && c != '#' && c != '?':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}

// Size is the terminal's size in cells.
func (t *Terminal) Size() (w, h int, err error) {
	return xterm.GetSize(int(t.out.Fd()))
}

// Close gives the terminal back.
//
// The order is the reverse of taking it over, and every step runs even if an
// earlier one failed: a terminal left in raw mode is unusable, so a failure to
// write the restore sequences must not be a reason to skip leaving raw mode.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		signal.Stop(t.winch)
		close(t.stop)
		// The reader is waiting for the terminal to say something, and nothing will:
		// waking it is what lets it see that the session is over. It is also what
		// releases a reader parked mid-handover, so a session closed while the
		// terminal is somebody else's still ends.
		t.waker.wake()
		<-t.fanDone

		// Frames first: the writer may still hold one, and writing it after the
		// modes have been put back would draw onto the user's restored screen.
		var errs []error
		if err := t.writer.Close(); err != nil {
			errs = append(errs, err)
		}
		errs = append(errs, t.giveBack()...)
		// The pump owns the event channel, so it has to have finished before anyone
		// could observe the channel closed.
		<-t.pumpDone
		t.closeErr = errors.Join(errs...)
	})
	return t.closeErr
}

// giveBack puts the terminal back the way it was found: every mode this session
// turned on, turned off in the opposite order, and then cooked mode.
//
// It is the whole of what taking a terminal over has to be undone by, which is why
// closing the session and handing it to a child both go through here. Two of these
// would drift, and the way they would drift is one of them forgetting a mode — a
// terminal the user has to close.
func (t *Terminal) giveBack() []error {
	var errs []error
	if _, err := t.out.WriteString(t.modes.leave()); err != nil {
		errs = append(errs, fmt.Errorf("term: give the terminal back: %w", err))
	}
	if err := xterm.Restore(int(t.in.Fd()), t.oldState); err != nil {
		errs = append(errs, fmt.Errorf("term: leave raw mode: %w", err))
	}
	return errs
}

// read waits for the terminal to have something to say, reads it, and hands the
// chunk to the pump.
//
// The waiting is separate from the reading, and that is the whole shape of it. A
// blocking read cannot be cancelled portably, so a reader that simply read would be
// inside a read at exactly the moments a session needs it not to be: while the
// terminal belongs to a child, and after the session has been closed. Waiting first
// means every one of those finds this goroutine holding nothing — see [waker].
//
// Where there is no way to wait — see [interruptible] — the wait reports "maybe"
// and the read below it does the blocking, which is what a reader that cannot be
// woken did before any of this existed.
func (t *Terminal) read(raw chan<- []byte, readErr chan<- error) {
	defer t.waker.close()
	for {
		// Before the read and not after it: a byte taken while the terminal is
		// somebody else's is a byte they never see.
		t.handed.park(t.stop)
		select {
		case <-t.stop:
			return
		default:
		}
		ready, err := t.waker.wait()
		if err != nil {
			readErr <- err
			return
		}
		if !ready {
			// This process asked for the reader back, so nothing was read and there is
			// nothing to hand on. Whether that was a handover or the end of the session
			// is answered at the top of the loop.
			continue
		}

		buf := make([]byte, 4096)
		n, err := t.in.Read(buf)
		if n > 0 {
			select {
			case raw <- buf[:n]:
			case <-t.stop:
				return
			}
		}
		if err != nil {
			readErr <- err
			return
		}
		select {
		case <-t.stop:
			return
		default:
		}
	}
}

// fanResize turns resize signals into wake-ups the pump can select on, collapsing
// a burst of them into one. A drag of the window edge produces dozens; the size is
// read once, after.
func (t *Terminal) fanResize() {
	defer close(t.fanDone)
	for {
		select {
		case <-t.winch:
			select {
			case t.resized <- struct{}{}:
			default:
			}
		case <-t.stop:
			return
		}
	}
}
