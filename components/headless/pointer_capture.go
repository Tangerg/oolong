package headless

import "github.com/Tangerg/oolong/core/input"

// pointerCapture owns routing for one press. Its proposed recipient is published
// before delivery so synchronous input can cancel it. Every reentrant pointer event
// revokes the interrupted route; a decline may withdraw only its own press receipt.
type pointerCapture[T interface{ deliverPointer(event input.Mouse) bool }] struct {
	target *pointerPress[T]
	turn   uint64
}

// Retain a cancelled receipt through release without leaving the recipient's gesture live.
type pointerPress[T interface{ deliverPointer(event input.Mouse) bool }] struct {
	target    T
	button    input.Button
	cancelled bool
}

func (p *pointerPress[T]) continues(event input.Mouse) bool {
	return p != nil && (event.Action == input.MouseDrag || event.Action == input.MouseUp)
}

func (p *pointerPress[T]) cancel(event input.Mouse) {
	if p.cancelled {
		return
	}
	p.cancelled = true
	event.Action = input.MouseCancel
	p.target.deliverPointer(event)
}

func (c *pointerCapture[T]) begin(event input.Mouse) (*pointerPress[T], uint64) {
	previous := c.target
	c.turn++
	if event.Action == input.MouseDown || event.Action == input.MouseCancel || previous != nil && event.Releases(previous.button) {
		c.target = nil
	}
	turn := c.turn
	if previous != nil && (event.Action == input.MouseDown || event.Action == input.MouseCancel) {
		previous.cancel(event)
	}
	return previous, turn
}

func (c *pointerCapture[T]) current(turn uint64) bool { return c.turn == turn }

func (c *pointerCapture[T]) cancel(turn uint64, press *pointerPress[T], event input.Mouse) bool {
	if c.current(turn) {
		press.cancel(event)
	}
	return c.current(turn)
}

func (c *pointerCapture[T]) deliver(turn uint64, target T, event input.Mouse) bool {
	if !c.current(turn) {
		return false
	}
	if event.Action != input.MouseDown {
		return target.deliverPointer(event)
	}
	press := &pointerPress[T]{target: target, button: event.Button}
	c.target = press
	handled := target.deliverPointer(event)
	if !handled && c.target == press {
		c.target = nil
	}
	return handled
}
