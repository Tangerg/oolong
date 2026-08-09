package term

import (
	"strconv"
	"sync"
	"time"
)

// ProgressState is how a host should present the state of one foreground task.
type ProgressState uint8

const (
	// ProgressNone clears native progress. It is the zero value.
	ProgressNone ProgressState = iota
	// ProgressNormal is work proceeding normally.
	ProgressNormal
	// ProgressError is work that failed.
	ProgressError
	// ProgressIndeterminate is active work whose completion cannot be measured.
	ProgressIndeterminate
	// ProgressWarning is work that completed or paused with a warning.
	ProgressWarning
)

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

func (p Progress) normalized() Progress {
	if p.State > ProgressWarning {
		return Progress{}
	}
	switch p.State {
	case ProgressNone, ProgressIndeterminate:
		p.Percent = 0
	default:
		p.Percent = min(max(p.Percent, 0), 100)
	}
	return p
}

func (p Progress) sequence() string {
	p = p.normalized()
	switch p.State {
	case ProgressNone:
		return progressClear
	case ProgressIndeterminate:
		return progressIndeterminate
	default:
		return progressPrefix + strconv.Itoa(int(p.State)) + ";" +
			strconv.Itoa(p.Percent) + progressEnd
	}
}

const (
	progressPrefix        = "\x1b]9;4;"
	progressEnd           = "\x07"
	progressClear         = progressPrefix + "0" + progressEnd
	progressIndeterminate = progressPrefix + "3" + progressEnd
	progressKeepalive     = time.Second
)

// taskProgress owns the terminal-native progress state and its keepalive.
//
// Some terminals clear OSC 9;4 if it is not refreshed. A ticker therefore exists
// only while progress is active and this process owns the terminal. SetProgress may
// be called from any goroutine, so state changes and keepalive writes share one lock.
type taskProgress struct {
	mu      sync.Mutex
	current Progress
	paused  bool
	dirty   bool
	closed  bool

	wake chan struct{}
	done chan struct{}
}

func newTaskProgress() *taskProgress {
	return &taskProgress{wake: make(chan struct{}, 1), done: make(chan struct{})}
}

// to remembers next and queues it unless another process owns the terminal.
func (p *taskProgress) to(next Progress, queue func([]byte) uint64) {
	next = next.normalized()
	p.mu.Lock()
	if p.closed || next == p.current {
		p.mu.Unlock()
		return
	}
	p.current = next
	if p.paused {
		p.dirty = true
	} else {
		queue([]byte(next.sequence()))
	}
	p.mu.Unlock()
	p.signal()
}

// pause stops keepalives before output is drained for a handover. Once it returns,
// no keepalive write is in flight.
func (p *taskProgress) pause() {
	p.mu.Lock()
	p.paused = true
	p.dirty = false
	p.mu.Unlock()
	p.signal()
}

// restore resumes a handover that never happened. A change made while paused is
// queued now because no direct terminal reacquisition will restate it.
func (p *taskProgress) restore(queue func([]byte) uint64) {
	p.mu.Lock()
	if !p.closed && p.dirty {
		queue([]byte(p.current.sequence()))
	}
	p.paused = false
	p.dirty = false
	p.mu.Unlock()
	p.signal()
}

// resume follows a real terminal reacquisition, whose direct write already restated
// the latest progress.
func (p *taskProgress) resume() {
	p.mu.Lock()
	p.paused = false
	p.dirty = false
	p.mu.Unlock()
	p.signal()
}

// enter restates active progress after a handover. None needs no sequence because
// giving the terminal away already cleared it.
func (p *taskProgress) enter() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current.State == ProgressNone {
		return ""
	}
	return p.current.sequence()
}

// leave clears active progress without forgetting it, so a handover can restore it.
func (p *taskProgress) leave() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current.State == ProgressNone {
		return ""
	}
	return progressClear
}

func (p *taskProgress) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// run refreshes active progress and parks without a timer in every other state.
func (p *taskProgress) run(stop <-chan struct{}, queue func([]byte) uint64) {
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
		active := !p.paused && p.current.State != ProgressNone
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

func (p *taskProgress) repeat(queue func([]byte) uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.paused || p.current.State == ProgressNone {
		return
	}
	queue([]byte(p.current.sequence()))
}

// SetProgress changes task progress outside the cell grid. Unsupported terminals
// ignore it. Repeating an unchanged value writes nothing; active values are refreshed
// often enough for terminals that expire the indicator.
func (t *Terminal) SetProgress(progress Progress) {
	t.task.to(progress, t.writer.Queue)
}
