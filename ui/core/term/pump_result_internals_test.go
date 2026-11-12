package term

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestPumpRetainsReadFailureWhenDeliveryEnds(t *testing.T) {
	readFailure := errors.New("input transport failed")
	deliveryFailure := errors.New("protocol handling failed")
	wrappedEOF := fmt.Errorf("transport: %w", io.EOF)
	joinedEOF := errors.Join(io.EOF, readFailure)
	for _, test := range []struct {
		name   string
		result error
		want   error
	}{
		{name: "live"},
		{name: "EOF", result: io.EOF},
		{name: "failure", result: readFailure, want: readFailure},
		{name: "wrapped EOF", result: wrappedEOF, want: wrappedEOF},
		{name: "joined EOF", result: joinedEOF, want: joinedEOF},
	} {
		for _, startup := range []bool{false, true} {
			for _, stopped := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/startup=%t/stopped=%t", test.name, startup, stopped), func(t *testing.T) {
					stop := make(chan struct{})
					p := &pump{stop: stop, out: make(chan input.Event), stream: &input.Stream{}}
					var wantDelivery error
					if stopped {
						close(stop)
					} else {
						wantDelivery = deliveryFailure
						p.consume = func(input.Event) (bool, error) { return false, deliveryFailure }
					}
					result := readResult{data: []byte("x"), at: time.Unix(1, 0), err: test.result}
					var err error
					if startup {
						p.early = []input.Event{input.Key{Code: input.Character, Rune: 'x'}}
						if test.result != nil {
							p.end = &readResult{at: result.at, err: test.result}
						}
						err = p.run()
					} else {
						var ended bool
						ended, err = p.take(result)
						if !ended {
							t.Fatal("refused delivery kept the pump running")
						}
					}
					for _, want := range []error{test.want, wantDelivery} {
						if want != nil && !errors.Is(err, want) {
							t.Errorf("pump result = %v, lost %v", err, want)
						}
					}
					if test.want == nil && wantDelivery == nil && err != nil {
						t.Errorf("clean stop = %v", err)
					}
				})
			}
		}
	}
}
