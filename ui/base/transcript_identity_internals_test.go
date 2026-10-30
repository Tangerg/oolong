package base

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

type identityBlock struct{}

func (identityBlock) HeightForWidth(int) int { return 1 }
func (identityBlock) Draw(grid.View)         {}

func transcriptPublisher(tb testing.TB) *publication.Publisher {
	tb.Helper()
	p, err := publication.New(publication.Config{Width: 80, MaxItems: 1})
	if err != nil {
		tb.Fatal(err)
	}
	return p
}

func TestTranscriptIdentityExhaustionCannotReuseAnOldIdentity(t *testing.T) {
	p := transcriptPublisher(t)
	transcript, err := NewTranscript(TranscriptConfig{Publisher: p, Retention: PublishCompleted})
	if err != nil {
		t.Fatal(err)
	}
	transcript.next = BlockID(math.MaxUint64 - 1)
	item, err := transcript.Add(identityBlock{})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID() != BlockID(math.MaxUint64-1) || item.State() != ContentRetired {
		t.Fatal("last identity was not retired")
	}
	if transcript.FirstBlock() != BlockID(math.MaxUint64) || transcript.Block(item.ID()) != nil {
		t.Fatal("retired identity still resolves")
	}
	p.Commit(p.Prepare(), 1)
	p.Written(1)
	if _, err := transcript.Add(identityBlock{}); err == nil {
		t.Fatal("exhausted transcript reused an identity")
	}
	if transcript.Len() != 0 {
		t.Fatal("exhaustion changed retained content")
	}
}

func TestTranscriptReleasesTheFullAllocationAfterPrefixRetires(t *testing.T) {
	p := transcriptPublisher(t)
	if _, err := p.TryPublish(identityBlock{}); err != nil {
		t.Fatal(err)
	}
	transcript, err := NewTranscript(TranscriptConfig{Publisher: p, Retention: PublishCompleted, MaxItems: 2000})
	if err != nil {
		t.Fatal(err)
	}
	for range 2000 {
		if _, err := transcript.Add(identityBlock{}); err != nil {
			t.Fatal(err)
		}
	}
	original := transcript.entries
	for sequence := uint64(1); sequence <= 1900; sequence++ {
		p.Commit(p.Prepare(), sequence)
		p.Written(sequence)
		p.FlushNotifications()
	}
	if len(transcript.entries) != 100 {
		t.Fatalf("retained %d items", len(transcript.entries))
	}
	for _, item := range original[:1900] {
		if item != nil && item.body != nil {
			t.Fatal("retired prefix still retains a content payload")
		}
	}
	if cap(transcript.entries) > 2*len(transcript.entries)+64 {
		t.Fatalf("small suffix pins capacity %d", cap(transcript.entries))
	}
}
