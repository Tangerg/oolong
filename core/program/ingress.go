package program

import (
	"errors"
	"io"
	"sync"
)

// ErrStopped means an ingress lost its interface owner before all accepted data
// could be applied. Pending data is deliberately released: cancellation ends the
// live interface and does not turn unconsumed input into published output.
var ErrStopped = errors.New("program: stopped")

// ByteBatch is one owner-side delivery from a [ByteIngress].
//
// Data is the ordered concatenation of bytes accepted since the previous delivery.
// The consumer owns it and may retain or change it after the callback returns. Final
// is true exactly once, after every accepted byte; Err is meaningful only then. A nil
// Err is successful completion.
type ByteBatch struct {
	Data  []byte
	Err   error
	Final bool
}

// ByteIngress carries a lossless ordered byte stream to an interface owner.
//
// Write may block when limit bytes are waiting for the owner, applying backpressure
// at the producer rather than growing [Dispatcher]'s general task queue. Adjacent
// writes are combined and at most one delivery task is pending at a time. The
// consumer is always called on the interface goroutine.
//
// Close or CloseWithError completes the stream after all accepted bytes. When the
// program stops first, blocked writes return [ErrStopped], pending bytes are released,
// and the consumer is not called from a background goroutine. A ByteIngress must be
// closed by its producer; its internal cancellation waiter then exits. The zero value
// is stopped.
type ByteIngress struct {
	dispatch Dispatcher
	limit    int
	consume  func(ByteBatch)

	mu      sync.Mutex
	pending []byte
	result  error
	closed  bool
	posted  bool
	stopped bool

	room chan struct{}
	done chan struct{}
	once sync.Once
}

// NewByteIngress makes a bounded byte ingress.
//
// limit is the maximum number of bytes accepted but not yet taken by the interface
// owner. It must be positive. consume must be non-nil. A stopped or zero dispatcher
// is refused because it has no owner on which to invoke consume.
func NewByteIngress(dispatch Dispatcher, limit int, consume func(ByteBatch)) (*ByteIngress, error) {
	if limit <= 0 {
		return nil, errors.New("program: byte ingress limit must be positive")
	}
	if consume == nil {
		return nil, errors.New("program: byte ingress requires a consumer")
	}
	select {
	case <-dispatch.Done():
		return nil, ErrStopped
	default:
	}

	i := &ByteIngress{
		dispatch: dispatch,
		limit:    limit,
		consume:  consume,
		room:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go i.waitForOwner()
	return i, nil
}

// Write accepts p in order, blocking while limit bytes are pending. It copies p; the
// caller may reuse p after Write returns. A partial count is accompanied by an error.
func (i *ByteIngress) Write(p []byte) (n int, err error) {
	if i == nil || i.done == nil {
		return 0, ErrStopped
	}
	for len(p) > 0 {
		i.mu.Lock()
		switch {
		case i.closed:
			i.mu.Unlock()
			return n, io.ErrClosedPipe
		case i.stopped:
			i.mu.Unlock()
			return n, ErrStopped
		}

		available := i.limit - len(i.pending)
		if available == 0 {
			i.mu.Unlock()
			select {
			case <-i.room:
				continue
			case <-i.done:
				return n, i.writeErr()
			}
		}

		take := min(available, len(p))
		i.pending = append(i.pending, p[:take]...)
		post := !i.posted
		i.posted = true
		i.mu.Unlock()

		n += take
		p = p[take:]
		if post && !i.dispatch.post(i.drain) {
			i.stop()
			return n, ErrStopped
		}
	}
	return n, nil
}

// Close completes the stream successfully after all accepted bytes are delivered.
func (i *ByteIngress) Close() error { return i.CloseWithError(nil) }

// CloseWithError completes the stream with err after all accepted bytes are
// delivered. The first close wins. [io.EOF] is normalized to successful completion.
func (i *ByteIngress) CloseWithError(err error) error {
	if i == nil || i.done == nil {
		return ErrStopped
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}

	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return io.ErrClosedPipe
	}
	if i.stopped {
		i.mu.Unlock()
		return ErrStopped
	}
	i.closed = true
	i.result = err
	post := !i.posted
	i.posted = true
	i.mu.Unlock()

	i.signalRoom()
	if post && !i.dispatch.post(i.drain) {
		i.stop()
		return ErrStopped
	}
	return nil
}

// Done closes after the final batch is consumed or the interface owner stops.
func (i *ByteIngress) Done() <-chan struct{} {
	if i == nil || i.done == nil {
		return closedSignal
	}
	return i.done
}

func (i *ByteIngress) drain() {
	i.mu.Lock()
	if i.stopped {
		i.mu.Unlock()
		return
	}
	batch := ByteBatch{Data: i.pending, Err: i.result, Final: i.closed}
	i.pending = nil
	i.posted = false
	if batch.Final {
		i.stopped = true
	}
	i.mu.Unlock()

	i.signalRoom()
	if len(batch.Data) > 0 || batch.Final {
		i.consume(batch)
	}
	if batch.Final {
		i.settle()
	}
}

func (i *ByteIngress) waitForOwner() {
	select {
	case <-i.dispatch.Done():
		i.stop()
	case <-i.done:
	}
}

func (i *ByteIngress) stop() {
	i.mu.Lock()
	if !i.stopped {
		i.stopped = true
		clear(i.pending)
		i.pending = nil
	}
	i.mu.Unlock()

	// stopped and settled are deliberately not the same bit. A final consumer runs
	// after stopped is published; if it panics, the dispatcher shutdown path arrives
	// here with stopped already true and must still release every Done waiter.
	i.signalRoom()
	i.settle()
}

func (i *ByteIngress) writeErr() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return io.ErrClosedPipe
	}
	return ErrStopped
}

func (i *ByteIngress) signalRoom() {
	select {
	case i.room <- struct{}{}:
	default:
	}
}

func (i *ByteIngress) settle() { i.once.Do(func() { close(i.done) }) }
