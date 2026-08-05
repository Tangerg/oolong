// Package program runs a terminal interface.
//
// It owns the terminal, the frame schedule, and the one goroutine that is allowed to
// touch the interface's state. It knows nothing about what the interface is for: what
// it drives is a [Component], which draws itself and answers input, and everything a
// component needs from the program it asks for through a [Loop].
//
// # The concurrency model, in full
//
// One goroutine draws and handles input. Anything that happens elsewhere — a request
// finishing, a file changing, a timer firing — reaches the interface by being posted
// to that goroutine with [Loop.Post], and runs there. That is the whole of it, and it
// is why every widget below this package is an ordinary mutable object with no lock in
// it.
//
// The program parks when there is nothing to do. It wakes for input, for posted work,
// and for the terminal reporting progress — never on a clock that runs regardless. A
// component that wants a clock starts one with [Loop.Every], and an interface with
// nothing animating costs nothing.
//
// # The two places an interface can be
//
// A program either takes a screen of its own, which it gives back on the way out, or
// draws in the terminal's own screen as a block with the session's output above it.
// The second is what [Config.Inline] asks for, and it is the difference between a
// program the user enters and leaves and one that is part of their session: what an
// inline interface has finished with is printed with [InlineLoop.Print] and belongs to
// the terminal from then on — scrollable, selectable, and still there afterwards.
package program

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/present"
	"github.com/Tangerg/oolong/core/term"
)

// DefaultFrameRate is the fastest a program redraws. A terminal cannot usefully show
// more, and a stream of updates would otherwise ask for a frame each.
const DefaultFrameRate = 16 * time.Millisecond

// Component is an interface a program can run: it draws itself into the space it is
// given, and says whether it wants an event.
//
// It is handed a view that is already positioned and clipped, so its coordinates are
// its own. An event it does not consume is dropped by the program — a component is the
// root of its own tree and there is nobody above it to pass one on to.
type Component interface {
	grid.Drawer
	input.Handler
}

// Loop is what a component may ask of the program running it.
//
// Every method is safe from any goroutine, which is the point: a component holds one of
// these and hands it to whatever fetches, watches or waits on its behalf, and the
// answers come back on the goroutine that owns the state.
type Loop interface {
	// Refresh asks for a frame.
	Refresh()

	// Post runs fn on the program's goroutine and asks for a frame afterwards.
	//
	// This is the only safe way to change what a component holds from anywhere else.
	// Work posted after the program has stopped is dropped rather than run: there is
	// nothing left to show it, and blocking the caller for ever would be worse.
	Post(fn func())

	// Every calls fn on the program's goroutine at an interval until the returned
	// function is called, or until the program stops.
	//
	// It is how anything animated advances. Nothing ticks unless something asked for
	// it, which is what lets an idle interface be silent.
	Every(d time.Duration, fn func()) (stop func())

	// Quit asks the program to stop. The program returns from Run once the frame in
	// hand has been dealt with.
	Quit()

	// Ground is what the terminal's own two colours are, as far as anyone knows.
	//
	// They are known when the terminal was asked and answered — see
	// [term.Options.Probe]. The background is the one a look is usually built on, and
	// [grid.RGB.Dark] turns it into the only question a theme has; a component given
	// no answer has to choose for itself, and there is no safe guess, because dark is
	// the commoner choice and light is the one that becomes unreadable when it is
	// guessed wrong.
	//
	// The colours are here rather than the conclusion because the conclusion is not
	// the only use for them. A component drawing a gradient, or floating a layer over
	// what is behind it, needs the numbers — and a lower layer that only ever answered
	// "dark?" would have decided for everyone above it. The frame already carries the
	// same answer for drawing's sake, which is why a widget with a view in hand asks
	// [grid.View.Ground] instead of coming back here.
	Ground() grid.Ground

	// Copy asks for text to be put on the system clipboard, reporting false for
	// text too large to carry — see [term.Terminal.Copy].
	//
	// Who does the copying is the host's business and not a component's. A terminal
	// asks the terminal, because over ssh or through a multiplexer that is the only
	// end of the connection the user is at; a host that knows better can do it
	// another way without anything above having to hear about it.
	Copy(text string) bool

	// Wheel is what this terminal's wheel reports are worth, which is not a constant:
	// terminals send between one and three of them for one notch, and there is no way
	// to tell from a report. A component holding a scroll passes it on once — see
	// [headless.Scroll.Wheel].
	Wheel() input.Wheel

	// Keyboard is which of the Kitty keyboard protocol's enhancements the terminal
	// actually turned on, and whether it said.
	//
	// A component that waits for a key to be let go needs it. Asking for the
	// enhancements is not the same as getting them: a terminal can accept unambiguous
	// key codes and give nothing for releases, and then every key is held for ever as
	// far as this program can tell. Nothing in the events distinguishes that from a
	// user who has not lifted a key, so a component that cannot ask has no way to
	// choose a different interaction — which is what this is for.
	Keyboard() (input.KeyboardFlags, bool)

	// ReportDirectory tells the terminal which directory the program is working in.
	//
	// It is what lets a terminal resolve the relative paths in a program's own output
	// — which is why [link.Link.Hyperlink] declines to make one a hyperlink and leaves
	// it to the terminal. A host that is not a terminal does nothing.
	ReportDirectory(path string) error

	// Paste asks for the system clipboard's contents.
	//
	// The answer arrives as an ordinary [input.Paste], which is what makes this
	// worth having: a component that already inserts what the user pasted needs
	// nothing further to insert what they copied somewhere else. It may never
	// arrive — most terminals refuse to be read — so nothing should wait on one.
	Paste()
}

// InlineLoop is what an inline program's component may ask of it: everything a
// [Loop] offers, and somewhere to put output that is finished.
//
// It is a separate interface rather than two more methods on [Loop] because a
// program drawing on a screen of its own has nowhere to print: that screen has no
// scrollback, and output written above the interface would be scrolled away and
// gone. A component that means to print says so by asking for this, and a program
// that cannot offer it cannot be given such a component.
type InlineLoop interface {
	Loop

	// Print draws something that can size itself into the terminal's own output,
	// above the interface, where it stays after the program exits.
	//
	// It is how a streaming interface says something final: the interface itself is
	// what is still changing, and everything it has finished with belongs to the
	// terminal.
	//
	// The width is the program's to know and not the caller's. Measuring happens
	// here, on the goroutine that owns the interface, which is both why a caller
	// does not have to remember how wide the last frame was and why measuring a
	// widget from somewhere else — a mutable object, on another goroutine — is not
	// something this can be made to do by accident.
	Print(p Printable)

	// PrintRows is the same for a caller that has already worked out the height:
	// rows it composed by hand, or content whose shape it knows better than any
	// measurement would. draw is given a view rows tall and as wide as the
	// interface.
	PrintRows(rows int, draw func(grid.View))

	// Append puts output onto the end of what was printed last rather than onto a
	// row of its own, which is what output arriving in pieces needs: a reply
	// streaming in three words at a time is one paragraph, not three rows.
	//
	// draw is given what is left of the open row and says whether there is more to
	// come. When there is, it is called again with a whole row, and again until it
	// says there is not — which is what lets a caller lay text out against room it
	// has no way of knowing until the loop tells it. A round with a whole row to
	// itself that draws nothing ends it, because asking again would never end.
	Append(draw func(v grid.View) (more bool))
}

// Printable is something that can say how tall it is at a width and then draw
// itself into that space.
//
// Both halves are named from below rather than invented here: [grid.Drawer] and
// [layout.Measurer] are what the substrate already calls these, and a loop that
// coined its own words for them would be a lower layer taking its vocabulary from
// an upper one. It is the same reason components' own Sized is built from the same
// two — not because either copied the other, but because both are spelled in the
// language underneath them, which is what lets everything in components satisfy
// this without an adapter and without the loop knowing components exists.
type Printable interface {
	grid.Drawer
	layout.Measurer
}

// Host is where a program's input comes from and its frames go.
//
// A program opens the real terminal unless it is given one of these. Being able to
// supply it is what lets an interface be driven and inspected in a test, with no
// terminal in sight.
type Host interface {
	// Events is the input, closed when the input ends.
	Events() <-chan input.Event
	// Writer is where frames go.
	Writer() *term.Writer
	// Size is the terminal's size in cells.
	Size() (w, h int, err error)
	// Ground is the terminal's own two colours. A host that is not a terminal answers
	// the zero value, and so does a terminal nobody asked.
	//
	// It is here rather than in [Config] because it is a fact about the thing being
	// drawn on, and this is what stands for that thing. A test host that can say it
	// is light is a test that can check a look both ways round.
	Ground() grid.Ground
	// Wheel is what the terminal's wheel reports are worth. A host that is not a
	// terminal answers the zero value, which is the common arrangement.
	Wheel() input.Wheel
	// Keyboard is which Kitty keyboard enhancements are on, and whether the terminal
	// said. A host that cannot ask reports false.
	Keyboard() (input.KeyboardFlags, bool)
	// ReportDirectory tells the terminal where the program is working. A host that is
	// not a terminal does nothing and reports no error: there is nobody to tell.
	ReportDirectory(path string) error
	// Copy puts text on the system clipboard, reporting false for text it will not
	// carry.
	Copy(text string) bool
	// Paste asks for the system clipboard, whose contents arrive later on Events as
	// an [input.Paste]. A host that cannot read a clipboard does nothing.
	//
	// Copying and pasting are the host's because how they are done depends entirely
	// on what is at the other end. A terminal asks the terminal, over a protocol it
	// may refuse; a host somewhere else can shell out to the local tools and be
	// right for that case, which asking the terminal would not be.
	Paste()
}

// Config is what a program needs to run.
//
// Exactly one of Root and Inline says what to run, and which one it is decides
// where the interface is drawn: Root takes a screen of its own, Inline draws in the
// terminal's own screen and prints finished output into its scrollback.
type Config struct {
	// Root builds the component to run on a screen of its own. It is given the loop
	// first, so the component can hold it from the moment it exists.
	Root func(Loop) Component

	// Inline builds the component to run as a block in the terminal's own screen,
	// with output that is finished printed above it. Its component is given an
	// [InlineLoop], which is a [Loop] that can also print.
	Inline func(InlineLoop) Component

	// Terminal says which of the terminal's optional behaviours to ask for. Ignored
	// when Host is set.
	//
	// AltScreen is the program's to decide rather than the caller's, because where
	// frames go is the rendering model and not an input capability: it follows from
	// which of Root and Inline was set. Asking for it alongside Inline is a
	// contradiction and is reported as one.
	Terminal term.Options

	// Color says how much colour the terminal can show. The zero value, [grid.Auto],
	// asks [term.DetectDepth] — which is the one thing in this library that reads
	// its environment rather than making a request and letting it be ignored,
	// because a truecolor sequence a terminal cannot read prints wrong rather than
	// degrading.
	//
	// Setting it is how a program that already knows — from its own configuration,
	// or because it is writing to something that is not a terminal at all — takes
	// that decision back.
	Color grid.Depth

	// Host overrides where input comes from and frames go. Nil opens the real terminal
	// and gives it back on the way out.
	Host Host

	// FrameRate caps how often the interface redraws. Zero uses [DefaultFrameRate].
	FrameRate time.Duration
}

// Run draws the interface until it is asked to stop, its input ends, or the terminal
// fails.
//
// A cancelled context stops the program without being reported as a failure: being
// asked to stop is not one.
func Run(ctx context.Context, cfg Config) (err error) {
	if (cfg.Root == nil) == (cfg.Inline == nil) {
		return errors.New("program: exactly one of Root and Inline is required")
	}
	if cfg.Inline != nil && cfg.Terminal.AltScreen {
		return errors.New("program: an inline interface cannot take the alternate screen")
	}
	opts := cfg.Terminal
	opts.AltScreen = cfg.Root != nil

	host := cfg.Host
	if host == nil {
		terminal, openErr := term.Open(opts)
		if openErr != nil {
			return openErr
		}
		// Giving the terminal back matters more than anything that could go wrong while
		// using it: a terminal left in raw mode is one the user has to close.
		defer func() { err = errors.Join(err, terminal.Close()) }()
		host = terminal
	}

	// The size is asked for once, here, rather than waited for. A program draws before
	// it selects — that is what puts the interface up without the user having to press
	// something — and a first frame drawn onto a screen of no size is a blank terminal.
	width, height, err := host.Size()
	if err != nil {
		return err
	}

	p := &program{
		host:      host,
		writer:    host.Writer(),
		frameRate: cfg.FrameRate,
		tasks:     make(chan func(), 256),
		done:      make(chan struct{}),
	}
	if p.frameRate <= 0 {
		p.frameRate = DefaultFrameRate
	}
	defer close(p.done)
	depth := cfg.Color
	if depth == grid.Auto {
		depth = term.DetectDepth()
	}
	if cfg.Inline != nil {
		p.inline = grid.NewInline(width, height)
		p.inline.SetDepth(depth)
		p.canvas = p.inline
		p.root = cfg.Inline(inlineLoop{loop{p}})
	} else {
		screen := grid.NewScreen(width, height)
		screen.SetDepth(depth)
		p.canvas = screen
		p.root = cfg.Root(loop{p})
	}
	// What the terminal draws with and on goes to the canvas, not just to the
	// component: a cell left at the terminal's own colours has no numbers of its own,
	// and a layer floating over one has to mix with something. This is the only place
	// that knows both the answer and the surface, so it is the only place that can
	// join them — see [grid.Ground].
	p.canvas.SetGround(host.Ground())
	return p.run(ctx)
}

// canvas is somewhere frames go: a screen of the program's own, or a block in the
// terminal's. The program drives both the same way, and the difference between them
// is entirely in how a frame reaches the wire.
type canvas interface {
	Size() (w, h int)
	Resize(w, h int)
	Invalidate()
	SetGround(g grid.Ground)
	Frame() grid.View
	Flush(w io.Writer) error
}

// program is one running interface.
type program struct {
	root   Component
	host   Host
	canvas canvas
	// inline is the canvas again when the interface is drawn in the terminal's own
	// screen, and nil when it has a screen to itself. It is what printing needs and
	// a screen cannot offer.
	inline *grid.Inline
	writer *term.Writer

	present   present.Presenter
	frameRate time.Duration

	// tasks carries work to be run on this goroutine. Its buffer is what absorbs a
	// burst without making the producer wait.
	tasks chan func()
	// done closes when the program stops, so anything waiting to post gives up and
	// anything ticking exits.
	done chan struct{}

	quit bool
}

// run is the event loop.
func (p *program) run(ctx context.Context) error {
	// However this ends — asked to stop, input gone, terminal broken — an inline
	// interface has one more frame to draw and a cursor to leave in a sane place.
	defer p.finish()
	events := p.host.Events()

	// due fires when a frame that was turned away for arriving too soon becomes
	// allowed. Without it the last update of a burst would sit undrawn until something
	// else happened to wake the loop.
	due := time.NewTimer(0)
	if !due.Stop() {
		<-due.C
	}
	armed := false

	p.present.RequestFull()
	for !p.quit {
		p.draw()

		if armed && !due.Stop() {
			<-due.C
		}
		armed = false
		if at, pending := p.present.DueAt(); pending {
			due.Reset(max(time.Until(at), 0))
			armed = true
		}

		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-events:
			if !ok {
				// The input has ended, which is the session ending: a terminal that went
				// away, a pipe that closed.
				return nil
			}
			p.handle(ev)

		case task := <-p.tasks:
			p.apply(task)
			// Whatever else arrived without waiting is applied before drawing, so a
			// burst is one frame rather than one frame each.
			p.drain()

		case <-p.writer.Progress():
			p.present.Wrote(p.writer.Written())
			if err := p.writer.Err(); err != nil {
				// A terminal that has failed a write does not recover, and an interface
				// that cannot reach its terminal has nothing left to do.
				return err
			}

		case <-due.C:
			armed = false
		}
	}
	return nil
}

// apply runs one posted task and asks for a frame.
func (p *program) apply(task func()) {
	if task != nil {
		task()
	}
	p.present.RequestBy(time.Now(), p.frameRate)
}

// drain runs whatever else is already waiting.
func (p *program) drain() {
	for {
		select {
		case task := <-p.tasks:
			p.apply(task)
		default:
			return
		}
	}
}

// handle deals with one terminal event.
//
// Resize and focus are the program's own: the first changes the geometry everything
// else is drawn against, and the second means another program may have written to the
// terminal, so what it is showing can no longer be assumed. Everything else is the
// component's.
func (p *program) handle(ev input.Event) {
	switch e := ev.(type) {
	case input.Resize:
		p.canvas.Resize(e.Width, e.Height)
		p.present.RequestFull()
		return
	case input.FocusIn:
		p.present.RequestFull()
		return
	}
	if p.root.Handle(ev) {
		p.present.RequestBy(time.Now(), p.frameRate)
	}
}

// draw renders a frame, if one is owed and the terminal is keeping up.
func (p *program) draw() {
	p.present.Present(time.Now(), func(full bool) uint64 {
		if full {
			p.canvas.Invalidate()
		}
		p.root.Draw(p.canvas.Frame())
		return p.flush()
	})
}

// flush hands the frame to the writer and returns the sequence it was queued under.
//
// The canvas writes into a buffer rather than straight to the terminal, because the
// write has to happen on the writer's goroutine: that is the whole reason there is one.
func (p *program) flush() uint64 {
	var frame frameBuffer
	// Nothing here can fail — the destination is memory — so the error is the compiler's
	// concern and not this program's.
	_ = p.canvas.Flush(&frame)
	if len(frame.bytes) == 0 {
		return 0
	}
	return p.writer.Queue(frame.bytes)
}

// finish settles what the program leaves behind.
//
// An inline interface's last state is what stays in the terminal, so it is drawn one
// more time and the cursor is left below it — otherwise the shell's next prompt lands
// on top of what the program was showing. The last frame is drawn without asking the
// presenter: pacing is about not drawing more often than a terminal can keep up with,
// and there is no next frame to be too close to. Anything printed but not yet written
// goes out with it, because output the caller asked for must not be lost to the timing
// of the exit.
//
// A program on a screen of its own needs none of that. Giving the screen back takes
// the interface with it, which is what makes that mode simple and this one not.
//
// Both wait for the terminal to catch up, so that Run returning means what the
// program drew has been written. Without it a caller printing its own output next
// would find the program's last frame arriving in the middle of it.
func (p *program) finish() {
	if p.inline != nil {
		p.root.Draw(p.inline.Frame())
		p.flush()

		var tail frameBuffer
		_ = p.inline.Finish(&tail)
		if len(tail.bytes) > 0 {
			p.writer.Queue(tail.bytes)
		}
	}
	p.writer.Drain(term.DrainGrace)
}

// frameBuffer collects one frame's bytes.
type frameBuffer struct{ bytes []byte }

func (f *frameBuffer) Write(b []byte) (int, error) {
	f.bytes = append(f.bytes, b...)
	return len(b), nil
}

// inlineLoop is the program's side of [InlineLoop]. It exists only when there is a
// terminal screen to print into, which is what keeps [InlineLoop.Print] from being a
// method that quietly does nothing half the time.
type inlineLoop struct{ loop }

func (l inlineLoop) Print(p Printable) {
	if p == nil {
		return
	}
	l.Post(func() {
		width, _ := l.p.inline.Size()
		l.p.inline.Print(p.Measure(width), p.Draw)
	})
}

func (l inlineLoop) PrintRows(rows int, draw func(grid.View)) {
	l.Post(func() { l.p.inline.Print(rows, draw) })
}

func (l inlineLoop) Append(draw func(grid.View) bool) {
	if draw == nil {
		return
	}
	l.Post(func() {
		for {
			before := l.room()
			more := false
			l.p.inline.Append(func(v grid.View) { more = draw(v) })
			if !more {
				return
			}
			if l.room() == before && before == 0 {
				// A whole row to itself and nothing drawn into it. No amount of room
				// would help, so asking again is asking forever.
				return
			}
			l.p.inline.Break()
		}
	})
}

// room is how much of the open row has been taken, or zero when the next thing
// published starts a row of its own.
func (l inlineLoop) room() int {
	col, open := l.p.inline.Tail()
	if !open {
		return 0
	}
	return col
}

// loop is the program's side of [Loop]. It is a value so a component can copy it
// freely and hand it to whatever needs it.
type loop struct{ p *program }

func (l loop) Refresh() { l.Post(nil) }

func (l loop) Post(fn func()) {
	select {
	case l.p.tasks <- fn:
	case <-l.p.done:
		// The program has stopped. There is nothing left to show the work on, and
		// blocking the caller for ever would be worse than dropping it.
	}
}

func (l loop) Quit() {
	l.Post(func() { l.p.quit = true })
}

// Ground reads through to the host rather than caching the answer. What the
// terminal said its colours were was settled before the program started and cannot
// change under it, so there is nothing to keep in step.
func (l loop) Ground() grid.Ground { return l.p.host.Ground() }

// Copy and Paste read through to the host. Neither needs the program's goroutine:
// nothing about the interface changes, and the answer to a paste comes back the way
// every other event does.
func (l loop) Copy(text string) bool { return l.p.host.Copy(text) }

func (l loop) Wheel() input.Wheel { return l.p.host.Wheel() }

func (l loop) Keyboard() (input.KeyboardFlags, bool) { return l.p.host.Keyboard() }

func (l loop) ReportDirectory(path string) error { return l.p.host.ReportDirectory(path) }

func (l loop) Paste() { l.p.host.Paste() }

func (l loop) Every(d time.Duration, fn func()) (stop func()) {
	if d <= 0 || fn == nil {
		return func() {}
	}
	stopped := make(chan struct{})
	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.Post(fn)
			case <-stopped:
				return
			case <-l.p.done:
				return
			}
		}
	}()
	// sync.Once rather than a bool: stop is handed out to a caller who may keep it
	// anywhere, and two goroutines reaching a plain flag at once would both close
	// the channel and one would panic.
	var once sync.Once
	return func() { once.Do(func() { close(stopped) }) }
}
