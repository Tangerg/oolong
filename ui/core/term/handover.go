package term

import (
	"errors"
	"sync"
	"time"
)

// ErrHandoverActive means another Hand already owns terminal reacquisition.
var ErrHandoverActive = errors.New("term: terminal handover already active")

// An input queue waiting on the UI owner cannot drain while that owner is
// handing over the terminal. Refuse the handover before changing modes in
// that case: only the reader can confirm it stopped consuming keystrokes.
const parkGrace = 100 * time.Millisecond

// handover parks the sole reader before input ownership changes. A flag
// checked between reads would leave a race in which the next read steals the
// child's keystroke.
type handover struct {
	mu sync.Mutex
	// resume is closed to let the reader go on. Nil when no handover is claimed.
	resume chan struct{}
	// Parking is requested only after terminal replies have settled. The sole
	// reader closes parked once before blocking until this claim is released.
	parked chan struct{}
}

// hold claims the handover lifetime before any modes or reader state can change.
// Input remains available for outstanding replies until pause requests parking.
func (h *handover) hold() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.resume != nil {
		return false
	}
	h.resume = make(chan struct{})
	return true
}

func (h *handover) pause() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.parked = make(chan struct{})
	return h.parked
}

// release ends the claim after restoration or rollback, including a child panic.
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
	h.mu.Unlock()

	if parked == nil {
		return
	}
	close(parked)
	select {
	case <-resume:
	case <-stop:
	}
}
