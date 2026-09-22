package headless

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

type identityBlock struct{}

func (identityBlock) HeightForWidth(int) int { return 1 }
func (identityBlock) Draw(grid.View)         {}

func TestTranscriptIdentityExhaustionCannotReuseAnOldIdentity(t *testing.T) {
	transcript := Transcript{first: exhaustedBlockID - 1}
	stageTranscriptForTest(&transcript, 1)
	id := transcript.Append(identityBlock{})
	if id != exhaustedBlockID-1 {
		t.Fatalf("last identity = %d, want %d", id, exhaustedBlockID-1)
	}
	transcript.Finish(id)
	if committed := transcript.Commit(func(Block, int) bool { return true }); committed != 1 {
		t.Fatalf("committed %d blocks, want one", committed)
	}
	if transcript.FirstBlock() != exhaustedBlockID || transcript.Block(id) != nil {
		t.Fatal("committing the last identity did not advance past it")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("an exhausted transcript reused a block identity")
		}
	}()
	transcript.Append(identityBlock{})
}

func TestTranscriptReleasesTheFullAllocationAfterPrefixCommits(t *testing.T) {
	var transcript Transcript
	stageTranscriptForTest(&transcript, 1)
	for range 2000 {
		transcript.Finish(transcript.Append(identityBlock{}))
	}
	oldLast := &transcript.blocks[len(transcript.blocks)-1]
	remaining := 1900
	transcript.Commit(func(Block, int) bool { remaining--; return remaining >= 0 })
	if len(transcript.blocks) != 100 {
		t.Fatalf("retained %d blocks", len(transcript.blocks))
	}
	if oldLast == &transcript.blocks[len(transcript.blocks)-1] {
		t.Fatal("small suffix pinned the original allocation")
	}
}
