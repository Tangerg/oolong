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
	next = next.normalized()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	if next == p.current {
		return nil
	}
	if p.paused {
		p.current = next
		p.dirty = true
	} else {
		if _, err := queue([]byte(next.sequence())); err != nil {
			return err
		}
		p.current = next
		p.published = next.State != ProgressNone
	}
	p.signal()
	return nil
}

// pause stops keepalives before output is drained for a handover. Once it returns,
// no keepalive write is in flight.
func (p *taskProgress) pause() {
	p.mu.Lock()
	p.paused = true
	p.dirty = p.current.State != ProgressNone
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
	return p.publish(queue)
}

// leave clears active progress without forgetting it, so a handover can restore it.
func (p *taskProgress) leave() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.published {
		return ""
	}
	p.published = false
	return progressClear
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
	_ = p.publish(queue) // A full queue retains dirty state for the next keepalive.
}

// Admission creates or settles the cleanup obligation; a refused proposal cannot
// replace what the writer already owns. The caller holds mu through admission.
func (p *taskProgress) publish(queue func([]byte) (uint64, error)) error {
	if _, err := queue([]byte(p.current.sequence())); err != nil {
		p.dirty = true
		return err
	}
	p.published = p.current.State != ProgressNone
	p.dirty = false
	return nil
}
