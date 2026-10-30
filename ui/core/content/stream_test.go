package content_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func plainRenderer(_ context.Context, source string) (grid.Drawable, error) {
	return text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of(source, grid.Style{})}}), nil
}

func newStream(t *testing.T, limit int) *content.StreamDocument {
	t.Helper()
	d, err := content.NewStreamDocument(content.StreamConfig{Format: "text", MaxSourceBytes: limit})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDocumentRejectsStaleAndForeignPreparations(t *testing.T) {
	d := newStream(t, 0)
	if err := d.Append([]byte("one")); err != nil {
		t.Fatal(err)
	}
	old := d.Snapshot().Prepare(context.Background(), plainRenderer)
	if err := d.Append([]byte(" two")); err != nil {
		t.Fatal(err)
	}
	if err := d.Accept(old); !errors.Is(err, content.ErrStalePreparation) {
		t.Fatal(err)
	}
	other := newStream(t, 0)
	if err := other.Append([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := other.Append([]byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := d.Accept(other.Snapshot().Prepare(context.Background(), plainRenderer)); !errors.Is(err, content.ErrStalePreparation) {
		t.Fatal(err)
	}
	current := d.Snapshot().Prepare(context.Background(), plainRenderer)
	if err := d.Accept(current); err != nil {
		t.Fatal(err)
	}
	if err := d.Accept(current); !errors.Is(err, content.ErrAlreadyPrepared) {
		t.Fatal(err)
	}
}

func TestFinalizationRequiresFinalSourceRevisionAndKeepsPartialCause(t *testing.T) {
	d := newStream(t, 0)
	if err := d.Append([]byte("useful")); err != nil {
		t.Fatal(err)
	}
	preview := d.Snapshot().Prepare(context.Background(), plainRenderer)
	if err := d.Accept(preview); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("source cancelled")
	if err := d.End(cause); err != nil {
		t.Fatal(err)
	}
	if d.Current().State != content.Finalizing || d.Current().Publishable() {
		t.Fatal("EOF reused provisional parse")
	}
	if err := d.Accept(preview); !errors.Is(err, content.ErrStalePreparation) {
		t.Fatal(err)
	}
	if err := d.Accept(d.Snapshot().Prepare(context.Background(), plainRenderer)); err != nil {
		t.Fatal(err)
	}
	if d.Current().State != content.Partial || !errors.Is(d.Current().SourceError, cause) {
		t.Fatalf("partial=%+v", d.Current())
	}
	if err := d.Append([]byte("late")); !errors.Is(err, content.ErrSourceClosed) {
		t.Fatal(err)
	}
}

func TestRefusalAndSnapshotsPreserveAcceptedSource(t *testing.T) {
	d := newStream(t, 4)
	bytes := []byte("abc")
	if err := d.Append(bytes); err != nil {
		t.Fatal(err)
	}
	snapshot := d.Snapshot()
	bytes[0] = 'x'
	if err := d.Append([]byte("de")); !errors.Is(err, content.ErrSourceLimit) {
		t.Fatal(err)
	}
	if d.Snapshot().Text() != "abc" || snapshot.Text() != "abc" || d.Snapshot().Revision() != snapshot.Revision() {
		t.Fatal("input ownership or rejected append changed source")
	}
	if err := d.Append([]byte("d")); err != nil {
		t.Fatal(err)
	}
	if snapshot.Text() != "abc" {
		t.Fatal("old worker snapshot changed")
	}
}

func TestDocumentSubscriptionObservesSettledFactAndRevokes(t *testing.T) {
	d := newStream(t, 0)
	calls := 0
	var stop func()
	stop = d.Subscribe(func(change content.DocumentChange) {
		calls++
		if change.Revision != d.Current().Revision {
			t.Fatal("notification preceded settlement")
		}
		stop()
	})
	if err := d.Append([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := d.Append([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestDocumentNotificationsCannotCreateReentrantSourceOwner(t *testing.T) {
	d := newStream(t, 0)
	d.Subscribe(func(content.DocumentChange) {
		defer func() {
			if recover() == nil {
				t.Fatal("reentrant source mutation was allowed")
			}
		}()
		_ = d.Append([]byte("replacement"))
	})
	if err := d.Append([]byte("original")); err != nil {
		t.Fatal(err)
	}
	if d.Snapshot().Text() != "original" {
		t.Fatal("reentrant mutation changed source")
	}
}

func TestRegistryPreparationKeepsDispatchAndDocumentIdentityTogether(t *testing.T) {
	renderFailure := errors.New("render stopped after readable output")
	calls := 0
	registry, err := content.New(content.Config{Bindings: []content.Binding{{Format: "text", Render: func(ctx context.Context, source string) (grid.Drawable, error) {
		calls++
		body, renderErr := plainRenderer(ctx, source)
		return body, errors.Join(renderErr, renderFailure)
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	document := newStream(t, 0)
	if err := document.Append([]byte("readable")); err != nil {
		t.Fatal(err)
	}
	preview := registry.Prepare(t.Context(), document.Snapshot())
	if err := document.End(nil); err != nil {
		t.Fatal(err)
	}
	if err := document.Accept(preview); !errors.Is(err, content.ErrStalePreparation) {
		t.Fatalf("registry result bypassed final revision: %v", err)
	}
	if err := document.Accept(registry.Prepare(t.Context(), document.Snapshot())); err != nil {
		t.Fatal(err)
	}
	change := document.Current()
	if calls != 2 || change.State != content.Partial || change.Body == nil || !errors.Is(change.PrepareError, renderFailure) {
		t.Fatalf("partial dispatch was not preserved: calls=%d change=%+v", calls, change)
	}
	missing := newStream(t, 0)
	if err := missing.End(nil); err != nil {
		t.Fatal(err)
	}
	var empty content.Registry
	if err := missing.Accept(empty.Prepare(t.Context(), missing.Snapshot())); err != nil {
		t.Fatal(err)
	}
	if missing.Current().State != content.Failed || !errors.Is(missing.Current().PrepareError, content.ErrUnknownFormat) {
		t.Fatal("unknown format did not settle as an explicit preparation failure")
	}
}
