package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func itemKeys() *keymap.Map {
	keys := base.DefaultScrollKeys()
	keys.Rebind(base.SelectPrev, input.Keys{{Code: input.Up}})
	keys.Rebind(base.SelectNext, input.Keys{{Code: input.Down}})
	keys.Rebind(base.SelectFirst, input.Keys{{Code: input.Home}})
	keys.Rebind(base.SelectLast, input.Keys{{Code: input.End}})
	return keys
}

func current(t *testing.T, transcript *base.Transcript) base.BlockID {
	t.Helper()
	id, ok := transcript.Current()
	if !ok {
		t.Fatal("no current item")
	}
	return id
}

// Stepping through items stays among the items still retained, and the first step
// lands on an item the reader can see.
func TestTheCurrentItemMovesAmongRetainedItems(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{Keys: itemKeys()})
	first := transcriptAdd(t, transcript, transcriptBody("one")).ID()
	second := transcriptAdd(t, transcript, transcriptBody("two")).ID()
	third := transcriptAdd(t, transcript, transcriptBody("three")).ID()
	root := transcriptScene(t, transcript)
	transcriptCommit(t, root, 20, 2, 1)
	if _, ok := transcript.Current(); ok {
		t.Fatal("a transcript nobody stepped through has a current item")
	}

	steps := []struct {
		key  input.Code
		want base.BlockID
	}{{input.Up, third}, {input.Up, second}, {input.Home, first}, {input.Up, first}, {input.Down, second}, {input.End, third}, {input.Down, third}}
	for _, step := range steps {
		root.Handle(input.Key{Code: step.key})
		if got := current(t, transcript); got != step.want {
			t.Fatalf("after %v current = %d, want %d", step.key, got, step.want)
		}
	}

	if text, ok := transcript.Layout().BlockText(current(t, transcript)); !ok || text != "three" {
		t.Fatalf("current item text = %q, %t", text, ok)
	}
	transcript.SetCurrent(first)
	if transcript.DiscardBefore(second) != 1 || current(t, transcript) != second {
		t.Fatal("retiring the current item did not make the oldest retained item current")
	}
	if transcript.SetCurrent(first) {
		t.Fatal("a retired item became current")
	}
}

func TestDefaultTranscriptKeysScrollWithoutMovingTheCurrentItem(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, transcript, transcriptBody("one"))
	transcriptCommit(t, transcriptScene(t, transcript), 20, 2, 1)
	transcript.Handle(input.Key{Code: input.Up})
	if _, ok := transcript.Current(); ok {
		t.Fatal("a scroll key moved the current item")
	}
}

func TestPressingAnItemMakesItCurrent(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, transcript, transcriptBody("one"))
	second := transcriptAdd(t, transcript, transcriptBody("two")).ID()
	root := transcriptScene(t, transcript)
	transcript.Scroll().ToTop()
	transcriptCommit(t, root, 20, 2, 1)
	root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)})
	if current(t, transcript) != second {
		t.Fatal("a press on an item did not make it current")
	}
}

// A reader at the top of a long transcript who steps forward lands on the first item
// they can see rather than being carried to the end.
func TestTheFirstStepLandsInView(t *testing.T) {
	transcript := transcriptModel(t, base.TranscriptConfig{Keys: itemKeys()})
	first := transcriptAdd(t, transcript, transcriptBody("one")).ID()
	transcriptAdd(t, transcript, transcriptBody("two"))
	transcriptAdd(t, transcript, transcriptBody("three"))
	root := transcriptScene(t, transcript)
	transcript.Scroll().ToTop()
	transcriptCommit(t, root, 20, 2, 1)
	root.Handle(input.Key{Code: input.Down})
	if current(t, transcript) != first {
		t.Fatal("a forward step from the top left the view")
	}
	if transcript.Scroll().Offset() != 0 {
		t.Fatal("the first step scrolled away from where the reader was")
	}
}
