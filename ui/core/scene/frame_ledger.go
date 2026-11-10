package scene

// frameLedger follows committed frames through the writer. A frame is in flight
// from its commit until the writer reports its sequence; the last frame the
// writer covered is the geometry physical input is matched against.
type frameLedger struct {
	inFlight []*layoutSnapshot
	written  *layoutSnapshot
	// sequence is the writer progress last reported, which may run ahead of
	// every frame when output that was not a frame was written too.
	sequence uint64
}

// admit records a committed frame. A frame committed under the sequence of the
// newest frame in flight replaces it: the writer will never cover the older one.
func (l *frameLedger) admit(frame *layoutSnapshot) {
	if n := len(l.inFlight); n != 0 && l.inFlight[n-1].sequence == frame.sequence {
		l.inFlight[n-1] = frame
		return
	}
	l.inFlight = append(l.inFlight, frame)
}

// wrote advances writer progress and reports whether anything changed.
func (l *frameLedger) wrote(sequence uint64) bool {
	if sequence < l.sequence {
		return false
	}
	covered := 0
	for covered < len(l.inFlight) && l.inFlight[covered].sequence <= sequence {
		covered++
	}
	if covered == 0 && sequence == l.sequence {
		return false
	}
	if covered > 0 {
		l.written = l.inFlight[covered-1]
		clear(l.inFlight[:covered])
		l.inFlight = l.inFlight[covered:]
		if len(l.inFlight) == 0 {
			l.inFlight = nil
		}
	}
	l.sequence = sequence
	return true
}

// revoke forgets every frame's geometry while keeping writer progress, which
// stays true of the transport.
func (l *frameLedger) revoke() {
	l.inFlight, l.written = nil, nil
}
