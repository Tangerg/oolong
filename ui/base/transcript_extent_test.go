package base_test

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

type widthDependentTranscriptBlock struct {
	wide, narrow int
}

func (b widthDependentTranscriptBlock) HeightForWidth(width int) int {
	if width < 20 {
		return b.narrow
	}
	return b.wide
}

func (widthDependentTranscriptBlock) Draw(grid.View) {}

func TestTranscriptRefusesUnrepresentableRowSpace(t *testing.T) {
	for _, operation := range []string{"initial frame", "new content", "reflow"} {
		t.Run(operation, func(t *testing.T) {
			transcript := transcriptModel(t, base.TranscriptConfig{})
			first := transcriptAdd(t, transcript, transcriptHuge{rows: 1})
			root := transcriptScene(t, transcript)
			if operation != "initial frame" {
				transcriptCommit(t, root, 20, 1, 1)
			}
			second := transcriptAdd(t, transcript, widthDependentTranscriptBlock{wide: 1, narrow: math.MaxInt})
			if operation == "reflow" {
				transcriptCommit(t, root, 20, 1, 2)
			}
			before := transcript.Layout()
			func() {
				defer func() {
					if recover() == nil {
						t.Error("unrepresentable row space was accepted")
					}
				}()
				_, _ = root.Prepare(grid.NewSurface(10, 1).View())
			}()
			if transcript.Width() != before.Width() || transcript.StartRow() != before.StartRow() ||
				transcript.Height() != before.Height() || transcript.EndRow() != before.EndRow() {
				t.Fatal("failed layout changed the committed row space")
			}
			if transcript.Len() != 2 || transcript.Block(first.ID()) == nil || transcript.Block(second.ID()) == nil {
				t.Fatal("failed layout discarded source references")
			}
			transcriptCommit(t, root, 20, 1, 3)
			if top, rows, ok := transcript.Extent(second.ID()); !ok || top != 1 || rows != 1 || transcript.EndRow() != 2 {
				t.Fatalf("valid retry did not settle: top=%d rows=%d exists=%v end=%d", top, rows, ok, transcript.EndRow())
			}
		})
	}
}

func TestTranscriptOverflowIncludesRetiredRowOrigin(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20, MaxItems: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.TryPublish(transcriptBody("occupied")); err != nil {
		t.Fatal(err)
	}
	transcript := transcriptModel(t, base.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
	t.Cleanup(transcript.Close)
	first := transcriptAdd(t, transcript, transcriptBody("first\nsecond"))
	root := transcriptScene(t, transcript)
	transcriptCommit(t, root, 20, 1, 1)
	publisher.Commit(publisher.Prepare(), 1)
	publisher.Written(1)
	publisher.FlushNotifications()
	if first.State() != base.ContentRetired {
		t.Fatal("prefix did not retire")
	}
	transcriptCommit(t, root, 20, 1, 2)
	second := transcriptAdd(t, transcript, widthDependentTranscriptBlock{wide: 1, narrow: math.MaxInt - 1})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("retired origin was omitted from overflow checking")
			}
		}()
		_, _ = root.Prepare(grid.NewSurface(10, 1).View())
	}()
	if transcript.StartRow() != 2 || transcript.EndRow() != 2 || transcript.Height() != 0 || transcript.Width() != 20 {
		t.Fatal("overflow changed the committed retired origin")
	}
	if transcript.Len() != 1 || transcript.Block(second.ID()) == nil {
		t.Fatal("overflow discarded retained content")
	}
	transcriptCommit(t, root, 20, 1, 3)
	if top, rows, ok := transcript.Extent(second.ID()); !ok || top != 2 || rows != 1 || transcript.EndRow() != 3 {
		t.Fatalf("retry lost the retired origin: top=%d rows=%d exists=%v end=%d", top, rows, ok, transcript.EndRow())
	}
}

func TestTranscriptUsesItsLastRepresentableRow(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, transcript, &measuredTranscriptBlock{rows: math.MaxInt - 1})
	last := transcriptAdd(t, transcript, &measuredTranscriptBlock{rows: 1})
	root := transcriptScene(t, transcript)
	transcriptCommit(t, root, 20, 1, 1)
	if transcript.Height() != math.MaxInt || transcript.EndRow() != math.MaxInt {
		t.Fatal("last representable row refused")
	}
	if id, offset, ok := transcript.At(math.MaxInt - 1); !ok || id != last.ID() || offset != 0 {
		t.Fatalf("last row=%d,%d,%v", id, offset, ok)
	}
	empty := transcriptAdd(t, transcript, &measuredTranscriptBlock{})
	transcriptCommit(t, root, 20, 1, 2)
	if top, rows, ok := transcript.Extent(empty.ID()); !ok || top != math.MaxInt || rows != 0 {
		t.Fatalf("empty extent=%d,%d,%v", top, rows, ok)
	}
}
