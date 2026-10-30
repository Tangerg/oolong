package grid

import "slices"

// Output is a prepared terminal transaction. Preparation does not advance the
// diff baseline or inline anchor. The caller admits Bytes to its one writer and
// then commits, or aborts without accepting any bytes. Settlement runs no user code.
// While an Output is unsettled its canvas cannot begin another operation.
// An Output must not be copied after construction.
type Output struct {
	noCopy  noCopy
	payload []byte
	settle  func(bool)
}

// Bytes returns a detached payload which an asynchronous writer may own.
func (o *Output) Bytes() []byte {
	if o == nil {
		return nil
	}
	return slices.Clone(o.payload)
}

// Len is zero for a settled output; it does not allocate a payload copy.
func (o *Output) Len() int {
	if o == nil {
		return 0
	}
	return len(o.payload)
}

// Commit panics if this output was already committed or aborted. It runs only
// after the writer has accepted Bytes, so settlement cannot call user code.
func (o *Output) Commit() {
	if o == nil || o.settle == nil {
		panic("grid: output already settled")
	}
	settle := o.settle
	o.settle, o.payload = nil, nil
	settle(true)
}

// Abort is idempotent and releases the candidate without advancing the canvas.
func (o *Output) Abort() {
	if o == nil || o.settle == nil {
		return
	}
	settle := o.settle
	o.settle, o.payload = nil, nil
	settle(false)
}
