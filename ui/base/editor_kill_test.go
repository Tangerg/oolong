package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestConsecutiveBackwardKillsAccumulateInReadingOrder(t *testing.T) {
	editor := &base.Editor{}
	editor.SetText("one two three")
	editor.Do(base.DeleteWordBack)
	editor.Do(base.DeleteWordBack)
	if got := editor.Text(); got != "one " {
		t.Fatalf("after kills = %q, want %q", got, "one ")
	}

	editor.Do(base.Yank)
	if got := editor.Text(); got != "one two three" {
		t.Fatalf("after yank = %q, want original text", got)
	}
}

func TestConsecutiveForwardKillsAccumulateAcrossALineBreak(t *testing.T) {
	editor := &base.Editor{}
	editor.SetText("keep one\ntwo")
	editor.SetCursor(0, len("keep "))
	editor.Do(base.KillToEnd)
	editor.Do(base.KillToEnd)
	editor.Do(base.KillToEnd)
	if got := editor.Text(); got != "keep " {
		t.Fatalf("after kills = %q, want %q", got, "keep ")
	}

	editor.Do(base.Yank)
	if got := editor.Text(); got != "keep one\ntwo" {
		t.Fatalf("after yank = %q, want original paragraph", got)
	}
}

func TestYankPopCyclesOlderKills(t *testing.T) {
	editor := &base.Editor{}
	for _, text := range []string{"one", "two", "three"} {
		editor.SetText(text)
		editor.Do(base.DeleteWordBack)
	}
	editor.SetText("")

	editor.Do(base.Yank)
	if got := editor.Text(); got != "three" {
		t.Fatalf("yank = %q, want newest kill", got)
	}
	editor.Do(base.YankPop)
	if got := editor.Text(); got != "two" {
		t.Fatalf("first yank-pop = %q, want next older kill", got)
	}
	editor.Do(base.YankPop)
	if got := editor.Text(); got != "one" {
		t.Fatalf("second yank-pop = %q, want oldest kill", got)
	}
	editor.Do(base.YankPop)
	if got := editor.Text(); got != "three" {
		t.Fatalf("cycling yank-pop = %q, want newest kill", got)
	}
}

func TestAnInterveningActionEndsYankPop(t *testing.T) {
	editor := &base.Editor{}
	for _, text := range []string{"old", "new"} {
		editor.SetText(text)
		editor.Do(base.DeleteWordBack)
	}
	editor.SetText("")
	editor.Do(base.Yank)
	editor.Insert("!")
	editor.Do(base.YankPop)
	if got := editor.Text(); got != "new!" {
		t.Fatalf("yank-pop replaced text after another edit: %q", got)
	}
}
