package program

import "testing"

func TestTaskQueueYieldsWithinAnAcceptedBurst(t *testing.T) {
	queue := newTaskQueue()
	defer queue.stop()

	want := make([]int, maxTasksPerTurn+3)
	var got []int
	for i := range want {
		want[i] = i
		if err := queue.post(func() { got = append(got, i) }); err != nil {
			t.Error(err)
		}
	}
	queue.requestFrame()

	// Stand in for the program select that observed the coalesced initial wake.
	<-queue.wake
	first, refreshed := queue.take()
	if refreshed {
		t.Fatal("a full turn granted the frame request before the work behind it")
	}
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
	last, refreshed := queue.take()
	if len(last) != 3 || !refreshed {
		t.Fatalf("last turn = %d tasks with refresh %v, want three tasks then refresh", len(last), refreshed)
	}
	for _, task := range last {
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
