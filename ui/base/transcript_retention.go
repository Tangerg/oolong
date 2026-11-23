package base

import (
	"errors"
	"math"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/internal/identity"
)

// DiscardBefore releases every retained item older than id without publishing it and
// reports how many it released, which is how an owner bounds what it keeps when it
// alone knows which content has settled. The reader keeps their place: rows leaving
// the start leave the scroll offset with them, and a selection touching released
// content ends. Released content of a publishing transcript is never published.
// It panics if releasing could exhaust content revisions.
func (t *Transcript) DiscardBefore(id BlockID) int {
	t.assertMutable()
	count := 0
	for count < t.entries.Len() && t.entries.At(count).id < id {
		count++
	}
	t.requireRevisions(count)
	for range count {
		t.retireHead(t.headRows())
	}
	// Releasing unfinished content can leave completed content at the front.
	t.publish()
	return count
}

func (t *Transcript) publicationCount(extra *TranscriptItem) int {
	if t.closed || t.policy.retention != PublishCompleted || t.policy.publisher == nil {
		return 0
	}
	completed := 0
	for item := range t.entries.Values() {
		if !t.publishable(item) {
			break
		}
		completed++
	}
	if extra != nil && completed == t.entries.Len() && t.publishable(extra) {
		completed++
	}
	return max(completed-t.policy.keepCompleted, 0)
}

func (t *Transcript) publishable(item *TranscriptItem) bool {
	change := item.content()
	return change.State == content.Final || t.policy.publishPartial && (change.State == content.Partial || change.State == content.Failed)
}

func (t *Transcript) publish() {
	count := t.publicationCount(nil)
	if count == 0 {
		return
	}
	t.assertMutable()
	t.requireRevisions(count)
	for range count {
		item := t.entries.At(0)
		rows := t.headRows()
		var receipt publication.Receipt
		var err error
		t.element.ReadOnly(func() { receipt, err = t.policy.publisher.TryPublish(item.content().Body) })
		if err != nil {
			if !errors.Is(err, publication.ErrCapacity) && !identity.Same(item.err, err) {
				item.err = err
				t.element.Invalidate()
			}
			return
		}
		item.receipt, item.err = receipt, nil
		t.retireHead(rows)
	}
}

// headRows is the committed height of the oldest item, checked against the row
// origin before anything about that item changes.
func (t *Transcript) headRows() int {
	_, rows, _ := t.geometry.Value().Extent(t.entries.At(0).id)
	if rows > math.MaxInt-t.startRow {
		panic("base: transcript row origin exhausted")
	}
	return rows
}

// retireHead releases the oldest item. Every owner of a position moves with it in
// one step, so scroll, selection and sticky furniture never describe content the
// transcript no longer holds.
func (t *Transcript) retireHead(rows int) {
	item := t.entries.At(0)
	t.startRow += rows
	t.scroll.discard(rows)
	t.selection.discardBefore(item.id + 1)
	item.retire()
	t.entries.Discard(1)
	t.sticky.discardBefore(t.FirstBlock())
	t.changed()
}
