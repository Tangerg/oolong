package headless

import "github.com/Tangerg/oolong/core/input"

// pointerCapture owns routing for one press. Its proposed recipient is published
// before delivery so synchronous input can cancel it. Only the same press may
// withdraw a declined proposal; an interrupted delivery cannot replace a newer one.
type pointerCapture[T interface{ deliverPointer(event input.Mouse) bool }] struct {
	target *T
	button input.Button
	turn   uint64
}

func (c *pointerCapture[T]) begin(event input.Mouse) (*T, uint64) {
	previous := c.target
	if event.Action == input.MouseDown || event.Action == input.MouseCancel || event.Releases(c.button) {
		c.target = nil
		c.turn++
	}
	if event.Action == input.MouseDown {
		c.button = event.Button
	}
	turn := c.turn
	if previous != nil && (event.Action == input.MouseDown || event.Action == input.MouseCancel) {
		cancel := event
		cancel.Action = input.MouseCancel
		(*previous).deliverPointer(cancel)
	}
	return previous, turn
}

func (c *pointerCapture[T]) current(turn uint64) bool { return c.turn == turn }

func (c *pointerCapture[T]) offer(turn uint64, target T, deliver func() bool) bool {
	if !c.current(turn) {
		return false
	}
	c.target = &target
	handled := deliver()
	if !handled && c.current(turn) {
		c.target = nil
	}
	return handled
}
