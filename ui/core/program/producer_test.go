package program_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type receiptWriter struct {
	protocolWriter
	written atomic.Uint64
}

func (w *receiptWriter) Written() uint64 { return w.written.Load() }

func TestProducerCannotSettleItsOwnReceipts(t *testing.T) {
	err := runFixture(t.Context(), program.InlineConfig{Host: newHost(t), Build: func(r *program.InlineRuntime) (scene.Element, error) {
		if _, ok := any(r.Publisher()).(interface{ Written(sequence uint64) }); ok {
			t.Error("producer can acknowledge output without the writer")
		}
		r.Quit()
		return scene.NewElement(scene.ElementConfig{}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicationRefusalUsesTheCurrentWriterAndSession(t *testing.T) {
	for _, retained := range []bool{true, false} {
		name := "fresh producer"
		if retained {
			name = "retained producer"
		}
		t.Run(name, func(t *testing.T) {
			cause := errors.New("destination failed after the first delivery")
			writer := &receiptWriter{changes: make(chan struct{})}
			host := &protocolHost{events: make(chan input.Event), writer: writer}
			var after func() (publication.Receipt, error)
			calls := 0
			forbidden := measurer{measure: func(int) int { calls++; return 1 }, draw: func(grid.View) { calls++ }}
			err := runFixture(t.Context(), program.InlineConfig{Host: host, Build: func(r *program.InlineRuntime) (scene.Element, error) {
				producer := r.Publisher()
				first, err := producer.TryPublish(measurer{measure: func(int) int { return 1 }, draw: func(v grid.View) { v.Text(0, 0, "accepted", grid.Style{}) }})
				if err != nil {
					return scene.Element{}, err
				}
				notifications := 0
				deferStop := producer.Subscribe(func() { notifications++ })
				after = func() (publication.Receipt, error) {
					deferStop()
					if notifications == 0 {
						t.Error("producer missed settled progress")
					}
					return producer.TryPublish(forbidden)
				}
				err = r.Dispatcher().Post(func() {
					if first.Snapshot().Sequence != 1 {
						t.Fatal("first frame did not reach writer admission")
					}
					writer.written.Store(1)
					writer.err = cause
					if !retained {
						producer = r.Publisher()
					}
					_, refusal := producer.TryPublish(forbidden)
					if !errors.Is(refusal, cause) || calls != 0 {
						t.Errorf("late admission=%v content callbacks=%d", refusal, calls)
					}
					if s := first.Snapshot(); s.State != publication.Written || s.Sequence != 1 {
						t.Errorf("confirmed write lost at failure: %+v", s)
					}
					r.Quit()
				})
				return scene.NewElement(scene.ElementConfig{}), err
			}})
			if !errors.Is(err, cause) {
				t.Fatalf("run failure=%v", err)
			}
			if _, err := after(); !errors.Is(err, program.ErrStopped) {
				t.Errorf("retained producer survived Run: %v", err)
			}
		})
	}
}

func TestInlinePublicationLimitsUseHostGeometry(t *testing.T) {
	host := newHost(t)
	width, _, sizeErr := host.Size()
	if sizeErr != nil {
		t.Fatal(sizeErr)
	}
	limits := publication.Limits{MaxCells: width - 1}
	if err := limits.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := program.NewInline(program.InlineConfig{Host: host, Publication: publication.Limits{MaxItems: -1}, Build: func(*program.InlineRuntime) (scene.Element, error) {
		t.Fatal("invalid budget called Build")
		return scene.Element{}, nil
	}}); err == nil {
		t.Fatal("invalid budget accepted")
	}
	err := runFixture(t.Context(), program.InlineConfig{Host: host, Publication: limits, Build: func(rt *program.InlineRuntime) (scene.Element, error) {
		_, err := rt.Publisher().TryPublish(measurer{measure: func(got int) int {
			if got != width {
				t.Errorf("capture width=%d, host width=%d", got, width)
			}
			return 1
		}, draw: func(grid.View) { t.Error("over-budget content drawn") }})
		if !errors.Is(err, publication.ErrTooLarge) {
			t.Errorf("budget refusal=%v", err)
		}
		rt.Quit()
		return scene.NewElement(scene.ElementConfig{}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
}
