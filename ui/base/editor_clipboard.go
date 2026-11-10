package base

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// ErrPasteStale reports a read whose original edit location no longer exists.
var ErrPasteStale = errors.New("base: clipboard read target changed")

type pasteTarget struct {
	attachment scene.Attachment
	session    *EditSession
	revision   uint64
	cursor     Caret
	anchor     Caret
	selecting  bool
}

// Paste binds the read to this editor's document, selection and current attachment.
// Changing focus alone leaves that receiver intact. A different document revision,
// selection or attachment at completion reports ErrPasteStale. A never-mounted editor
// may complete only if it has not entered a scene lifetime; detached editors refuse.
// complete runs on this UI owner with success or failure. Caller cancellation or the
// clipboard provider's owner ending revokes it; completion is not promised afterward.
// A returned error means no read was accepted. complete must not be nil.
func (e *Editor) Paste(ctx context.Context, complete func(error)) error {
	e.beginViewUpdate()
	defer e.endViewUpdate()
	e.endTyping()
	if complete == nil {
		return errors.New("base: paste requires a completion receiver")
	}
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	if e.clipboard == nil {
		return errors.ErrUnsupported
	}
	element := e.element
	if !element.Alive() && element.HasBeenAttached() {
		return scene.ErrDetached
	}
	target := pasteTarget{
		attachment: element.Attachment(), session: e.document(), revision: e.Revision(),
		cursor: Caret{Line: e.line, Byte: e.byteOffset}, anchor: e.anchor, selecting: e.selection != editorSelectionNone,
	}
	return e.clipboard.Read(ctx, func(value string, err error) {
		if ctx.Err() != nil {
			return
		}
		if !target.current(e) {
			err = errors.Join(err, ErrPasteStale)
		}
		if err == nil && value != "" {
			e.Insert(value)
		}
		complete(err)
	})
}

func (p pasteTarget) current(e *Editor) bool {
	if p.session != e.document() || p.revision != e.Revision() ||
		p.cursor != (Caret{Line: e.line, Byte: e.byteOffset}) || p.anchor != e.anchor || p.selecting != (e.selection != editorSelectionNone) {
		return false
	}
	target := p.attachment.Target()
	if target.IsZero() {
		return !e.element.HasBeenAttached()
	}
	return target.Valid() && e.element.Target() == target
}

// PasteError is the most recently delivered keyboard paste failure. Programmatic
// requests receive their own errors through Paste's return and completion callback.
func (e *Editor) PasteError() error { return e.pasteErr }

func (e *Editor) pasteFromInput() {
	e.setPasteError(nil)
	if err := e.Paste(context.Background(), func(err error) {
		if !errors.Is(err, ErrPasteStale) {
			e.setPasteError(err)
		}
	}); err != nil {
		e.setPasteError(err)
	}
}

func (e *Editor) setPasteError(err error) {
	e.requireMutable()
	changed := e.pasteErr != nil || err != nil
	e.pasteErr = err
	if changed {
		e.element.Invalidate()
	}
}

// Clipboard supplies the user-facing copy and read capability.
//
// A runtime adapter commonly provides it, but the interface is declared here where
// it is consumed. A caller with somewhere else to put text is equally valid.
type Clipboard interface {
	// Copy puts text where a paste would find it, reporting false for text it will
	// not carry.
	Copy(text string) bool
	// Read accepts a directed read. receive runs on the caller's UI owner sequence,
	// including for failure; it must never be invoked by a background worker.
	// Caller cancellation or this provider's owner ending revokes receive; no
	// callback is promised afterward. A returned error means no read was accepted.
	Read(ctx context.Context, receive func(string, error)) error
}
