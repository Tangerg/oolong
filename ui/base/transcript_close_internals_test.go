package base

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestTranscriptCloseReleasesItsCommittedContentProjection(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	transcript, err := NewTranscript(TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	item, err := transcript.Add(&retainedBlock{payload: []byte("retired body")})
	if err != nil {
		t.Fatal(err)
	}
	root := scenes.Mount(transcript)
	defer root.Root.Close()
	root.Draw(grid.NewSurface(20, 3).View())
	retained := transcript.Layout()
	if retained.Block(item.ID()) == nil {
		t.Fatal("committed layout has no retained body")
	}
	transcript.Close()
	if len(transcript.geometry.Value().blocks) != 0 || len(transcript.presentation.Value().layout.blocks) != 0 {
		t.Fatal("closed transcript still owns content through its committed projections")
	}
	if retained.Block(item.ID()) == nil {
		t.Fatal("closing an owner changed a caller's immutable layout")
	}
}
