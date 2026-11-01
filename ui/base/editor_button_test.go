package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestEditorSelectionWaitsForItsPressedButton(t *testing.T) {
	for _, release := range []struct {
		name    string
		button  input.Button
		action  input.MouseAction
		handled bool
	}{
		{name: "left", button: input.ButtonLeft, action: input.MouseUp, handled: true},
		{name: "unspecified", button: input.ButtonNone, action: input.MouseUp, handled: true},
		{name: "cancel", button: input.ButtonNone, action: input.MouseCancel},
	} {
		t.Run(release.name, func(t *testing.T) {
			var editor base.Editor
			editor.SetText("abcdef")
			root := newTestRoot(&editor)
			root.Draw(grid.NewSurface(10, 1).View())
			root.Handle(input.Mouse{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonLeft})
			root.Handle(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseDrag, Button: input.ButtonLeft})
			if editor.Selected() != "bc" {
				t.Fatalf("initial drag selected %q, want bc", editor.Selected())
			}
			if root.Handle(input.Mouse{Pos: image.Pt(3, 0), Action: input.MouseUp, Button: input.ButtonRight}) {
				t.Error("another button's release was consumed by the editor")
			}
			if !root.Handle(input.Mouse{Pos: image.Pt(5, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || editor.Selected() != "bcde" {
				t.Fatalf("another button's release ended the editor drag: selected %q", editor.Selected())
			}
			if handled := root.Handle(input.Mouse{Pos: image.Pt(-1, 0), Action: release.action, Button: release.button}); handled != release.handled {
				t.Fatalf("ending gesture returned %v, want %v", handled, release.handled)
			}
			if root.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) || editor.Selected() != "bcde" {
				t.Fatal("a released editor drag resumed or changed its selection")
			}
		})
	}
}

func TestEditorUsesKittyTextAndLayoutShortcutsIndependently(t *testing.T) {
	var editor base.Editor
	var parser input.Parser
	send := func(wire string) bool {
		t.Helper()
		events := parser.Feed([]byte(wire))
		if len(events) != 1 {
			t.Fatalf("%q: %v", wire, events)
		}
		return editor.Handle(events[0])
	}
	if !send("\x1b[50:64;2u") || !send("\x1b[0;;101:769u") || !send("\x1b[97:65;2;196u") || editor.Text() != "@e\u0301Ä" {
		t.Fatalf("reported text and shifted fallback: %q", editor.Text())
	}
	if send("\x1b[97::122;5:3u") || editor.Text() != "@e\u0301Ä" {
		t.Fatalf("release changed the editor: %q", editor.Text())
	}
	if !send("\x1b[1094::119;5u") || editor.Text() == "@e\u0301Ä" {
		t.Fatalf("Ctrl+w on a Cyrillic layout did not edit the document: %q", editor.Text())
	}
}

func TestKeypadTextAndNavigationReachTheEditor(t *testing.T) {
	var editor base.Editor
	var parser input.Parser
	// A localized decimal, a digit with no associated text, and a physical
	// keypad arrow exercise three distinct projections of the same wire identity.
	events := parser.Feed([]byte("\x1b[57400;1;49u\x1b[57409;1;44u\x1b[57401u\x1b[57417u\x1b[57402u"))
	for _, event := range events {
		if !editor.Handle(event) {
			t.Fatalf("unhandled keypad event %v", event)
		}
	}
	if got := editor.Text(); got != "1,32" {
		t.Fatalf("keypad editing produced %q", got)
	}
}
