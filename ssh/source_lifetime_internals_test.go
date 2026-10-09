package ssh

import (
	"errors"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/core/input"
)

type exitingReader struct{ prefix string }

func (r *exitingReader) Read(data []byte) (int, error) {
	if r.prefix != "" {
		n := copy(data, r.prefix)
		r.prefix = r.prefix[n:]
		return n, nil
	}
	runtime.Goexit()
	return 0, nil
}

func TestSourceReportsAReaderExitAfterDeliveringAcceptedBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newEventSource(t.Context().Done(), &exitingReader{prefix: "a"}, nil, nil, nil)
		defer source.Close()
		select {
		case event := <-source.Events():
			if key, ok := event.(input.Key); !ok || key.Rune != 'a' {
				t.Fatalf("accepted input = %#v, want a key", event)
			}
		case <-time.After(time.Second):
			t.Fatal("reader exit discarded earlier accepted input")
		}
		select {
		case event, ok := <-source.Events():
			if ok || !errors.Is(source.Err(), errReadInterrupted) {
				t.Fatalf("reader exit = event %#v, open %t, error %v", event, ok, source.Err())
			}
		case <-time.After(time.Second):
			t.Fatal("input remained open after its reader exited")
		}
	})
}
