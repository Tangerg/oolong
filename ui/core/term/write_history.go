package term

import (
	"slices"
	"sync"
	"time"
)

// Only the FIFO worker advances this evidence. Readers can prove a settled
// prefix between writes, but a timestamp inside a write cannot identify what
// the terminal displayed while that frame was only partly delivered.
type writeHistory struct {
	mu      sync.RWMutex
	opened  time.Time
	limit   int
	written uint64
	records []writeRecord
}

type writeRecord struct {
	sequence           uint64
	started, completed time.Time
}

func (h *writeHistory) begin(sequence uint64, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == h.limit {
		h.records = slices.Delete(h.records, 0, 1)
	}
	h.records = append(h.records, writeRecord{sequence: sequence, started: at})
}

func (h *writeHistory) complete(sequence uint64, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sequence != h.written+1 {
		return
	}
	h.records[len(h.records)-1].completed = at
	h.written = sequence
}

func (h *writeHistory) latest() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.written
}

func (h *writeHistory) at(at, now time.Time) (uint64, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if at.IsZero() || !at.After(h.opened) || at.After(now) {
		return 0, false
	}
	for _, record := range slices.Backward(h.records) {
		if at.Before(record.started) {
			continue
		}
		if record.completed.IsZero() || !at.After(record.completed) {
			return 0, false
		}
		return record.sequence, true
	}
	return 0, len(h.records) == 0 || h.records[0].sequence == 1
}
