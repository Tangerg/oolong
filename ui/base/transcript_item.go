package base

import (
	"errors"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/publication"
)

// Retention determines whether finalized content stays interactive or transfers to a publisher.
type Retention uint8

// ContentState describes a transcript reference, independently of source and writer progress.
type ContentState uint8

// TranscriptItem is a read-only receipt for a content reference. Retirement clears
// its document and body while preserving identity and the publication receipt.
// Retaining this handle never retains the completed conversation's source bytes.
type TranscriptItem struct {
	id       BlockID
	observed content.Revision // last preparation announced to the selection owner
	// source is the one place an item's content comes from; nil means retired.
	source  itemSource
	cancel  func()
	receipt publication.Receipt
	err     error
}

// itemSource is a live stream document, which can still revise, or passive
// content, which is final from the moment it is added.
type itemSource interface {
	Current() content.DocumentChange
}

type passiveContent struct{ body Block }

func (p passiveContent) Current() content.DocumentChange {
	return content.DocumentChange{Body: p.body, PreparedRevision: 1, State: content.Final}
}

// ID survives retirement without keeping the document or body alive.
func (i *TranscriptItem) ID() BlockID {
	if i == nil {
		return 0
	}
	return i.id
}

// State reports whether this reference can still revise or participate in the live transcript.
func (i *TranscriptItem) State() ContentState {
	if i == nil || i.source == nil {
		return ContentRetired
	}
	state := i.content()
	if state.Publishable() || state.State == content.Failed {
		return ContentRetained
	}
	return ContentLive
}

// Receipt reports permanent-output progress after admission; before admission its zero value is invalid.
func (i *TranscriptItem) Receipt() publication.Receipt {
	if i == nil {
		return publication.Receipt{}
	}
	return i.receipt
}

// Err preserves source, preparation and publication refusal causes while content remains readable.
func (i *TranscriptItem) Err() error {
	if i == nil {
		return nil
	}
	if i.source == nil {
		return i.err
	}
	state := i.source.Current()
	if state.SourceError == nil && state.PrepareError == nil {
		return i.err
	}
	return errors.Join(state.SourceError, state.PrepareError, i.err)
}

func (i *TranscriptItem) content() content.DocumentChange { return i.source.Current() }

func (i *TranscriptItem) retire() {
	i.err = i.Err()
	if i.cancel != nil {
		i.cancel()
		i.cancel = nil
	}
	i.source = nil
}
