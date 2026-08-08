package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestConsecutiveBackwardKillsAccumulateInReadingOrder(t *testing.T) {
	editor := headless.NewEditor()
	editor.SetText("one two three")
	editor.Do(headless.DeleteWordBack)
	editor.Do(headless.DeleteWordBack)
	if got := editor.Text(); got != "one " {
		t.Fatalf("after kills = %q, want %q", got, "one ")
	}

	editor.Do(headless.Yank)
	if got := editor.Text(); got != "one two three" {
		t.Fatalf("after yank = %q, want original text", got)
	}
}

func TestConsecutiveForwardKillsAccumulateAcrossALineBreak(t *testing.T) {
	editor := headless.NewEditor()
	editor.SetText("keep one\ntwo")
	editor.SetCursor(0, len("keep "))
	editor.Do(headless.KillToEnd)
	editor.Do(headless.KillToEnd)
	editor.Do(headless.KillToEnd)
	if got := editor.Text(); got != "keep " {
		t.Fatalf("after kills = %q, want %q", got, "keep ")
	}

	editor.Do(headless.Yank)
	if got := editor.Text(); got != "keep one\ntwo" {
		t.Fatalf("after yank = %q, want original paragraph", got)
	}
}

func TestYankPopCyclesOlderKills(t *testing.T) {
	editor := headless.NewEditor()
	for _, text := range []string{"one", "two", "three"} {
		editor.SetText(text)
		editor.Do(headless.DeleteWordBack)
	}
	editor.SetText("")

	editor.Do(headless.Yank)
	if got := editor.Text(); got != "three" {
		t.Fatalf("yank = %q, want newest kill", got)
	}
	editor.Do(headless.YankPop)
	if got := editor.Text(); got != "two" {
		t.Fatalf("first yank-pop = %q, want next older kill", got)
	}
	editor.Do(headless.YankPop)
	if got := editor.Text(); got != "one" {
		t.Fatalf("second yank-pop = %q, want oldest kill", got)
	}
	editor.Do(headless.YankPop)
	if got := editor.Text(); got != "three" {
		t.Fatalf("cycling yank-pop = %q, want newest kill", got)
	}
}

func TestAnInterveningActionEndsYankPop(t *testing.T) {
	editor := headless.NewEditor()
	for _, text := range []string{"old", "new"} {
		editor.SetText(text)
		editor.Do(headless.DeleteWordBack)
	}
	editor.SetText("")
	editor.Do(headless.Yank)
	editor.Insert("!")
	editor.Do(headless.YankPop)
	if got := editor.Text(); got != "new!" {
		t.Fatalf("yank-pop replaced text after another edit: %q", got)
	}
}
