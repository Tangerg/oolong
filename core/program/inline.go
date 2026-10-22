package program

import (
	"errors"

	"github.com/Tangerg/oolong/core/grid"
)

// InlineRuntime is a [Runtime] that can publish completed output into terminal
// scrollback. It is only constructed for [Config.Inline], and its zero value is
// inert and refuses publication with ErrStopped.
// Print and Append callbacks may draw into their supplied view but cannot publish
// more output or mutate the same inline canvas; reentrant publication panics.
type InlineRuntime struct{ *Runtime }

// inlineCanvas is the publication surface when this is a live inline runtime.
// Keeping the embedded-runtime and mode checks together makes every publishing
// operation share the same zero-value semantics.
func (r *InlineRuntime) inlineCanvas() (*grid.Inline, error) {
	if r == nil {
		return nil, ErrStopped
	}
	p := r.owner()
	if p == nil {
		return nil, ErrStopped
	}
	if p.inline == nil {
		return nil, errors.ErrUnsupported
	}
	if p.frameFailed || p.outputFailed {
		return nil, ErrDisplayFailed
	}
	if err := p.writer.Err(); err != nil {
		p.outputFailed = true
		return nil, p.fail(err)
	}
	return p.inline, nil
}

// Print accepts a measured drawable for publication above an inline interface.
// Nil reports acceptance into the publication buffer, not terminal delivery; Run
// reports later transport failures. A zero or stopped runtime returns ErrStopped,
// a non-inline runtime returns errors.ErrUnsupported, and a failed display refuses
// further output. An unavailable runtime does not measure or draw p. Nil p is an
// accepted no-op only while publication is available.
// Content using native paint regions is drawn privately and then refused with
// errors.ErrUnsupported; those regions require a live frame owner.
func (r *InlineRuntime) Print(p grid.Drawable) error {
	inline, err := r.inlineCanvas()
	if err != nil {
		return err
	}
	return inline.Print(p)
}

// Append continues the last published row until draw reports completion.
// The whole call is accepted or refused together on the same terms as Print.
// A native paint region returns errors.ErrUnsupported; requesting another row without
// drawing in a whole empty row returns io.ErrNoProgress. Neither accepts earlier rows
// from that call. A nil draw is an accepted no-op on a live inline runtime.
func (r *InlineRuntime) Append(draw func(grid.View) bool) error {
	inline, err := r.inlineCanvas()
	if err != nil {
		return err
	}
	return inline.Append(draw)
}
