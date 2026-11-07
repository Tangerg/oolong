package docprep_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
)

type widthBody struct {
	width   atomic.Int64
	calls   atomic.Int32
	started chan int
	gate    <-chan struct{}
	err     error
}

func (body *widthBody) PrepareWidth(ctx context.Context, width int) error {
	body.calls.Add(1)
	if body.started != nil {
		body.started <- width
	}
	if body.gate != nil {
		<-body.gate
	}
	if body.err != nil {
		return body.err
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	body.width.Store(int64(width))
	return nil
}

func (body *widthBody) PreparedLayout(width int) (grid.Drawable, bool) {
	return frozenWidth(width), body.width.Load() == int64(width)
}
func (*widthBody) HeightForWidth(int) int { panic("mutable body reached publication") }
func (*widthBody) Draw(grid.View)         { panic("mutable body reached publication") }

type frozenWidth int

func (width frozenWidth) HeightForWidth(int) int { return int(width) }
func (frozenWidth) Draw(grid.View)               {}

type capturePublisher struct {
	captured  []grid.Drawable
	listeners int
}

func (p *capturePublisher) TryPublish(body grid.Drawable) (publication.Receipt, error) {
	p.captured = append(p.captured, body)
	return publication.Receipt{}, nil
}

func (p *capturePublisher) Subscribe(func()) func() {
	p.listeners++
	return func() { p.listeners-- }
}

func TestWarmCompletionRetriesColdPublicationAfterReleasingTheWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		body := &widthBody{gate: gate, started: make(chan int, 1)}
		source := &capturePublisher{}
		var transcript *base.Transcript
		fixture.apply(t, func() error {
			publisher := fixture.queue.Publisher(source)
			var err error
			transcript, err = base.NewTranscript(base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
			if err != nil {
				return err
			}
			if _, err := transcript.Add(body); err != nil {
				return err
			}
			if frameReady(t, fixture.queue, 40, transcript) || len(source.captured) != 0 {
				t.Error("cold body was captured")
			}
			if _, err := publisher.TryPublish(body); !errors.Is(err, publication.ErrCapacity) {
				t.Errorf("active layout capture=%v", err)
			}
			return nil
		})
		if width := <-body.started; width != 40 {
			t.Fatal(width)
		}
		release()
		synctest.Wait()
		fixture.apply(t, func() error {
			if len(source.captured) != 1 || transcript.Len() != 0 || source.captured[0].HeightForWidth(1) != 40 {
				t.Errorf("captures=%v retained=%d", source.captured, transcript.Len())
			}
			fixture.queue.Close()
			transcript.Close()
			if source.listeners != 0 {
				t.Errorf("closed queue retained %d publication listeners", source.listeners)
			}
			return nil
		})
	})
}

func TestResizePreparesLatestWidthAndIdleFramesStartNoWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		body := &widthBody{}
		transcript, err := base.NewTranscript(base.TranscriptConfig{})
		if err != nil {
			t.Fatal(err)
		}
		fixture.apply(t, func() error {
			if _, err := transcript.Add(body); err != nil {
				return err
			}
			frameReady(t, fixture.queue, 40, transcript)
			return nil
		})
		synctest.Wait()
		fixture.host.events <- input.Resize{Width: 20, Height: 8}
		synctest.Wait()
		fixture.apply(t, func() error {
			if frameReady(t, fixture.queue, 20, transcript) {
				t.Error("old width was treated as ready")
			}
			return nil
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			for range 100 {
				if !frameReady(t, fixture.queue, 20, transcript) {
					t.Error("prepared frame became cold")
				}
			}
			fixture.scope.Close()
			if !frameReady(t, fixture.queue, 20, transcript) {
				t.Error("scope cancellation denied an already prepared shutdown frame")
			}
			return nil
		})
		time.Sleep(time.Second)
		if body.calls.Load() != 2 {
			t.Errorf("layout calls=%d, want exactly initial and resize", body.calls.Load())
		}
	})
}

func TestLayoutFailureDoesNotPollAfterCallerCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		cause := errors.New("cannot prepare width")
		body := &widthBody{err: cause}
		transcript, err := base.NewTranscript(base.TranscriptConfig{})
		if err != nil {
			t.Fatal(err)
		}
		fixture.apply(t, func() error {
			if _, err := transcript.Add(body); err != nil {
				return err
			}
			frameReady(t, fixture.queue, 40, transcript)
			return nil
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			fixture.queue.Clear()
			for range 20 {
				ready, err := fixture.queue.Ready(40, transcript)
				if ready || !errors.Is(err, cause) {
					t.Errorf("failed layout readiness=%v, %v", ready, err)
				}
			}
			return nil
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			if body.calls.Load() != 1 || len(fixture.failures) != 0 {
				t.Errorf("calls=%d failures=%v", body.calls.Load(), fixture.failures)
			}
			return nil
		})
	})
}

func TestNewPreparedBodyAtTheSameIdentityCanRecoverFromLayoutFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		cause := errors.New("old layout failed")
		old, replacement := &widthBody{err: cause}, &widthBody{}
		document := newDocument(t, "old", false)
		transcript, err := base.NewTranscript(base.TranscriptConfig{})
		if err != nil {
			t.Fatal(err)
		}
		var identity base.BlockID
		var revision uint64
		fixture.apply(t, func() error {
			if err := document.Accept(document.Snapshot().Prepare(context.Background(), func(context.Context, string) (grid.Drawable, error) { return old, nil })); err != nil {
				return err
			}
			item, err := transcript.AddDocument(document)
			if err != nil {
				return err
			}
			identity, revision = item.ID(), transcript.ContentRevision()
			frameReady(t, fixture.queue, 40, transcript)
			return nil
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			if ready, err := fixture.queue.Ready(40, transcript); ready || !errors.Is(err, cause) {
				t.Errorf("failed readiness=%v, %v", ready, err)
			}
			if err := document.Append([]byte(" replacement")); err != nil {
				return err
			}
			if err := document.Accept(document.Snapshot().Prepare(context.Background(), func(context.Context, string) (grid.Drawable, error) { return replacement, nil })); err != nil {
				return err
			}
			if transcript.FirstBlock() != identity || transcript.ContentRevision() == revision {
				t.Error("fixture did not replace content at the same retained identity")
			}
			if frameReady(t, fixture.queue, 40, transcript) {
				t.Error("replacement was already prepared")
			}
			return nil
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			if !frameReady(t, fixture.queue, 40, transcript) || old.calls.Load() != 1 || replacement.calls.Load() != 1 {
				t.Error("old failure blocked the replacement")
			}
			return nil
		})
	})
}

func TestLayoutAdmissionFailureReturnsFromReadyWithoutAWorkerWakeup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 1)
		body := &widthBody{}
		transcript, err := base.NewTranscript(base.TranscriptConfig{})
		if err != nil {
			t.Fatal(err)
		}
		fixture.apply(t, func() error {
			if _, err := transcript.Add(body); err != nil {
				return err
			}
			ready, err := fixture.queue.Ready(40, transcript)
			if ready || !errors.Is(err, program.ErrCapacity) || body.calls.Load() != 0 || len(fixture.failures) != 0 {
				t.Errorf("ready=%v err=%v runs=%d failures=%v", ready, err, body.calls.Load(), fixture.failures)
			}
			return nil
		})
	})
}

func TestQueuedResizePublishesOnlyTheLatestImmutableWidth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		body := &widthBody{gate: gate, started: make(chan int, 2)}
		source := &capturePublisher{}
		var transcript *base.Transcript
		fixture.apply(t, func() error {
			var err error
			transcript, err = base.NewTranscript(base.TranscriptConfig{Publisher: fixture.queue.Publisher(source), Retention: base.PublishCompleted})
			if err != nil {
				return err
			}
			if _, err := transcript.Add(body); err != nil {
				return err
			}
			frameReady(t, fixture.queue, 40, transcript)
			return nil
		})
		if width := <-body.started; width != 40 {
			t.Fatal(width)
		}
		fixture.host.events <- input.Resize{Width: 20, Height: 8}
		synctest.Wait()
		fixture.apply(t, func() error {
			if frameReady(t, fixture.queue, 20, transcript) || len(source.captured) != 0 {
				t.Error("resize captured a cold or changing cache")
			}
			return nil
		})
		release()
		synctest.Wait()
		if width := <-body.started; width != 20 {
			t.Fatal(width)
		}
		fixture.apply(t, func() error {
			if len(source.captured) != 1 || transcript.Len() != 0 || source.captured[0].HeightForWidth(1) != 20 || body.calls.Load() != 2 {
				t.Errorf("captures=%v retained=%d runs=%d", source.captured, transcript.Len(), body.calls.Load())
			}
			return nil
		})
	})
}

type noncooperativeLayout struct {
	widthBody
	started chan struct{}
}

func (body *noncooperativeLayout) PrepareWidth(_ context.Context, width int) error {
	body.width.Store(int64(width))
	close(body.started)
	<-body.gate
	return nil
}

func TestRevokedDeliveryWaitsForActualLayoutExitBeforeTheShutdownFrame(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		body := &noncooperativeLayout{gate: gate, started: make(chan struct{})}
		transcript, err := base.NewTranscript(base.TranscriptConfig{})
		if err != nil {
			t.Fatal(err)
		}
		fixture.apply(t, func() error {
			if _, err := transcript.Add(body); err != nil {
				return err
			}
			frameReady(t, fixture.queue, 40, transcript)
			return nil
		})
		<-body.started
		fixture.apply(t, func() error {
			fixture.scope.Close()
			return nil
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			if frameReady(t, fixture.queue, 40, transcript) {
				t.Error("revoked delivery was mistaken for worker exit")
			}
			if err := fixture.queue.Prepare(newDocument(t, "cancelled", true), plain, nil); !errors.Is(err, program.ErrScopeClosed) {
				t.Errorf("cancelled scope admitted source: %v", err)
			}
			return nil
		})
		release()
		synctest.Wait()
		fixture.apply(t, func() error {
			if !frameReady(t, fixture.queue, 40, transcript) {
				t.Error("completed, revoked worker stranded an already hot shutdown frame")
			}
			return nil
		})
	})
}
