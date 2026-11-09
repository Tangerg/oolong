package program

import (
	"errors"
	"io"
	"sync"
)

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
// is stopped. A ByteIngress must not be copied after construction: its pending bytes,
// backpressure and final delivery belong to one stream.
type ByteIngress struct {
	slot    *reservation
	limit   int
	consume func(ByteBatch)

	mu      sync.Mutex
	pending []byte
	result  error
	closed  bool
	stopped bool

	room chan struct{}
	done chan struct{}
	once sync.Once
}

// ByteIngressConfig owns the complete construction contract of a [ByteIngress].
// Dispatcher and Consume are required. Limit is the maximum number of bytes accepted
// but not yet taken by the interface owner and must be positive.
type ByteIngressConfig struct {
	Dispatcher Dispatcher
	Limit      int
	Consume    func(ByteBatch)
}

// NewByteIngress refuses a stopped or zero dispatcher because it has no owner
// on which to invoke config.Consume.
func NewByteIngress(config ByteIngressConfig) (*ByteIngress, error) {
	if config.Limit <= 0 {
		return nil, errors.New("program: byte ingress limit must be positive")
	}
	if config.Consume == nil {
		return nil, errors.New("program: byte ingress requires a consumer")
	}
	ownerDone := config.Dispatcher.Done()
	select {
	case <-ownerDone:
		return nil, ErrStopped
	default:
	}

	slot, err := config.Dispatcher.tasks.reserve()
	if err != nil {
		return nil, err
	}
	i := &ByteIngress{
		slot:    slot,
		limit:   config.Limit,
		consume: config.Consume,
		done:    make(chan struct{}),
	}
	go i.waitForOwner(ownerDone)
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
			if i.room == nil {
				i.room = make(chan struct{})
			}
			room := i.room
			i.mu.Unlock()
			<-room
			continue
		}

		if !i.admitDrain() {
			i.mu.Unlock()
			i.stop()
			return n, ErrStopped
		}
		take := min(available, len(p))
		i.pending = append(i.pending, p[:take]...)
		i.mu.Unlock()

		n += take
		p = p[take:]
	}
	return n, nil
}

// Close completes the stream successfully after all accepted bytes are delivered.
func (i *ByteIngress) Close() error { return i.CloseWithError(nil) }

// CloseWithError completes the stream with err after all accepted bytes are
// delivered. The first close wins. Bare [io.EOF] is normalized to successful
// completion; wrapped or joined errors are preserved as failures.
func (i *ByteIngress) CloseWithError(err error) error {
	if i == nil || i.done == nil {
		return ErrStopped
	}
	if err == io.EOF { //nolint:errorlint // Only bare EOF ends a reader successfully; wrapped or joined failures must survive.
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
	if !i.admitDrain() {
		i.mu.Unlock()
		i.stop()
		return ErrStopped
	}
	i.closed = true
	i.result = err
	i.wakeWriters()
	i.mu.Unlock()
	return nil
}

// Done closes after the final batch is consumed or the interface owner stops.
func (i *ByteIngress) Done() <-chan struct{} {
	if i == nil || i.done == nil {
		return closedSignal
	}
	return i.done
}

// With mu held, pending bytes already have one scheduled (possibly taken)
// drain. Only the queue decides whether that drain still has a live owner.
func (i *ByteIngress) admitDrain() bool {
	if len(i.pending) == 0 {
		return i.slot.post(i.drain)
	}
	select {
	case <-i.slot.queue.done:
		return false
	default:
		return true
	}
}

func (i *ByteIngress) drain() bool {
	i.mu.Lock()
	if i.stopped {
		i.mu.Unlock()
		return false
	}
	batch := ByteBatch{Data: i.pending, Err: i.result, Final: i.closed}
	consume := i.consume
	i.pending = nil
	if batch.Final {
		i.stopped = true
		i.releaseTerminalReferences()
	}
	i.wakeWriters()
	i.mu.Unlock()
	if batch.Final {
		defer i.settle()
	}
	if len(batch.Data) > 0 || batch.Final {
		consume(batch)
	}
	return len(batch.Data) > 0 || batch.Final
}

func (i *ByteIngress) waitForOwner(ownerDone <-chan struct{}) {
	select {
	case <-ownerDone:
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
	i.releaseTerminalReferences()
	i.wakeWriters()
	i.mu.Unlock()

	// stopped and settled are deliberately not the same bit. A final consumer runs
	// after stopped is published. Its deferred cleanup or a concurrent dispatcher
	// shutdown must still release every Done waiter exactly once.
	i.settle()
}

// releaseTerminalReferences settles the object graph crossing the ingress boundary.
// It is called under mu only after no future delivery may be admitted.
func (i *ByteIngress) releaseTerminalReferences() {
	i.result = nil
	i.consume = nil
}

// Under mu, release everyone waiting on this capacity epoch. Closing admission
// must wake blocked producers even if the final consumer never takes another turn.
func (i *ByteIngress) wakeWriters() {
	if i.room != nil {
		close(i.room)
		i.room = nil
	}
}

func (i *ByteIngress) settle() {
	i.once.Do(func() {
		i.mu.Lock()
		slot := i.slot
		i.slot = nil
		i.mu.Unlock()
		slot.close()
		close(i.done)
	})
}
