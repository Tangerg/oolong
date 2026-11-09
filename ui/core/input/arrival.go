package input

import "time"

type arrival struct {
	started, completed time.Time
}

type receivedEvent struct {
	event Event
	arrival
}

type arrivedBytes struct {
	count int
	at    time.Time
}

func eventsOnly(received []receivedEvent) []Event {
	var events []Event
	if len(received) > 0 {
		events = make([]Event, len(received))
		for i, item := range received {
			events[i] = item.event
		}
	}
	return events
}

func (p *Parser) buffer(chunk []byte, at time.Time) {
	if len(chunk) == 0 {
		return
	}
	p.buf = append(p.buf, chunk...)
	if last := len(p.arrivals) - 1; last >= 0 && p.arrivals[last].at.Equal(at) {
		p.arrivals[last].count += len(chunk)
		return
	}
	p.arrivals = append(p.arrivals, arrivedBytes{count: len(chunk), at: at})
}

// Consuming the framing bytes advances their provenance in the same operation.
// Retained payload may disappear at its budget while this unit's origin survives.
func (p *Parser) consumeArrival(n int) {
	for n > 0 {
		part := &p.arrivals[0]
		if !p.hasOrigin {
			p.origin.started = part.at
			p.hasOrigin = true
		}
		p.origin.completed = part.at
		consumed := min(n, part.count)
		n -= consumed
		part.count -= consumed
		if part.count == 0 {
			*part = arrivedBytes{}
			p.arrivals = p.arrivals[1:]
		}
	}
	if len(p.arrivals) == 0 {
		p.arrivals = nil
	}
}
