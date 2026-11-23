package term

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// ProgressState values are the state parameters of OSC 9;4, so a state crosses
// the wire as its own number.
type ProgressState uint8

const (
	ProgressNone ProgressState = iota
	ProgressNormal
	ProgressError
	ProgressIndeterminate
	ProgressWarning
)

var ErrInvalidProgressState = errors.New("term: invalid progress state")

func (s ProgressState) Validate() error {
	if s > ProgressWarning {
		return fmt.Errorf("%w: %d", ErrInvalidProgressState, s)
	}
	return nil
}

// Progress is task progress shown by the terminal outside the cell grid, such as
// in its window or taskbar. Percent is clamped to 0–100 and ignored for None and
// Indeterminate. The zero value clears an earlier value.
//
// It is deliberately distinct from a progress component drawn inside an interface:
// native progress remains useful while the terminal window is obscured and belongs
// to the terminal session's lifecycle rather than to layout or theme.
type Progress struct {
	State   ProgressState
	Percent int
}

const fullPercent = 100

func (p Progress) normalized() Progress {
	switch p.State {
	case ProgressNone, ProgressIndeterminate:
		p.Percent = 0
	default:
		p.Percent = min(max(p.Percent, 0), fullPercent)
	}
	return p
}

// sequence omits the percentage for states that ignore it.
func (p Progress) sequence() string {
	p = p.normalized()
	state := progressPrefix + strconv.Itoa(int(p.State))
	switch p.State {
	case ProgressNone, ProgressIndeterminate:
		return state + commandEnd
	default:
		return state + ";" + strconv.Itoa(p.Percent) + commandEnd
	}
}

// progressPrefix is ConEmu's task progress, the subcommand 4 of OSC 9.
var progressPrefix = ansi.OSCPrefix(ansi.OSCNotify) + "4;"

const progressKeepalive = time.Second

// taskProgress owns the terminal-native progress state and its keepalive.
//
// Some terminals clear OSC 9;4 if it is not refreshed. A ticker therefore exists
// only while progress is active and this process owns the terminal. SetProgress may
// be called from any goroutine, so state changes and keepalive writes share one lock.
type taskProgress struct {
	mu        sync.Mutex
	current   Progress
	published bool
	paused    bool
	dirty     bool
	closed    bool

	wake chan struct{}
	done chan struct{}
}

func newTaskProgress() *taskProgress {
	return &taskProgress{wake: make(chan struct{}, 1), done: make(chan struct{})}
}

func (p *taskProgress) to(next Progress, queue func([]byte) (uint64, error)) error {
	if err := next.State.Validate(); err != nil {
		return err
	}
	next = next.normalized()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	if next == p.current && !p.dirty {
		return nil
	}
	if p.paused {
		p.current = next
		p.dirty = true
	} else if err := p.publish(next, queue); err != nil {
		return err
	}
	p.signal()
	return nil
}

// pause stops keepalives before output is drained for a handover. Once it returns,
// no keepalive write is in flight.
func (p *taskProgress) pause() {
	p.mu.Lock()
	p.paused = true
	p.dirty = p.dirty || p.current.State != ProgressNone
	p.mu.Unlock()
	p.signal()
}

// restore resumes a handover that never happened. A change made while paused is
// queued now because no direct terminal reacquisition will restate it.
func (p *taskProgress) restore(queue func([]byte) (uint64, error)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
	defer p.signal()
	if p.closed {
		return ErrClosed
	}
	if !p.dirty {
		return nil
	}
	return p.publish(p.current, queue)
}

// Building cleanup bytes cannot discharge the obligation: their admission can still fail.
func (p *taskProgress) leave() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.published {
		return ""
	}
	return Progress{}.sequence()
}

func (p *taskProgress) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *taskProgress) run(stop <-chan struct{}, queue func([]byte) (uint64, error)) {
	defer close(p.done)
	var ticker *time.Ticker
	var ticks <-chan time.Time
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
	}()

	for {
		select {
		case <-p.wake:
		case <-ticks:
			p.repeat(queue)
		case <-stop:
			return
		}

		p.mu.Lock()
		active := !p.paused && (p.current.State != ProgressNone || p.dirty)
		p.mu.Unlock()
		switch {
		case active && ticker == nil:
			ticker = time.NewTicker(progressKeepalive)
			ticks = ticker.C
		case !active && ticker != nil:
			ticker.Stop()
			ticker = nil
			ticks = nil
		}
	}
}

func (p *taskProgress) repeat(queue func([]byte) (uint64, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.paused || (p.current.State == ProgressNone && !p.dirty) {
		return
	}
	_ = p.publish(p.current, queue) // A full queue retains dirty state for the next keepalive.
}

// Admission creates or settles the cleanup obligation; a refused proposal cannot
// replace what the writer already owns. The caller holds mu through admission.
func (p *taskProgress) publish(next Progress, queue func([]byte) (uint64, error)) error {
	if _, err := queue([]byte(next.sequence())); err != nil {
		return err
	}
	p.current = next
	p.published = next.State != ProgressNone
	p.dirty = false
	return nil
}

// SetProgress changes task progress outside the cell grid. Unsupported terminals
// ignore it. Repeating an unchanged value writes nothing; active values are refreshed
// often enough for terminals that expire the indicator.
// An unknown state returns [ErrInvalidProgressState] without replacing accepted progress.
func (t *Terminal) SetProgress(progress Progress) error {
	return t.task.to(progress, t.writer.Queue)
}
