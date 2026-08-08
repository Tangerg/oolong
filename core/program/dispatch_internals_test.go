package program

import "testing"

func TestTaskQueueYieldsWithinAnAcceptedBurst(t *testing.T) {
	queue := newTaskQueue()
	defer queue.stop()

	want := make([]int, maxTasksPerTurn+3)
	var got []int
	for i := range want {
		want[i] = i
		queue.post(func() { got = append(got, i) })
	}
	queue.post(nil)

	// Stand in for the program select that observed the coalesced initial wake.
	<-queue.wake
	first := queue.take()
	if len(first) != maxTasksPerTurn {
		t.Fatalf("first turn contains %d tasks, want %d", len(first), maxTasksPerTurn)
	}
	for _, task := range first {
		task()
	}

	select {
	case <-queue.wake:
	default:
		t.Fatal("work left after one turn did not wake the owner again")
	}
	last := queue.take()
	endsInRefresh := len(last) > 0 && last[len(last)-1] == nil
	if len(last) != 4 || !endsInRefresh {
		t.Fatalf("last turn = %d entries ending in refresh %v, want three tasks then refresh", len(last), endsInRefresh)
	}
	for _, task := range last[:len(last)-1] {
		task()
	}
	if len(got) != len(want) {
		t.Fatalf("ran %d tasks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("task %d ran as %d", i, got[i])
		}
	}
}
