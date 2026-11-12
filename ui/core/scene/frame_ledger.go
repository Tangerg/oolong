package scene

// frameLedger matches committed frames to reported writer completion in either
// arrival order. Only a confirmed prefix can release frame capacity or route input.
type frameLedger struct {
	inFlight []*layoutSnapshot
	written  *layoutSnapshot
	// Nil distinguishes no confirmation from a confirmed zero-byte prefix.
	completion *uint64
}

// admit records a committed frame. A frame committed under the sequence of the
// newest frame in flight replaces it: the writer will never cover the older one.
func (l *frameLedger) admit(frame *layoutSnapshot) {
	if n := len(l.inFlight); n != 0 && l.inFlight[n-1].sequence == frame.sequence {
		l.inFlight[n-1] = frame
	} else {
		l.inFlight = append(l.inFlight, frame)
	}
	l.settle()
}

// wrote advances writer progress and reports whether anything changed.
func (l *frameLedger) wrote(sequence uint64) bool {
	if l.completion != nil && sequence < *l.completion {
		return false
	}
	changed := l.completion == nil || sequence != *l.completion
	if l.completion == nil {
		l.completion = new(uint64)
	}
	*l.completion = sequence
	return l.settle() || changed
}

func (l *frameLedger) writtenSequence() uint64 {
	if l.completion == nil {
		return 0
	}
	return *l.completion
}

func (l *frameLedger) settle() bool {
	if l.completion == nil {
		return false
	}
	covered := 0
	for covered < len(l.inFlight) && l.inFlight[covered].sequence <= *l.completion {
		covered++
	}
	if covered == 0 {
		return false
	}
	l.written = l.inFlight[covered-1]
	clear(l.inFlight[:covered])
	l.inFlight = l.inFlight[covered:]
	if len(l.inFlight) == 0 {
		l.inFlight = nil
	}
	return true
}

// revoke forgets every frame's geometry while keeping writer progress, which
// stays true of the transport.
func (l *frameLedger) revoke() {
	l.inFlight, l.written = nil, nil
}
