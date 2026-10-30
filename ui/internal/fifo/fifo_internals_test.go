package fifo

import "testing"

func TestQueuePreservesOrderAcrossSingleAndBatchTakes(t *testing.T) {
	var queue Queue[int]
	for value := range 6 {
		queue.Push(value)
	}
	if got, ok := queue.Pop(); !ok || got != 0 {
		t.Fatalf("Pop = %d, %t; want 0, true", got, ok)
	}
	batch := queue.Take(3)
	if len(batch) != 3 || batch[0] != 1 || batch[1] != 2 || batch[2] != 3 {
		t.Fatalf("Take = %v, want [1 2 3]", batch)
	}
	batch[0] = 99
	if got, ok := queue.Pop(); !ok || got != 4 {
		t.Fatalf("Pop after caller mutated its batch = %d, %t; want 4, true", got, ok)
	}
	if got := queue.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1", got)
	}
	if got := queue.Take(-1); got != nil {
		t.Fatalf("Take with a negative limit = %v, want nil", got)
	}
	if got := queue.Take(99); len(got) != 1 || got[0] != 5 {
		t.Fatalf("Take past the end = %v, want [5]", got)
	}
	if got, ok := queue.Pop(); ok || got != 0 {
		t.Fatalf("empty Pop = %d, %t; want 0, false", got, ok)
	}
}

func TestQueueReleasesARecededBurst(t *testing.T) {
	type payload struct{ value *int }
	var queue Queue[payload]
	for value := range 4096 {
		queue.Push(payload{value: &value})
	}
	old := queue.items
	queue.Take(4088)
	for i, item := range old[:4088] {
		if item.value != nil {
			t.Fatalf("removed slot %d still retains its payload", i)
		}
	}
	if queue.Len() != 8 || cap(queue.items) > 80 {
		t.Fatalf("receded queue has len %d cap %d, want 8 and at most 80", queue.Len(), cap(queue.items))
	}

	queue.Clear()
	if queue.Len() != 0 || queue.items != nil || queue.head != 0 {
		t.Fatal("Clear retained queue state")
	}
}
