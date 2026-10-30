package program

import (
	"errors"
	"strings"
	"testing"
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
	consumer, dispatcher, result := ingress.consume, ingress.dispatch.tasks, ingress.result
	ingress.mu.Unlock()
	if pending != 0 {
		t.Fatalf("stopped ingress retained %d pending bytes", pending)
	}
	if !stopped {
		t.Fatal("ingress did not enter its terminal state")
	}
	if consumer != nil || dispatcher != nil || result != nil {
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
	consumer, dispatcher, result := ingress.consume, ingress.dispatch.tasks, ingress.result
	ingress.mu.Unlock()
	if consumer != nil || dispatcher != nil || result != nil {
		t.Fatal("finished ingress retained consumer, dispatcher, or terminal error")
	}
	tasks.stop()
}

func TestByteIngressOwnerShutdownSettlesAPanickingFinalConsumer(t *testing.T) {
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

	tasks.stop()
	select {
	case <-ingress.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("panicking final consumer left Done open after owner shutdown")
	}
}
