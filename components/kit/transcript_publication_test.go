package kit_test

import (
	"errors"
	"image"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/program"
)

func TestTranscriptRetainsBlocksWhenPublicationHasNoOwner(t *testing.T) {
	content := session(t, 20, []string{"retained"})
	content.Finish(0)
	view := &kit.Transcript{Content: content}
	var output program.InlineRuntime
	if got, err := view.Commit(&output, 0); got != 0 || !errors.Is(err, program.ErrStopped) || content.Len() != 1 {
		t.Fatalf("publication without an owner: committed=%d live=%d error=%v", got, content.Len(), err)
	}
}

func TestTranscriptRetainsNativeImagesThatScrollbackCannotOwn(t *testing.T) {
	content := session(t, 20, []string{"accepted"})
	content.Finish(0)
	imageID := content.Append(kit.Image{Of: graphics.Image{ID: 1, Size: image.Pt(20, 20)}, Cell: image.Pt(10, 10)})
	content.Finish(imageID)
	view := &kit.Transcript{Content: content}
	output := grid.NewInline(20, 3)
	if count, err := view.Commit(output, 0); count != 1 || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("image publication = %d, %v", count, err)
	}
	if content.Len() != 1 || content.FirstBlock() != imageID {
		t.Fatal("a native image was released without a scrollback owner")
	}
}

type publicationPrinter func(grid.Drawable) error

func (p publicationPrinter) Print(content grid.Drawable) error { return p(content) }

func TestTranscriptRetiresOnlyTheAcceptedPrefix(t *testing.T) {
	content := session(t, 20, []string{"accepted", "rows"}, []string{"refused"})
	content.Finish(0)
	content.Finish(1)
	var scroll headless.Scroll
	stageScroll(&scroll, 3, 1)
	scroll.ToBottom()
	view := &kit.Transcript{Content: content, Scroll: &scroll}
	cause := errors.New("printer refused output")
	calls := 0
	output := publicationPrinter(func(grid.Drawable) error {
		calls++
		if calls == 2 {
			return cause
		}
		return nil
	})
	if got, err := view.Commit(output, 0); got != 1 || !errors.Is(err, cause) {
		t.Fatalf("partial publication = %d, %v", got, err)
	}
	if content.Len() != 1 || content.FirstBlock() != 1 || content.StartRow() != 2 || scroll.Offset() != 0 {
		t.Fatalf("accepted prefix did not settle: live=%d first=%d start=%d offset=%d",
			content.Len(), content.FirstBlock(), content.StartRow(), scroll.Offset())
	}
	if got, err := view.Commit(output, 0); got != 1 || err != nil || calls != 3 {
		t.Fatalf("refused suffix did not remain available: committed=%d error=%v calls=%d", got, err, calls)
	}
}

func TestTranscriptPublicationKeepsItsOriginalModelOwners(t *testing.T) {
	content := session(t, 20, []string{"old", "rows"}, []string{"live"})
	content.Finish(0)
	var scroll headless.Scroll
	stageScroll(&scroll, 3, 1)
	scroll.ToBottom()
	var selection headless.Selection
	selection.Begin(headless.Point{Row: 0})
	sticky := &headless.Sticky{}
	sticky.SetBlocks([]headless.BlockID{0})
	view := &kit.Transcript{Content: content, Scroll: &scroll, Selection: &selection, Sticky: sticky}
	replacement := session(t, 20, []string{"other", "document", "rows"})
	var nextScroll headless.Scroll
	stageScroll(&nextScroll, 3, 1)
	nextScroll.By(1)
	var nextSelection headless.Selection
	nextSelection.Begin(headless.Point{Row: 0})
	nextSticky := &headless.Sticky{}
	nextSticky.SetBlocks([]headless.BlockID{0})
	output := publicationPrinter(func(grid.Drawable) error {
		view.Content, view.Scroll, view.Selection, view.Sticky = replacement, &nextScroll, &nextSelection, nextSticky
		return nil
	})
	if got, err := view.Commit(output, 0); got != 1 || err != nil {
		t.Fatalf("publication = %d, %v", got, err)
	}
	if scroll.Offset() != 0 || selection.Active() || len(sticky.Blocks()) != 0 {
		t.Fatal("the original model owners did not settle their accepted prefix")
	}
	if replacement.Len() != 1 || replacement.StartRow() != 0 || nextScroll.Offset() != 1 || !nextSelection.Active() || len(nextSticky.Blocks()) != 1 {
		t.Fatal("an old publication changed the replacement models")
	}
}

func TestTranscriptPublicationSettlesAcceptedPrefixOnAbnormalExit(t *testing.T) {
	for _, exit := range []string{"panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			content := session(t, 20, []string{"accepted", "rows"}, []string{"unsettled"})
			content.Finish(0)
			content.Finish(1)
			var scroll headless.Scroll
			stageScroll(&scroll, 3, 1)
			scroll.ToBottom()
			var selection headless.Selection
			selection.Begin(headless.Point{Row: 0})
			sticky := &headless.Sticky{}
			sticky.SetBlocks([]headless.BlockID{0})
			view := &kit.Transcript{Content: content, Scroll: &scroll, Selection: &selection, Sticky: sticky}
			calls := 0
			output := publicationPrinter(func(grid.Drawable) error {
				calls++
				if calls == 1 {
					return nil
				}
				if exit == "Goexit" {
					runtime.Goexit()
				}
				panic("printer interrupted")
			})
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if got := recover(); (exit == "panic" && got != "printer interrupted") || (exit == "Goexit" && got != nil) {
						t.Errorf("printer abnormal exit = %v", got)
					}
				}()
				_, _ = view.Commit(output, 0)
				t.Error("abnormal publication returned")
			}()
			<-done
			if content.Len() != 1 || content.StartRow() != 2 || scroll.Offset() != 0 || selection.Active() || sticky.Len() != 0 {
				t.Fatalf("accepted prefix left inconsistent models: live=%d start=%d offset=%d selected=%v pinned=%d",
					content.Len(), content.StartRow(), scroll.Offset(), selection.Active(), sticky.Len())
			}
		})
	}
}
