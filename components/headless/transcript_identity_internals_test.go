package headless

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

type identityBlock struct{}

func (identityBlock) Measure(int) int { return 1 }
func (identityBlock) Draw(grid.View)  {}

func TestTranscriptIdentityExhaustionCannotReuseAnOldIdentity(t *testing.T) {
	transcript := Transcript{transcriptState: transcriptState{first: exhaustedBlockID - 1}}
	transcript.Resize(1)
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
