package base_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/publication"

	"github.com/Tangerg/oolong/ui/base"
)

// A transcript is the one numbering a scroll offset, the ends of a selection, a
// search match and a pinned prompt all speak. They only mean the same thing to each
// other while the numbering answers the same way whichever question is asked of it,
// and the questions are asked by four different owners — so the answers are checked
// against each other rather than against what any one of them expects.

func blockCount(n int) base.BlockID {
	if n < 0 {
		panic("negative block count")
	}
	return base.BlockID(n)
}

// assertOneNumbering holds the transcript's three views of its own rows to each
// other: what each block says its extent is, what each row says it belongs to, and
// how tall the whole thing is.
func assertOneNumbering(t *testing.T, tr *base.Transcript) {
	t.Helper()
	first, total := tr.FirstBlock(), 0
	at := tr.StartRow()
	for i := range tr.Len() {
		id := first + blockCount(i)
		top, height, ok := tr.Extent(id)
		if !ok {
			t.Fatalf("block %d of %d has no extent", i, tr.Len())
		}
		if top != at {
			t.Fatalf("block %d starts at row %d, and the block before it ended at %d", i, top, at)
		}
		for offset := range height {
			row := top + offset
			gotID, gotOffset, ok := tr.At(row)
			if !ok || gotID != id || gotOffset != offset {
				t.Fatalf("row %d belongs to block %d offset %d, but block %d says it is its row %d",
					row, gotID, gotOffset, id, offset)
			}
		}
		at, total = at+height, total+height
	}
	if at != tr.EndRow() {
		t.Fatalf("the blocks end at row %d and the transcript ends at %d", at, tr.EndRow())
	}
	if total != tr.Height() {
		t.Fatalf("the blocks are %d rows and the transcript is %d", total, tr.Height())
	}
	if _, _, ok := tr.At(tr.EndRow()); ok {
		t.Fatalf("row %d is past the end and belongs to a block anyway", tr.EndRow())
	}
	// Every row of every block, so the answer is every block with rows in it. A
	// block of no rows is touched by nothing and is outside it at either end.
	if tr.Height() > 0 {
		wantFirst, wantLast := first, first+blockCount(tr.Len())
		for wantFirst < wantLast {
			if _, height, _ := tr.Extent(wantFirst); height > 0 {
				break
			}
			wantFirst++
		}
		for ; wantLast > wantFirst; wantLast-- {
			if _, height, _ := tr.Extent(wantLast - 1); height > 0 {
				break
			}
		}
		gotFirst, gotLast := tr.Visible(tr.StartRow(), tr.Height())
		if gotFirst != wantFirst || gotLast != wantLast {
			t.Fatalf("every row shows blocks [%d,%d), want [%d,%d)",
				gotFirst, gotLast, wantFirst, wantLast)
		}
	}
}

func TestATranscriptAnswersEveryQuestionFromOneNumbering(t *testing.T) {
	retired, reflowed := 0, 0
	for run := range 60 {
		random := rand.New(rand.NewPCG(uint64(run), 0x7472616e73)) //nolint:gosec // reproducible fixtures
		publisher, err := publication.New(publication.Config{Width: 40, MaxItems: 2})
		if err != nil {
			t.Fatal(err)
		}
		transcript := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
		root := transcriptScene(t, transcript)
		width := 1 + random.IntN(40)
		var documents []*content.StreamDocument
		var items []*base.TranscriptItem
		wire := uint64(0)
		for step := range 30 {
			switch random.IntN(8) {
			case 0, 1, 2:
				document := transcriptDocument(t)
				item, err := transcript.AddDocument(document)
				if err != nil {
					t.Fatal(err)
				}
				if err := document.Append([]byte(strings.Repeat("row\n", random.IntN(4)) + "last")); err != nil {
					t.Fatal(err)
				}
				transcriptPrepare(t, document)
				documents = append(documents, document)
				items = append(items, item)
			case 3:
				if len(documents) > 0 && !documents[len(documents)-1].Snapshot().Final() {
					document := documents[len(documents)-1]
					if err := document.Append([]byte("\nmore")); err != nil {
						t.Fatal(err)
					}
					transcriptPrepare(t, document)
				}
			case 4:
				for index, document := range documents {
					if items[index].State() == base.ContentLive && !document.Snapshot().Final() {
						if err := document.End(nil); err != nil {
							t.Fatal(err)
						}
						transcriptPrepare(t, document)
						break
					}
				}
			case 5:
				if batch := publisher.Prepare(); batch.Len() > 0 {
					wire++
					publisher.Commit(batch, wire)
					publisher.Written(wire)
					publisher.FlushNotifications()
				}
			case 6:
				if next := 1 + random.IntN(40); next != width {
					width = next
					reflowed++
				}
			}
			for len(items) > 0 && items[0].State() == base.ContentRetired {
				items = items[1:]
				documents = documents[1:]
				retired++
			}
			transcriptCommit(t, root, width, 6, uint64(step+1))
			if transcript.Len() != len(items) {
				t.Fatalf("run %d step %d: count=%d want=%d", run, step, transcript.Len(), len(items))
			}
			assertOneNumbering(t, transcript)
		}
	}
	if retired == 0 || reflowed == 0 {
		t.Fatalf("retired=%d reflowed=%d", retired, reflowed)
	}
}
