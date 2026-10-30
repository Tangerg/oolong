package main

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func runPrompt(t *testing.T) (*uitest.Host, <-chan error) {
	t.Helper()
	host := uitest.New(t, uitest.Config{Width: 80, Height: 16})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return newPrompt(runtime).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	return host, done
}

func quit(t *testing.T, host *uitest.Host, done <-chan error) {
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
	p := newPrompt(nil)
	body := "one\ntwo\nthree\nfour"
	p.insertPaste(body)
	editor := p.composer.Controller()
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
	p := newPrompt(nil)
	p.history.Add(draft{text: "something earlier"})
	p.insertPaste("one\ntwo\nthree\nfour")
	p.composer.Controller().Insert("summarize")
	before := p.composer.Controller().Text()

	p.recallBack()
	p.recallForward()

	if got := p.composer.Controller().Text(); got != before {
		t.Fatalf("the draft came back as %q, want %q", got, before)
	}
	elements := p.composer.Controller().Elements()
	if len(elements) != 1 {
		t.Fatalf("the draft came back with %d attachments, want the one it had", len(elements))
	}
	if got := p.pastes[elements[0].ID]; got != "one\ntwo\nthree\nfour" {
		t.Fatalf("the attachment stands for %q", got)
	}
}

func TestARecalledEntryTakesTheAttachmentsItWasSentWith(t *testing.T) {
	p := newPrompt(nil)
	p.composer.Controller().Insert("[paste 4 lines] ")
	p.insertPaste("one\ntwo\nthree\nfour")
	p.submit()

	p.recallBack()
	elements := p.composer.Controller().Elements()
	if len(elements) != 1 {
		t.Fatalf("the entry came back with %d attachments, want one", len(elements))
	}
	if elements[0].Start == 0 {
		t.Fatal("the attachment was bound to the plain words that read like one")
	}

	// The same words again, with nothing attached, are not last time's attachment.
	p.composer.Controller().Clear()
	p.composer.Controller().Insert("[paste 4 lines] [paste 4 lines]")
	p.submit()
	p.recallBack()
	if got := p.composer.Controller().Elements(); len(got) != 0 {
		t.Fatalf("plain words came back with %d attachments", len(got))
	}
}

func TestRecallingADraftGivesBackTheWordsItHad(t *testing.T) {
	// InsertElement writes a separator after a chip, so putting a chip back writes
	// one too. Where the recorded document has a separator that is the one it means;
	// where the writer deleted it and carried straight on, the one the editor adds is
	// a word this draft never had.
	p := newPrompt(nil)
	p.history.Add(draft{text: "something earlier"})
	p.insertPaste("one\ntwo\nthree")
	p.composer.Controller().DeleteBack()
	p.composer.Controller().Insert("suffix")
	before := p.composer.Controller().Text()

	p.recallBack()
	p.recallForward()

	if got := p.composer.Controller().Text(); got != before {
		t.Fatalf("the draft came back as %q, want %q", got, before)
	}
	if got := p.composer.Controller().Elements(); len(got) != 1 {
		t.Fatalf("the draft came back with %d attachments, want the one it had", len(got))
	}
}

func TestHistoryDistinguishesAttachmentsWithTheSameLabel(t *testing.T) {
	p := newPrompt(nil)
	first := "one\ntwo\nthree"
	second := "four\nfive\nsix"
	p.insertPaste(first)
	p.submit()
	p.insertPaste(second)
	p.submit()

	for _, want := range []string{second, first} {
		p.recallBack()
		elements := p.composer.Controller().Elements()
		if len(elements) != 1 {
			t.Fatalf("recalled attachments = %v, want one", elements)
		}
		if got := p.pastes[elements[0].ID]; got != want {
			t.Fatalf("recalled payload = %q, want %q", got, want)
		}
	}
}
