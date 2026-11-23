package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/publication"
)

// selectRow selects the first character of a committed row through the same layout
// hit testing a pointer uses.
func selectRow(t *testing.T, model *base.Transcript, row int) {
	t.Helper()
	selected, ok := model.Layout().Hit(base.Point{Row: row})
	if !ok {
		t.Fatalf("row %d has no text to select", row)
	}
	model.Selection().Set(selected)
}

func TestReplaceRevisesPassiveContentInPlace(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	first := transcriptAdd(t, model, transcriptBody("a0"))
	second := transcriptAdd(t, model, transcriptBody("b0"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 6, 1)
	selectRow(t, model, 0)
	revision := model.ContentRevision()

	replacement := transcriptBody("b0\nb1\nb2")
	if err := model.Replace(second.ID(), replacement); err != nil {
		t.Fatal(err)
	}
	if model.Block(second.ID()) != replacement || model.ContentRevision() == revision {
		t.Fatal("replacement did not become the item's content revision")
	}
	if !model.Selection().Active() {
		t.Fatal("a selection over untouched content ended")
	}
	surface := transcriptCommit(t, root, 12, 6, 2)
	if rowText(surface.View(), 3) != "b2" {
		t.Fatalf("replacement was not drawn in place: row 3 = %q", rowText(surface.View(), 3))
	}

	selectRow(t, model, 2)
	if err := model.Replace(second.ID(), transcriptBody("b")); err != nil {
		t.Fatal(err)
	}
	if model.Selection().Active() {
		t.Fatal("a selection naming replaced text survived")
	}
	if model.Block(first.ID()) == nil || second.State() != base.ContentRetained {
		t.Fatal("replacement changed the identity or state of retained content")
	}
}

func TestReplaceRefusesWhatItDoesNotOwn(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	document, err := model.AddDocument(transcriptDocument(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Replace(document.ID(), transcriptBody("x")); err == nil {
		t.Fatal("a document's content was replaced around its document")
	}
	if err := model.Replace(document.ID()+1, transcriptBody("x")); !errors.Is(err, base.ErrNotRetained) {
		t.Fatalf("an unissued identity returned %v", err)
	}
	passive := transcriptAdd(t, model, transcriptBody("p"))
	if err := model.Replace(passive.ID(), nil); err == nil {
		t.Fatal("nil content was accepted")
	}
}

func TestReplacingContentAboveTheReaderKeepsTheirPlace(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	above := transcriptAdd(t, model, transcriptBody("a0\na1"))
	transcriptAdd(t, model, transcriptBody("b0\nb1\nb2\nb3"))
	transcriptAdd(t, model, transcriptBody("c0\nc1\nc2\nc3"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 3, 1)
	model.Scroll().ToTop()
	model.Scroll().By(6)
	if top := rowText(transcriptCommit(t, root, 12, 3, 2).View(), 0); top != "c0" {
		t.Fatalf("setup shows %q at the top, want c0", top)
	}
	if err := model.Replace(above.ID(), transcriptBody("a0\na1\na2\na3\na4")); err != nil {
		t.Fatal(err)
	}
	if top := rowText(transcriptCommit(t, root, 12, 3, 3).View(), 0); top != "c0" {
		t.Fatalf("growth above moved the reader to %q, want c0", top)
	}
}

func TestDiscardBeforeKeepsTheReaderOnTheSameText(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	first := transcriptAdd(t, model, transcriptBody("a0\na1\na2"))
	transcriptAdd(t, model, transcriptBody("b0\nb1\nb2"))
	kept := transcriptAdd(t, model, transcriptBody("c0\nc1\nc2\nc3"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 2, 1)
	model.Scroll().ToTop()
	model.Scroll().By(7)
	if top := rowText(transcriptCommit(t, root, 12, 2, 2).View(), 0); top != "c1" {
		t.Fatalf("setup shows %q at the top, want c1", top)
	}
	selectRow(t, model, 0)

	if released := model.DiscardBefore(kept.ID()); released != 2 {
		t.Fatalf("released %d items, want 2", released)
	}
	if model.Len() != 1 || model.FirstBlock() != kept.ID() || model.Block(first.ID()) != nil {
		t.Fatal("released content is still retained")
	}
	if model.Selection().Active() {
		t.Fatal("a selection over released content survived")
	}
	if top := rowText(transcriptCommit(t, root, 12, 2, 3).View(), 0); top != "c1" {
		t.Fatalf("release moved the reader to %q, want c1", top)
	}
	if err := model.Replace(first.ID(), transcriptBody("x")); !errors.Is(err, base.ErrNotRetained) {
		t.Fatalf("replacing released content returned %v", err)
	}
	if released := model.DiscardBefore(kept.ID()); released != 0 {
		t.Fatalf("a repeated release released %d more", released)
	}
}

func TestDiscardBeforeLeavesAFollowingReaderAtTheEnd(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, transcriptBody("a0\na1\na2"))
	last := transcriptAdd(t, model, transcriptBody("b0\nb1\nb2"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 2, 1)
	model.DiscardBefore(last.ID())
	surface := transcriptCommit(t, root, 12, 2, 2)
	if !model.Scroll().FollowingEnd() || rowText(surface.View(), 1) != "b2" {
		t.Fatal("release stopped a following reader from showing the end")
	}
}

func TestRevealBlockShowsTheCurrentRevisionWhole(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	target := transcriptAdd(t, model, transcriptBody("t0"))
	if model.RevealBlock(target.ID()) {
		t.Fatal("a reveal was accepted before any frame gave the transcript a width")
	}
	transcriptAdd(t, model, transcriptBody("u0\nu1\nu2\nu3\nu4\nu5"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 3, 1)
	if err := model.Replace(target.ID(), transcriptBody("t0\nt1\nt2")); err != nil {
		t.Fatal(err)
	}
	if !model.RevealBlock(target.ID()) || model.RevealBlock(target.ID()+5) {
		t.Fatal("reveal accepted an unknown identity or refused a retained one")
	}
	surface := transcriptCommit(t, root, 12, 3, 2)
	for row, want := range []string{"t0", "t1", "t2"} {
		if got := rowText(surface.View(), row); got != want {
			t.Fatalf("row %d = %q, want %q", row, got, want)
		}
	}
}

// The scroll offset counts from the retained start, which a release moves past the
// committed frame's start. Reading the anchor without that correction lands on
// content that is still retained, and the reader jumps to it.
func TestAPartialReleaseReadsTheAnchorFromTheRetainedStart(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, transcriptBody("a0\na1\na2"))
	middle := transcriptAdd(t, model, transcriptBody("b0\nb1\nb2"))
	transcriptAdd(t, model, transcriptBody("c0\nc1\nc2\nc3"))
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 12, 2, 1)
	model.Scroll().ToTop()
	model.Scroll().By(7)
	transcriptCommit(t, root, 12, 2, 2)
	model.DiscardBefore(middle.ID())
	if top := rowText(transcriptCommit(t, root, 12, 2, 3).View(), 0); top != "c1" {
		t.Fatalf("partial release moved the reader to %q, want c1", top)
	}
}

// Releasing an unfinished item can leave finished content at the front of a
// publishing transcript; it is published then, not held until something else changes.
func TestDiscardBeforePublishesFinishedContentItExposes(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20})
	if err != nil {
		t.Fatal(err)
	}
	model := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
	t.Cleanup(model.Close)
	live, err := model.AddDocument(transcriptDocument(t))
	if err != nil {
		t.Fatal(err)
	}
	finished := transcriptAdd(t, model, transcriptBody("done"))
	if finished.State() != base.ContentRetained {
		t.Fatal("finished content behind a live item was published early")
	}
	if released := model.DiscardBefore(finished.ID()); released != 1 || live.State() != base.ContentRetired {
		t.Fatalf("released %d, live state %v", released, live.State())
	}
	if finished.State() != base.ContentRetired || finished.Receipt().Snapshot().State != publication.Accepted {
		t.Fatal("finished content left at the front was not published")
	}
}
