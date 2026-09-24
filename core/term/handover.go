package term

import (
	"sync"
	"time"
)

// parkGrace is how long a handover waits for the reader to come off the terminal.
//
// It normally takes no time at all: the reader waits for the terminal to have
// something to say before it reads, so waking it is enough. The wait is for the
// one case that is not instant — the reader holding a chunk nothing downstream is
// taking, because the interface's own goroutine is inside this call and is not
// draining events. Going ahead anyway is the right answer there: the cost is one
// chunk of input read by this process instead of by the child, and the cost of
// waiting for ever is a program that cannot be got out of.
const parkGrace = 100 * time.Millisecond

// handover is the reader's side of giving the terminal up: a latch it parks on,
// and a signal saying it has.
//
// It is a latch rather than a flag because parking has to be a wait. A flag would
// be read between two reads and leave the reader inside the next one, which is
// exactly where it must not be: a byte read there is a byte the child never sees.
type handover struct {
	mu sync.Mutex
	// resume is closed to let the reader go on. Nil when the terminal is ours.
	resume chan struct{}
	// parked is closed by the reader once it has stopped reading, and nil once that
	// has happened, so it cannot be closed twice.
	parked chan struct{}
}

// hold takes the terminal away from the reader and returns the channel that closes
// once the reader has noticed.
func (h *handover) hold() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resume = make(chan struct{})
	h.parked = make(chan struct{})
	return h.parked
}

// release gives it back. It is safe to call when nothing was ever held, which is
// what makes a failed handover recoverable by simply resuming.
func (h *handover) release() {
	h.mu.Lock()
	resume := h.resume
	h.resume, h.parked = nil, nil
	h.mu.Unlock()
	if resume != nil {
		close(resume)
	}
}

// park is what the reader calls before each read. It returns at once unless the
// terminal has been handed over, and otherwise waits until it comes back — or
// until the session ends, because a terminal handed over and never taken back must
// not keep a goroutine alive for ever.
func (h *handover) park(stop <-chan struct{}) {
	h.mu.Lock()
	resume, parked := h.resume, h.parked
	if resume != nil {
		// Answered once: the signal is what a handover waits on, and closing it twice
		// would be a panic on the second read of the terminal.
		h.parked = nil
	}
	h.mu.Unlock()

	if resume == nil {
		return
	}
	if parked != nil {
		close(parked)
	}
	select {
	case <-resume:
	case <-stop:
	}
}
