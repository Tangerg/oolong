package term

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestProgressHasOneCanonicalWireValue(t *testing.T) {
	tests := []struct {
		name string
		in   Progress
		want Progress
		seq  string
	}{
		{"clear", Progress{Percent: 75}, Progress{}, "\x1b]9;4;0\x07"},
		{"normal low", Progress{State: ProgressNormal, Percent: -1}, Progress{State: ProgressNormal}, "\x1b]9;4;1;0\x07"},
		{"normal high", Progress{State: ProgressNormal, Percent: 101}, Progress{State: ProgressNormal, Percent: 100}, "\x1b]9;4;1;100\x07"},
		{"error", Progress{State: ProgressError, Percent: 37}, Progress{State: ProgressError, Percent: 37}, "\x1b]9;4;2;37\x07"},
		{"indeterminate", Progress{State: ProgressIndeterminate, Percent: 37}, Progress{State: ProgressIndeterminate}, "\x1b]9;4;3\x07"},
		{"warning", Progress{State: ProgressWarning, Percent: 80}, Progress{State: ProgressWarning, Percent: 80}, "\x1b]9;4;4;80\x07"},
		{"unknown", Progress{State: ProgressState(255), Percent: 80}, Progress{}, "\x1b]9;4;0\x07"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.in.normalized(); got != test.want {
				t.Fatalf("normalized = %+v, want %+v", got, test.want)
			}
			if got := test.in.sequence(); got != test.seq {
				t.Fatalf("sequence = %q, want %q", got, test.seq)
			}
		})
	}
}

func TestProgressChangesWaitWhileAnotherProcessOwnsTheTerminal(t *testing.T) {
	p := newTaskProgress()
	var written []string
	queue := func(data []byte) uint64 {
		written = append(written, string(data))
		return uint64(len(written))
	}

	p.to(Progress{State: ProgressNormal, Percent: 20}, queue)
	p.to(Progress{State: ProgressNormal, Percent: 20}, queue)
	if len(written) != 1 {
		t.Fatalf("unchanged progress wrote %q", written)
	}
	p.pause()
	p.to(Progress{State: ProgressError, Percent: 100}, queue)
	if len(written) != 1 {
		t.Fatalf("wrote while paused: %q", written)
	}
	if got := p.leave(); got != progressClear {
		t.Fatalf("leave = %q, want progress cleared", got)
	}
	if got := p.enter(); got != "\x1b]9;4;2;100\x07" {
		t.Fatalf("enter = %q, want the latest state", got)
	}
	p.resume()

	p.pause()
	p.to(Progress{}, queue)
	p.restore(queue)
	if !slices.Equal(written, []string{"\x1b]9;4;1;20\x07", progressClear}) {
		t.Fatalf("restored writes = %q", written)
	}
}

func TestActiveProgressKeepsItselfAliveAndIdleProgressDoesNoWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newTaskProgress()
		stop := make(chan struct{})
		var mu sync.Mutex
		var written []string
		queue := func(data []byte) uint64 {
			mu.Lock()
			defer mu.Unlock()
			written = append(written, string(data))
			return uint64(len(written))
		}
		count := func() int {
			mu.Lock()
			defer mu.Unlock()
			return len(written)
		}
		go p.run(stop, queue)

		p.to(Progress{State: ProgressIndeterminate}, queue)
		synctest.Wait()
		if got := count(); got != 1 {
			t.Fatalf("opening writes = %d, want 1", got)
		}
		time.Sleep(progressKeepalive)
		synctest.Wait()
		if got := count(); got != 2 {
			t.Fatalf("writes after keepalive = %d, want 2", got)
		}

		p.to(Progress{}, queue)
		synctest.Wait()
		if got := count(); got != 3 {
			t.Fatalf("writes after clear = %d, want 3", got)
		}
		time.Sleep(2 * progressKeepalive)
		synctest.Wait()
		if got := count(); got != 3 {
			t.Fatalf("idle progress wrote %d times, want 3", got)
		}

		close(stop)
		<-p.done
	})
}
