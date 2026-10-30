package publication_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

type block struct {
	text string
	view *grid.View
	rows int
}

func (b *block) HeightForWidth(int) int {
	if b.rows != 0 {
		return b.rows
	}
	return 1
}

func (b *block) Draw(v grid.View) {
	v.Text(0, 0, b.text, grid.Style{})
	if b.view != nil {
		*b.view = v
	}
}

func newPublisher(t *testing.T, cfg publication.Config) *publication.Publisher {
	t.Helper()
	if cfg.Width == 0 {
		cfg.Width = 12
	}
	p, err := publication.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAdmissionFreezesContentAndWaitsForActualWrite(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	b := &block{text: "original"}
	r, err := p.TryPublish(b)
	if err != nil {
		t.Fatal(err)
	}
	b.text = "changed"
	batch := p.Prepare()
	rows, err := batch.Items()[0].Content.Rows(12)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][0].Content() != "o" {
		t.Fatal("accepted content was mutable")
	}
	rows[0][0].Link = "caller mutation"
	rows, err = batch.Items()[0].Content.Rows(12)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][0].Link != "" {
		t.Fatal("snapshot leaked writable storage")
	}
	p.Commit(batch, 9)
	if got := r.Snapshot(); got.State != publication.Accepted || got.Sequence != 9 || got.ID == publication.ID(got.Sequence) {
		t.Fatalf("admitted result=%+v", got)
	}
	p.Written(8)
	if r.Snapshot().State != publication.Accepted {
		t.Fatal("queued output counted as written")
	}
	p.Written(9)
	if r.Snapshot().State != publication.Written {
		t.Fatal("writer progress did not settle receipt")
	}
	if items, bytes := p.Pending(); items != 0 || bytes != 0 {
		t.Fatalf("retained %d items %d bytes", items, bytes)
	}
}

func TestCapacityIncludesInflightOutput(t *testing.T) {
	p := newPublisher(t, publication.Config{MaxItems: 1})
	if _, err := p.TryPublish(&block{text: "first"}); err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 1)
	if _, err := p.TryPublish(&block{text: "second"}); !errors.Is(err, publication.ErrCapacity) {
		t.Fatalf("capacity error=%v", err)
	}
	p.Written(1)
	if _, err := p.TryPublish(&block{text: "second"}); err != nil {
		t.Fatal(err)
	}
}

func TestOversizeAndUnsupportedRefuseBeforeIdentityAndOwnership(t *testing.T) {
	p := newPublisher(t, publication.Config{MaxCells: 12})
	if _, err := p.TryPublish(&block{rows: 2}); !errors.Is(err, publication.ErrTooLarge) {
		t.Fatalf("oversize=%v", err)
	}
	if items, _ := p.Pending(); items != 0 {
		t.Fatal("refused item consumed capacity")
	}
	r, err := p.TryPublish(&block{text: "fits"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Snapshot().ID != 1 {
		t.Fatal("refusal advanced accepted identity")
	}
}

func TestFailureDistinguishesPendingFromUnknownPartial(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	first, err := p.TryPublish(&block{text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 7)
	second, err := p.TryPublish(&block{text: "second"})
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("short transport write")
	p.Fail(cause, true)
	if first.Snapshot().State != publication.UnknownPartial || second.Snapshot().State != publication.Failed {
		t.Fatalf("failure states: %+v %+v", first.Snapshot(), second.Snapshot())
	}
	if !errors.Is(first.Snapshot().Err, cause) {
		t.Fatal("lost transport cause")
	}
	if _, err := p.TryPublish(&block{}); !errors.Is(err, publication.ErrClosed) {
		t.Fatal("closed publisher accepted retry")
	}
}

func TestNotificationsRunOnlyAfterAllOwnersCommit(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	calls := 0
	cancel := p.Subscribe(func() {
		calls++
		if calls == 1 {
			if _, err := p.TryPublish(&block{text: "next"}); err != nil {
				t.Fatal(err)
			}
			if p.FlushNotifications() {
				t.Fatal("recursive notification")
			}
		}
	})
	if _, err := p.TryPublish(&block{text: "first"}); err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 1)
	p.Written(1)
	if calls != 0 {
		t.Fatal("compositor settlement called application code")
	}
	if !p.FlushNotifications() || calls != 1 {
		t.Fatalf("notification calls=%d", calls)
	}
	if !p.FlushNotifications() || calls != 2 {
		t.Fatal("callback transition did not wait for a later turn")
	}
	cancel()
	p.Close(nil)
	p.FlushNotifications()
	if calls != 2 {
		t.Fatal("cancelled listener ran")
	}
}

func TestReceiptCanBeObservedWhileWriterProgressSettles(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	r, err := p.TryPublish(&block{text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			_ = r.Snapshot()
		}
	})
	p.Written(1)
	wg.Wait()
	if r.Snapshot().State != publication.Written {
		t.Fatal("receipt did not settle")
	}
}

func TestCloseAdmissionPreservesAcceptedWorkUntilFinalDrain(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	first, err := p.TryPublish(&block{text: "inflight"})
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 3)
	second, err := p.TryPublish(&block{text: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("session closing")
	notifications := 0
	p.Subscribe(func() { notifications++ })
	p.CloseAdmission(cause)
	if _, err := p.TryPublish(&panicBody{}); !errors.Is(err, publication.ErrClosed) || !errors.Is(err, cause) {
		t.Fatalf("admission refusal=%v", err)
	}
	if first.Snapshot().State != publication.Accepted || second.Snapshot().State != publication.Accepted || p.Prepare().Len() != 1 {
		t.Fatal("closing admission discarded accepted work")
	}
	if err := p.SetGeometry(8, grid.Ground{}); err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 4)
	p.Written(3)
	if first.Snapshot().State != publication.Written || second.Snapshot().State != publication.Accepted {
		t.Fatal("first watermark settled the wrong responsibility")
	}
	p.Written(4)
	p.Close(nil)
	if second.Snapshot().State != publication.Written {
		t.Fatal("terminal close replaced a successful result")
	}
	if notifications != 0 {
		t.Fatal("closing state called user code during settlement")
	}
	p.FlushNotifications()
	if notifications != 1 {
		t.Fatalf("final notification count=%d", notifications)
	}
	p.Written(100)
	p.FlushNotifications()
	if notifications != 1 {
		t.Fatal("terminally closed publisher retained subscriber")
	}
}

type panicBody struct{}

func (*panicBody) HeightForWidth(int) int { panic("closed owner called producer") }
func (*panicBody) Draw(grid.View)         { panic("closed owner drew producer") }

type emptyBody struct{}

func (emptyBody) HeightForWidth(int) int { return 0 }
func (emptyBody) Draw(grid.View)         { panic("empty content drawn") }

func TestEmptyContentHasAWrittenNoOpReceiptEvenAtCapacity(t *testing.T) {
	p := newPublisher(t, publication.Config{MaxItems: 1})
	accepted, err := p.TryPublish(&block{text: "full"})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []grid.Drawable{nil, emptyBody{}} {
		receipt, err := p.TryPublish(body)
		if err != nil {
			t.Fatal(err)
		}
		if got := receipt.Snapshot(); got.State != publication.Written || got.Sequence != 0 || got.ID <= accepted.Snapshot().ID {
			t.Fatalf("empty receipt=%+v", got)
		}
	}
	if items, _ := p.Pending(); items != 1 {
		t.Fatal("empty content consumed admission capacity")
	}
}

func TestInvalidBatchCannotAdvanceReceiptsOrCapacity(t *testing.T) {
	for _, name := range []string{"zero sequence", "foreign batch", "stale batch", "nonmonotonic sequence"} {
		t.Run(name, func(t *testing.T) {
			p := newPublisher(t, publication.Config{})
			receipt, err := p.TryPublish(&block{text: "first"})
			if err != nil {
				t.Fatal(err)
			}
			batch := p.Prepare()
			sequence := uint64(1)
			owner := p
			switch name {
			case "zero sequence":
				sequence = 0
			case "foreign batch":
				owner = newPublisher(t, publication.Config{})
			case "stale batch":
				p.Commit(batch, 1)
				p.Written(1)
				sequence = 2
			case "nonmonotonic sequence":
				p.Commit(batch, 5)
				p.Written(5)
				receipt, err = p.TryPublish(&block{text: "second"})
				if err != nil {
					t.Fatal(err)
				}
				batch = p.Prepare()
				sequence = 5
			}
			before := receipt.Snapshot()
			count, size := p.Pending()
			func() {
				defer func() {
					if recover() == nil {
						t.Error("invalid batch accepted")
					}
				}()
				owner.Commit(batch, sequence)
			}()
			after := receipt.Snapshot()
			nextCount, nextSize := p.Pending()
			if before != after || count != nextCount || size != nextSize {
				t.Fatal("invalid batch changed accepted state")
			}
		})
	}
}

func TestKnownFailureRefusesBeforeContentCallbacksAndPreservesCause(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	cause := errors.New("writer failed")
	p.Fail(cause, true)
	if _, err := p.TryPublish(&panicBody{}); !errors.Is(err, cause) || !errors.Is(err, publication.ErrClosed) {
		t.Fatalf("refusal lost cause=%v", err)
	}
	if err := p.SetGeometry(4, grid.Ground{}); !errors.Is(err, cause) || !errors.Is(err, publication.ErrClosed) {
		t.Fatalf("geometry refusal lost cause=%v", err)
	}
}

func TestCaptureReentryCannotAdvancePublisher(t *testing.T) {
	p := newPublisher(t, publication.Config{})
	body := callbackBody{draw: func(v grid.View) {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("capture reentered its owner")
				}
			}()
			p.CloseAdmission(nil)
		}()
		v.Text(0, 0, "kept", grid.Style{})
	}}
	receipt, err := p.TryPublish(body)
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(p.Prepare(), 1)
	p.Written(1)
	if receipt.Snapshot().State != publication.Written {
		t.Fatal("refused reentry damaged outer admission")
	}
}

type callbackBody struct{ draw func(grid.View) }

func (callbackBody) HeightForWidth(int) int { return 1 }
func (b callbackBody) Draw(v grid.View)     { b.draw(v) }
