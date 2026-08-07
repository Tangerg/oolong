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
	ingress, err := NewByteIngress(dispatch, 64, func(batch ByteBatch) {
		batches = append(batches, batch)
	})
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
	ingress, err := NewByteIngress(Dispatcher{tasks: tasks}, 1<<20, func(ByteBatch) {
		t.Fatal("stopped ingress invoked its consumer")
	})
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
	ingress.mu.Unlock()
	if pending != 0 {
		t.Fatalf("stopped ingress retained %d pending bytes", pending)
	}
	if !stopped {
		t.Fatal("ingress did not enter its terminal state")
	}
	if _, err := ingress.Write([]byte("later")); !errors.Is(err, ErrStopped) {
		t.Fatalf("Write after owner stop = %v, want ErrStopped", err)
	}
}

func TestByteIngressOwnerShutdownSettlesAPanickingFinalConsumer(t *testing.T) {
	tasks := newTaskQueue()
	ingress, err := NewByteIngress(Dispatcher{tasks: tasks}, 16, func(ByteBatch) {
		panic("consumer failed")
	})
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
