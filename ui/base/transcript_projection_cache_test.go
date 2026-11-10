package base_test

import (
	"context"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/text"
)

type countedTextProjection struct {
	*text.Block
	measurements, projections int
}

func newCountedTextProjection(source string) *countedTextProjection {
	return &countedTextProjection{Block: text.NewBlock(text.BlockConfig{Lines: []text.Line{text.Of(source, grid.Style{})}, Wrap: true})}
}

func (b *countedTextProjection) HeightForWidth(width int) int {
	b.measurements++
	return b.Block.HeightForWidth(width)
}

func (b *countedTextProjection) Rows(width int) []text.Row {
	b.projections++
	return b.Block.Rows(width)
}

func TestTranscriptUnchangedFramesReuseTextProjection(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	body := newCountedTextProjection("alpha beta")
	transcriptAdd(t, model, body)
	root := transcriptScene(t, model)
	for sequence := uint64(1); sequence <= 3; sequence++ {
		transcriptCommit(t, root, 20, 2, sequence)
		if rows := model.Layout().Rows(0, 1); len(rows) != 1 || rows[0].Content.String() != "alpha beta" {
			t.Fatalf("unchanged projection returned %v", rows)
		}
	}
	if body.measurements != 1 || body.projections != 1 {
		t.Fatalf("unchanged content measured %d times and projected %d times", body.measurements, body.projections)
	}
}

func TestTranscriptDocumentChangeReplacesCachedProjection(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	document := transcriptDocument(t)
	if _, err := model.AddDocument(document); err != nil {
		t.Fatal(err)
	}
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 20, 2, 1)
	before := model.ContentRevision()
	if err := document.Append([]byte("new text")); err != nil {
		t.Fatal(err)
	}
	body := newCountedTextProjection(document.Snapshot().Text())
	prepared := document.Snapshot().Prepare(context.Background(), func(context.Context, string) (grid.Drawable, error) { return body, nil })
	if err := document.Accept(prepared); err != nil {
		t.Fatal(err)
	}
	if model.ContentRevision() == before {
		t.Fatal("accepted document content did not advance its public revision")
	}
	for sequence := uint64(2); sequence <= 3; sequence++ {
		transcriptCommit(t, root, 20, 2, sequence)
		if rows := model.Layout().Rows(0, 1); len(rows) != 1 || rows[0].Content.String() != "new text" {
			t.Fatalf("changed document kept old projection: %v", rows)
		}
	}
	if body.measurements != 1 || body.projections != 1 {
		t.Fatal("settled document projection was not reused")
	}
}

func TestTranscriptAbortedResizeKeepsCommittedProjection(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	body := newCountedTextProjection("abcdefghij")
	transcriptAdd(t, model, body)
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 20, 3, 1)
	before := model.Layout()
	contentRevision := model.ContentRevision()
	candidate, err := root.Prepare(grid.NewSurface(4, 3).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Abort()
	if model.Layout().Width() != 20 || model.Layout().Height() != 1 {
		t.Fatal("aborted width replaced the committed text projection")
	}
	transcriptCommit(t, root, 20, 3, 2)
	if body.measurements != 2 || body.projections != 2 {
		t.Fatal("aborted candidate displaced the reusable committed projection")
	}
	transcriptCommit(t, root, 4, 3, 3)
	transcriptCommit(t, root, 4, 3, 4)
	if model.ContentRevision() != contentRevision {
		t.Fatal("a layout width advanced the content revision")
	}
	if model.Layout().Width() != 4 || model.Layout().Height() != 3 || body.measurements != 3 || body.projections != 3 {
		t.Fatalf("resize projection: width=%d height=%d measurements=%d projections=%d", model.Layout().Width(), model.Layout().Height(), body.measurements, body.projections)
	}
	if rows := before.Rows(0, 1); len(rows) != 1 || rows[0].Content.String() != "abcdefghij" {
		t.Fatal("new width mutated the earlier immutable projection")
	}
}

func TestTranscriptRetirementRemovesReusableProjection(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.TryPublish(transcriptBody("occupied")); err != nil {
		t.Fatal(err)
	}
	model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
	t.Cleanup(model.Close)
	item := transcriptAdd(t, model, transcriptBody("retained"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 20, 2, 1)
	transcriptCommit(t, root, 20, 2, 2)
	before := model.Layout()
	publisher.Commit(publisher.Prepare(), 1)
	publisher.Written(1)
	publisher.FlushNotifications()
	if item.State() != base.ContentRetired {
		t.Fatal("publication did not retire content")
	}
	transcriptCommit(t, root, 20, 2, 3)
	if model.Layout().Block(item.ID()) != nil || model.Layout().Height() != 0 || len(model.Layout().Rows(0, 2)) != 0 {
		t.Fatal("retirement left content in the reusable projection")
	}
	if before.Block(item.ID()) == nil {
		t.Fatal("retirement mutated a caller's immutable snapshot")
	}
}
