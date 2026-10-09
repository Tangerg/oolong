package grid

import (
	"errors"
	"io"
)

var errPublicationInterrupted = errors.New("grid: destination write interrupted; publication ended")

// publication keeps composition and settlement with the same owner while external
// drawing, painter and writer callbacks run. Reads and drawing into the supplied
// view are allowed; another operation on the canvas would replace its borrowed state.
type publication struct {
	active  bool
	failure error
}

func (p *publication) requireIdle() {
	if p.active {
		panic("grid: a publication callback cannot mutate or publish the same canvas")
	}
}

func (p *publication) begin() {
	p.requireIdle()
	p.active = true
}

func (p *publication) end() { p.active = false }

func (p *publication) write(w io.Writer, data []byte) error {
	// A writer can panic or exit the goroutine after emitting an unknown prefix.
	// Only a normal return can replace this receipt with success or its own error.
	p.failure = errPublicationInterrupted
	p.failure = writeAll(w, data)
	return p.failure
}
