package program

import (
	"errors"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

// InlineRuntime exposes permanent publication only for an inline session. The
// publisher is the unique owner of accepted pending content and its receipts.
type InlineRuntime struct{ *Runtime }

// Publisher is owner-only. A known transport failure closes publication before
// any new content can run its measurement or painting callbacks.
func (r *InlineRuntime) Publisher() *publication.Publisher {
	if r == nil {
		return nil
	}
	p := r.owner()
	if p == nil {
		return nil
	}
	if err := p.writer.Err(); err != nil && p.publisher != nil {
		p.publisher.Fail(err, true)
		p.outputFailed = true
		_ = p.fail(err)
	}
	return p.publisher
}

// Publish freezes and accepts content, then requests a frame. The returned
// receipt distinguishes acceptance from successful terminal writing.
func (r *InlineRuntime) Publish(content grid.Drawable) (publication.Receipt, error) {
	publisher := r.Publisher()
	if publisher == nil {
		if r != nil && r.owner() != nil {
			return publication.Receipt{}, errors.ErrUnsupported
		}
		return publication.Receipt{}, ErrStopped
	}
	receipt, err := publisher.TryPublish(content)
	if err == nil {
		r.Refresh()
	}
	return receipt, err
}
