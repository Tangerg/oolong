package main

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

func runPrompt(t *testing.T) (*programtest.Host, <-chan error) {
	t.Helper()
	host := programtest.New(t, programtest.Config{Width: 80, Height: 16})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return headless.NewRoot(newPrompt(runtime))
			},
		})
	}()
	return host, done
}

func quit(t *testing.T, host *programtest.Host, done <-chan error) {
	t.Helper()
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	if err := <-done; err != nil {
		t.Fatalf("composer stopped with %v", err)
	}
}

func TestAReferenceCompletionBecomesOrdinaryPromptText(t *testing.T) {
	host, done := runPrompt(t)
	host.Shows(t, "Type @ to reference")
	host.Type("@arch")
	host.Shows(t, "ownership and layering")
	host.Press(input.Tab)
	host.Type(" explain the ownership rule")
	host.Press(input.Enter)
	host.Shows(t, "sent: @docs/architecture.md explain the ownership rule")
	quit(t, host, done)
}

func TestALargePasteIsOneApplicationOwnedElement(t *testing.T) {
	host, done := runPrompt(t)
	host.Shows(t, "paste three or more lines")
	host.Send(input.Paste{Text: "one\ntwo\nthree\nfour"})
	host.Shows(t, "[paste 4 lines]")
	host.Type(" summarize")
	host.Shows(t, "[paste 4 lines]")
	host.Press(input.Enter)
	host.Shows(t, "1 attached paste(s)")
	quit(t, host, done)
}

func TestUndoRestoresTheOriginalPastePayload(t *testing.T) {
	host, done := runPrompt(t)
	host.Shows(t, "paste three or more lines")
	host.Send(input.Paste{Text: "one\ntwo\nthree\nfour"})
	host.Shows(t, "[paste 4 lines]")
	host.Press(input.Backspace)
	host.Press(input.Backspace)
	host.Shows(t, "Type @ to reference")
	host.Send(input.Key{Code: input.Character, Rune: '_', Mods: input.Ctrl})
	host.Shows(t, "[paste 4 lines]")
	host.Press(input.Enter)
	host.Shows(t, "1 attached paste(s)")
	quit(t, host, done)
}

func TestHistoryRestoresAnEntryWithoutLosingTheEditingPath(t *testing.T) {
	host, done := runPrompt(t)
	host.Shows(t, "Type @ to reference")
	host.Type("alpha")
	host.Press(input.Enter)
	host.Type("draft")
	host.Press(input.Up)
	host.Type("-again")
	host.Press(input.Enter)
	host.Shows(t, "sent: alpha-again")
	quit(t, host, done)
}

func TestRemovedPasteRetainsOriginalIdentityAndBytesThroughUndo(t *testing.T) {
	p := &prompt{pastes: make(map[uint64]string)}
	body := "one\ntwo\nthree\nfour"
	p.insertPaste(body)
	editor := p.composer.Editor()
	id := editor.Elements()[0].ID
	editor.RemoveElement(id)
	if len(editor.Elements()) != 0 {
		t.Fatal("attachment was not removed")
	}
	p.releaseRemovedPastes()
	editor.Undo()
	p.releaseRemovedPastes()
	elements := editor.Elements()
	if len(elements) != 1 || elements[0].ID != id || p.pastes[id] != body {
		t.Fatalf("elements=%v payload=%q", elements, p.pastes[id])
	}
}

func TestAnUnsentDraftKeepsItsAttachmentsThroughHistory(t *testing.T) {
	// A history of lines keeps the draft's words, which is all a history of lines
	// can keep. What the words stand for is this application's, and putting the
	// draft back from its text alone left the chip's label as ordinary words with
	// the bytes already released.
	p := &prompt{pastes: make(map[uint64]string)}
	p.history.Add("something earlier")
	p.insertPaste("one\ntwo\nthree\nfour")
	p.composer.Editor().Insert("summarize")
	before := p.composer.Editor().Text()

	p.recallBack()
	p.recallForward()

	if got := p.composer.Editor().Text(); got != before {
		t.Fatalf("the draft came back as %q, want %q", got, before)
	}
	elements := p.composer.Editor().Elements()
	if len(elements) != 1 {
		t.Fatalf("the draft came back with %d attachments, want the one it had", len(elements))
	}
	if got := p.pastes[elements[0].ID]; got != "one\ntwo\nthree\nfour" {
		t.Fatalf("the attachment stands for %q", got)
	}
}

func TestARecalledEntryTakesTheAttachmentsItWasSentWith(t *testing.T) {
	// Looking for text that reads like a chip binds the attachment to whichever
	// label was written first, and keying the record by what the line says lets a
	// line sent again without its chips inherit the ones from last time.
	p := &prompt{pastes: make(map[uint64]string)}
	p.composer.Editor().Insert("[paste 4 lines] ")
	p.insertPaste("one\ntwo\nthree\nfour")
	p.submit()

	p.recallBack()
	elements := p.composer.Editor().Elements()
	if len(elements) != 1 {
		t.Fatalf("the entry came back with %d attachments, want one", len(elements))
	}
	if elements[0].Start == 0 {
		t.Fatal("the attachment was bound to the plain words that read like one")
	}

	// The same words again, with nothing attached, are not last time's attachment.
	p.composer.Editor().Clear()
	p.composer.Editor().Insert("[paste 4 lines] [paste 4 lines]")
	p.submit()
	p.recallBack()
	if got := p.composer.Editor().Elements(); len(got) != 0 {
		t.Fatalf("plain words came back with %d attachments", len(got))
	}
}
