package base

import (
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
)

func expectTranscriptRevisionExhaustion(t *testing.T, change func()) {
	t.Helper()
	defer func() {
		if failure := recover(); failure != "base: transcript revisions exhausted" {
			t.Fatalf("revision exhaustion = %v", failure)
		}
	}()
	change()
	t.Fatal("exhausted revision accepted a mutation")
}

func TestTranscriptRevisionExhaustionPreservesOwnedReferences(t *testing.T) {
	for _, operation := range []string{"add", "accept", "close"} {
		t.Run(operation, func(t *testing.T) {
			transcript, err := NewTranscript(TranscriptConfig{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { transcript.revision = 0; transcript.Close() })
			item, err := transcript.Add(identityBlock{})
			if err != nil {
				t.Fatal(err)
			}
			item.observed = 1
			point := TextPoint{Content: item.id, Revision: 1}
			transcript.Selection().Begin(point)
			transcript.revision = math.MaxUint64
			next := transcript.next
			expectTranscriptRevisionExhaustion(t, func() {
				switch operation {
				case "add":
					_, _ = transcript.Add(identityBlock{})
				case "accept":
					transcript.accept(item, content.DocumentChange{PreparedRevision: 2})
				case "close":
					transcript.Close()
				}
			})
			if transcript.revision != math.MaxUint64 || transcript.Len() != 1 || transcript.next != next || transcript.closed || item.observed != 1 || item.State() != ContentRetained {
				t.Fatal("revision refusal partially advanced transcript ownership")
			}
			if transcript.Selection().Anchor() != point || !transcript.Selection().Dragging() {
				t.Fatal("revision refusal revoked the selected content")
			}
		})
	}
}

type countingTranscriptPublisher struct {
	*publication.Publisher
	calls int
}

func (p *countingTranscriptPublisher) TryPublish(body grid.Drawable) (publication.Receipt, error) {
	p.calls++
	return p.Publisher.TryPublish(body)
}

func TestTranscriptReservesRevisionsBeforeTransferringPublication(t *testing.T) {
	for _, operation := range []string{"immediate add", "resume prefix"} {
		t.Run(operation, func(t *testing.T) {
			publisher := &countingTranscriptPublisher{Publisher: transcriptPublisher(t)}
			transcript, err := NewTranscript(TranscriptConfig{Publisher: publisher, Retention: PublishCompleted})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { transcript.revision = 0; transcript.Close() })
			if operation == "resume prefix" {
				if _, err := publisher.TryPublish(identityBlock{}); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if _, err := transcript.Add(identityBlock{}); err != nil {
						t.Fatal(err)
					}
				}
				publisher.Commit(publisher.Prepare(), 1)
				publisher.Written(1)
			}
			publisher.calls = 0
			before, next := transcript.Len(), transcript.next
			transcript.revision = math.MaxUint64 - 1
			expectTranscriptRevisionExhaustion(t, func() {
				if operation == "immediate add" {
					_, _ = transcript.Add(identityBlock{})
				} else {
					publisher.FlushNotifications()
				}
			})
			if publisher.calls != 0 || transcript.Len() != before || transcript.next != next || transcript.revision != math.MaxUint64-1 {
				t.Fatal("revision refusal followed an irreversible publication or local admission")
			}
			if items, _ := publisher.Pending(); items != 0 {
				t.Fatal("revision refusal transferred content to the publisher")
			}
		})
	}
}

func TestTranscriptCanUseItsLastRepresentableRevision(t *testing.T) {
	transcript, err := NewTranscript(TranscriptConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { transcript.revision = 0; transcript.Close() })
	transcript.revision = math.MaxUint64 - 1
	item, err := transcript.Add(identityBlock{})
	if err != nil || item == nil || transcript.ContentRevision() != math.MaxUint64 || transcript.Len() != 1 {
		t.Fatalf("last revision = %d, item=%v, error=%v", transcript.ContentRevision(), item, err)
	}
}
