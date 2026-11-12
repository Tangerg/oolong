package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
)

type terminatingReader struct{ result error }

func (r terminatingReader) Read(data []byte) (int, error) {
	return copy(data, "ab"), r.result
}

func TestSourceKeepsReadFailureWhenDeliveryStopsMidBatch(t *testing.T) {
	cause := errors.New("channel read failed")
	wrappedEOF := fmt.Errorf("channel: %w", io.EOF)
	joinedEOF := errors.Join(io.EOF, cause)
	for _, test := range []struct {
		name   string
		result error
		want   error
	}{
		{name: "live"},
		{name: "EOF", result: io.EOF},
		{name: "failure", result: cause, want: cause},
		{name: "wrapped EOF", result: wrappedEOF, want: wrappedEOF},
		{name: "joined EOF", result: joinedEOF, want: joinedEOF},
	} {
		for _, cancelled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cancelled=%t", test.name, cancelled), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				source := newEventSource(ctx.Done(), terminatingReader{result: test.result}, nil, nil)
				defer source.Close()
				event := <-source.Events()
				if key, ok := event.(input.Key); !ok || !key.IsRune('a', 0) {
					t.Fatalf("accepted prefix = %#v", event)
				}
				if cancelled {
					cancel()
				}
				source.Close()
				if _, open := <-source.Events(); open {
					t.Fatal("stopped delivery emitted the remaining batch")
				}
				if err := source.Err(); !errors.Is(err, test.want) {
					t.Fatalf("stopped source = %v, want %v", err, test.want)
				}
			})
		}
	}
}
