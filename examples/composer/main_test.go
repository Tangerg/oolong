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
	host := programtest.New(t, 80, 16)
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
