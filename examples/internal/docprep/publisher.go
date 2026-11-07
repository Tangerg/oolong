package docprep

import (
	"slices"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
)

// Publisher refuses a cold layout before the permanent-output owner captures it.
// A resize cannot accidentally move first-time Markdown layout onto the UI owner
// through retirement, or publish a stale-width projection as complete content.
type Publisher struct {
	queue     *Queue
	source    base.TranscriptPublisher
	listeners []*listener
}

type listener struct{ fn, stop func() }

func (q *Queue) Publisher(source base.TranscriptPublisher) *Publisher {
	if source == nil || q.publisher != nil || q.closed {
		panic("docprep: one non-nil publisher is required")
	}
	p := &Publisher{queue: q, source: source}
	q.publisher = p
	return p
}

func (p *Publisher) TryPublish(body grid.Drawable) (publication.Receipt, error) {
	if p.queue == nil || p.queue.closed {
		return publication.Receipt{}, program.ErrScopeClosed
	}
	if p.queue.activeLayout() != nil {
		return publication.Receipt{}, publication.ErrCapacity
	}
	width, _ := p.queue.runtime.Size()
	if width == 0 {
		return publication.Receipt{}, program.ErrStopped
	}
	layout, ready := preparedLayout(body, width)
	if !ready {
		return publication.Receipt{}, publication.ErrCapacity
	}
	return p.source.TryPublish(layout)
}

func (p *Publisher) Subscribe(fn func()) func() {
	if fn == nil || p.queue == nil || p.queue.closed {
		return func() {}
	}
	entry := &listener{fn: fn}
	p.listeners = append(p.listeners, entry)
	entry.stop = p.source.Subscribe(fn)
	return func() {
		if entry.stop == nil {
			return
		}
		entry.fn = nil
		stop := entry.stop
		entry.stop = nil
		stop()
		p.listeners = slices.DeleteFunc(p.listeners, func(current *listener) bool { return current == entry })
	}
}

func (p *Publisher) close() {
	for _, entry := range p.listeners {
		entry.fn = nil
		if entry.stop != nil {
			stop := entry.stop
			entry.stop = nil
			stop()
		}
	}
	p.listeners, p.source, p.queue = nil, nil, nil
}

func (p *Publisher) notify() {
	for _, entry := range slices.Clone(p.listeners) {
		if entry.fn != nil {
			entry.fn()
		}
	}
}
