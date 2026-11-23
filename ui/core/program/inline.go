package program

import (
	"errors"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

// InlineRuntime exposes permanent publication only for an inline session.
type InlineRuntime struct{ *Runtime }

// Publisher grants producer-side access to permanent output on the UI owner.
// The compositor retains exclusive authority over geometry, writer admission and settlement.
func (r *InlineRuntime) Publisher() Publisher {
	if r == nil {
		return Publisher{}
	}
	return Publisher{runtime: r.Runtime}
}

// Publisher offers complete content and observes progress without granting writer
// authority. Retained copies follow the session lifetime; the zero value is stopped.
type Publisher struct{ runtime *Runtime }

// TryPublish accepts a complete capture and schedules its frame. A stopped session,
// or a known writer failure, refuses before measurement or drawing. A written receipt
// is settled from the actual writer watermark, including on a later transport failure.
func (publisher Publisher) TryPublish(body grid.Drawable) (publication.Receipt, error) {
	p := publisher.runtime.owner()
	if p == nil {
		return publication.Receipt{}, ErrStopped
	}
	if p.publisher == nil {
		return publication.Receipt{}, errors.ErrUnsupported
	}
	if err := p.writer.Err(); err != nil {
		p.failOutput(err)
		_ = p.fail(err)
	}
	receipt, err := p.publisher.TryPublish(body)
	if err == nil {
		publisher.runtime.Refresh()
	}
	return receipt, err
}

// Subscribe observes settled progress on the UI owner. The returned cancellation
// is idempotent; stopped and non-inline sessions register no callback.
func (publisher Publisher) Subscribe(fn func()) func() {
	if p := publisher.runtime.owner(); p != nil && p.publisher != nil {
		return p.publisher.Subscribe(fn)
	}
	return func() {}
}
