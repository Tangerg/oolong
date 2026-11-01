package program

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestByteIngressBurstOccupiesOneOwnerTask(t *testing.T) {
	tasks := newTaskQueue()
	dispatch := Dispatcher{tasks: tasks}
	var batches []ByteBatch
	ingress, err := NewByteIngress(ByteIngressConfig{Dispatcher: dispatch, Limit: 64, Consume: func(batch ByteBatch) {
		batches = append(batches, batch)
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"one", "two", "three", "four"} {
		if _, err := ingress.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}

	queued := tasks.take()
	if len(queued) != 1 {
		t.Fatalf("burst queued %d owner tasks, want 1", len(queued))
	}
	queued[0]()
	if len(batches) != 1 || string(batches[0].Data) != "onetwothreefour" || batches[0].Final {
		t.Fatalf("batches = %+v", batches)
	}

	if err := ingress.Close(); err != nil {
		t.Fatal(err)
	}
	queued = tasks.take()
	if len(queued) != 1 {
		t.Fatalf("close queued %d owner tasks, want 1", len(queued))
	}
	queued[0]()
	<-ingress.Done()
	if len(batches) != 2 || !batches[1].Final {
		t.Fatalf("batches after close = %+v", batches)
	}
	tasks.stop()
}

func TestByteIngressStopReleasesPendingBytes(t *testing.T) {
	tasks := newTaskQueue()
	ingress, err := NewByteIngress(ByteIngressConfig{Dispatcher: Dispatcher{tasks: tasks}, Limit: 1 << 20, Consume: func(ByteBatch) {
		t.Fatal("stopped ingress invoked its consumer")
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("x", 1<<19)
	if _, err := ingress.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}

	tasks.stop()
	select {
	case <-ingress.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("ingress did not observe its owner stopping")
	}
	ingress.mu.Lock()
	pending, stopped := len(ingress.pending), ingress.stopped
	consumer, reservation, result := ingress.consume, ingress.slot, ingress.result
	ingress.mu.Unlock()
	if pending != 0 {
		t.Fatalf("stopped ingress retained %d pending bytes", pending)
	}
	if !stopped {
		t.Fatal("ingress did not enter its terminal state")
	}
	if consumer != nil || reservation != nil || result != nil {
		t.Fatal("stopped ingress retained references across its owner boundary")
	}
	if _, err := ingress.Write([]byte("later")); !errors.Is(err, ErrStopped) {
		t.Fatalf("Write after owner stop = %v, want ErrStopped", err)
	}
}

func TestByteIngressFinalDeliverySettlesCrossBoundaryReferences(t *testing.T) {
	tasks := newTaskQueue()
	cause := errors.New("source ended")
	var delivered error
	ingress, err := NewByteIngress(ByteIngressConfig{Dispatcher: Dispatcher{tasks: tasks}, Limit: 16, Consume: func(batch ByteBatch) {
		delivered = batch.Err
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ingress.CloseWithError(cause); err != nil {
		t.Fatal(err)
	}
	queued := tasks.take()
	if len(queued) != 1 {
		t.Fatalf("close queued %d tasks, want 1", len(queued))
	}
	queued[0]()
	<-ingress.Done()
	if !errors.Is(delivered, cause) {
		t.Fatalf("delivered error = %v, want source cause", delivered)
	}

	ingress.mu.Lock()
	consumer, reservation, result := ingress.consume, ingress.slot, ingress.result
	ingress.mu.Unlock()
	if consumer != nil || reservation != nil || result != nil {
		t.Fatal("finished ingress retained consumer, queue reservation, or terminal error")
	}
	tasks.stop()
}

func TestFinalIngressConsumerUnwindSettlesBeforeOwnerShutdown(t *testing.T) {
	tasks := newTaskQueue()
	ingress, err := NewByteIngress(ByteIngressConfig{Dispatcher: Dispatcher{tasks: tasks}, Limit: 16, Consume: func(ByteBatch) {
		panic("consumer failed")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ingress.Close(); err != nil {
		t.Fatal(err)
	}
	queued := tasks.take()
	if len(queued) != 1 {
		t.Fatalf("close queued %d tasks, want 1", len(queued))
	}
	func() {
		defer func() { _ = recover() }()
		queued[0]()
	}()

	defer tasks.stop()
	select {
	case <-ingress.Done():
	default:
		t.Fatal("panicking final consumer left Done open until unrelated owner shutdown")
	}
}

func TestIngressAdmissionUsesTheDispatcherEvenBeforeItsShutdownWatcherRuns(t *testing.T) {
	for _, alreadyQueued := range []bool{false, true} {
		q := newTaskQueue()
		slot, err := q.reserve()
		if err != nil {
			t.Fatal(err)
		}
		// Keep the shutdown notification deliberately undelivered. The queue
		// has already revoked the reservation, independently of its observer.
		i := &ByteIngress{slot: slot, limit: 8, done: make(chan struct{}), consume: func(ByteBatch) { t.Fatal("called without an owner") }}
		if alreadyQueued {
			if _, writeErr := i.Write([]byte("a")); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		q.stop()
		if n, writeErr := i.Write([]byte("b")); n != 0 || !errors.Is(writeErr, ErrStopped) {
			t.Errorf("queued=%v: accepted bytes after stop: (%d,%v)", alreadyQueued, n, writeErr)
		}
		if closeErr := i.Close(); !errors.Is(closeErr, ErrStopped) {
			t.Errorf("queued=%v: accepted completion after stop: %v", alreadyQueued, closeErr)
		}
		i.stop()
	}
}

func TestIngressClaimedDrainAlsoOwnsTheFinalBatch(t *testing.T) {
	q := newTaskQueue()
	q.resourceCapacity = 1
	defer q.stop()
	i, err := NewByteIngress(ByteIngressConfig{Dispatcher: Dispatcher{tasks: q}, Limit: 4, Consume: func(batch ByteBatch) {
		if !batch.Final || string(batch.Data) != "ab" {
			t.Errorf("batch=%+v", batch)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = i.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	batch := q.take()
	if _, err = i.Write([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if err = i.Close(); err != nil {
		t.Fatal(err)
	}
	batch[0]()
	<-i.Done()
	slot, err := q.reserve()
	if err != nil {
		t.Fatalf("finished stream retained a redundant queued drain: %v", err)
	}
	slot.close()
}

func TestClosingIngressReleasesEveryBlockedProducerBeforeFinalDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := newTaskQueue()
		defer q.stop()
		i, err := NewByteIngress(ByteIngressConfig{Dispatcher: Dispatcher{tasks: q}, Limit: 1, Consume: func(ByteBatch) {}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := i.Write([]byte("a")); err != nil {
			t.Fatal(err)
		}
		results := make(chan error, 3)
		for range cap(results) {
			go func() { _, writeErr := i.Write([]byte("b")); results <- writeErr }()
		}
		synctest.Wait()
		if err := i.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		for range cap(results) {
			select {
			case err := <-results:
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatal(err)
				}
			default:
				t.Fatal("writers still waiting for the owner to consume after ingress closed")
			}
		}
	})
}
