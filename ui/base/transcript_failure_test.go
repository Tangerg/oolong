package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

func TestFailedPreparationDoesNotStrandAnAllowedPublicationPrefix(t *testing.T) {
	for _, publishPartial := range []bool{false, true} {
		publisher, err := publication.New(publication.Config{Width: 20})
		if err != nil {
			t.Fatal(err)
		}
		transcript := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted, PublishPartial: publishPartial})
		document := transcriptDocument(t)
		item, addErr := transcript.AddDocument(document)
		if addErr != nil {
			t.Fatal(addErr)
		}
		failure := errors.New("final renderer failed")
		if endErr := document.End(nil); endErr != nil {
			t.Fatal(endErr)
		}
		prepared := document.Snapshot().Prepare(t.Context(), func(context.Context, string) (grid.Drawable, error) { return nil, failure })
		if acceptErr := document.Accept(prepared); acceptErr != nil {
			t.Fatal(acceptErr)
		}
		if document.Current().State != content.Failed || !errors.Is(item.Err(), failure) {
			t.Fatal("retention changed or lost the failed source outcome")
		}
		next := transcriptAdd(t, transcript, transcriptBody("diagnostic and next reply"))
		if (item.State() == base.ContentRetired) != publishPartial || (next.State() == base.ContentRetired) != publishPartial {
			t.Fatalf("publishPartial=%t: failed=%v next=%v", publishPartial, item.State(), next.State())
		}
		if publishPartial && (item.Receipt().Snapshot().State != publication.Written || next.Receipt().Snapshot().ID <= item.Receipt().Snapshot().ID) {
			t.Fatal("empty failed capture did not preserve publication order")
		}
		transcript.Close()
	}
}
