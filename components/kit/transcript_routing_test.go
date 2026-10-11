package kit_test

import (
	"image"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

func TestTranscriptDragCannotUseAReplacementDocumentsCoordinates(t *testing.T) {
	first := session(t, 8, []string{"abcdef"})
	second := session(t, 8, []string{"uvwxyz", "next"})
	var selection headless.Selection
	view := &kit.Transcript{Content: first, Selection: &selection}
	root := headless.NewRoot(view)
	surface := grid.NewSurface(8, 2)
	root.Draw(surface.View())
	view.Handle(input.Mouse{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonLeft})
	view.Content = second
	if !view.Handle(input.Mouse{Pos: image.Pt(2, 0), Action: input.MouseDrag, Button: input.ButtonLeft}) {
		t.Fatal("pending replacement revoked the still-presented document")
	}
	start, end := selection.Range()
	root.Draw(surface.View())
	if view.Handle(input.Mouse{Pos: image.Pt(4, 1), Action: input.MouseDrag, Button: input.ButtonLeft}) {
		t.Fatal("replacement document advanced the original selection")
	}
	if gotStart, gotEnd := selection.Range(); gotStart != start || gotEnd != end {
		t.Fatalf("replacement changed selection from %v..%v to %v..%v", start, end, gotStart, gotEnd)
	}
	view.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
	if selection.Dragging() {
		t.Fatal("the original selection was left dragging")
	}
}

func TestTranscriptPressAndWheelRespectTheCommittedClip(t *testing.T) {
	content := session(t, 8, []string{"abcdef", "ghijkl", "mnopqr", "stuvwx"})
	var selection headless.Selection
	var scroll headless.Scroll
	view := &kit.Transcript{Content: content, Selection: &selection, Scroll: &scroll}
	frame := grid.NewSurface(3, 1).View().Sub(image.Rect(-2, -1, 6, 2))
	headless.NewRoot(view).Draw(frame)
	for _, point := range []image.Point{{0, 1}, {2, 0}, {5, 1}, {2, 2}} {
		for _, action := range []input.MouseAction{input.MouseDown, input.WheelDown} {
			if view.Handle(input.Mouse{Pos: point, Action: action, Button: input.ButtonLeft}) {
				t.Fatalf("invisible point %v consumed %v", point, action)
			}
		}
	}
	if selection.Active() || scroll.Offset() != 0 {
		t.Fatal("invisible input changed selection or scroll")
	}
	if !view.Handle(input.Mouse{Pos: image.Pt(2, 1), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("visible point was refused")
	}
	if start, _ := selection.Range(); start != (headless.Point{Row: 1, Col: 2}) {
		t.Fatalf("clip changed content coordinates: %v", start)
	}
	if !view.Handle(input.Mouse{Pos: image.Pt(99, 99), Action: input.MouseDrag, Button: input.ButtonLeft}) {
		t.Fatal("visible selection refused an outside continuation")
	}
	if _, end := selection.Range(); end != (headless.Point{Row: 1, Col: 4}) {
		t.Fatalf("drag escaped the visible cells: %v", end)
	}
}

func TestTranscriptPresentationReplacementCannotReviveAnOldGesture(t *testing.T) {
	for _, replacement := range []string{"content", "selection", "collapsed", "clipped"} {
		t.Run(replacement, func(t *testing.T) {
			content := session(t, 8, []string{"abcdef", "ghijkl"})
			var selection headless.Selection
			view := &kit.Transcript{Content: content, Selection: &selection}
			root := headless.NewRoot(view)
			surface := grid.NewSurface(8, 2)
			root.Draw(surface.View())
			view.Handle(input.Mouse{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonLeft})
			switch replacement {
			case "content":
				view.Content = session(t, 8, []string{"uvwxyz"})
				root.Draw(surface.View())
				view.Content = content
			case "selection":
				view.Selection = new(headless.Selection)
				root.Draw(surface.View())
				view.Selection = &selection
			case "collapsed":
				root.Draw(grid.NewSurface(0, 0).View())
			case "clipped":
				root.Draw(surface.View().Sub(image.Rect(9, 0, 17, 2)))
			}
			root.Draw(surface.View())
			if view.Handle(input.Mouse{Pos: image.Pt(4, 1), Action: input.MouseDrag, Button: input.ButtonLeft}) {
				t.Fatal("restored presentation revived the superseded gesture")
			}
			start, end := selection.Range()
			if start != (headless.Point{Row: 0, Col: 1}) || end != start {
				t.Fatalf("superseded gesture advanced selection: %v..%v", start, end)
			}
			if !view.Handle(input.Mouse{Action: input.MouseUp}) || selection.Dragging() {
				t.Fatal("superseded gesture did not settle its original selection")
			}
		})
	}
}

func TestTranscriptWheelUsesThePresentedScrollOwner(t *testing.T) {
	content := session(t, 8, []string{"abcdef", "ghijkl", "mnopqr", "stuvwx"})
	var first, second headless.Scroll
	view := &kit.Transcript{Content: content, Scroll: &first}
	root := headless.NewRoot(view)
	surface := grid.NewSurface(8, 1)
	root.Draw(surface.View())
	view.Scroll = &second
	if !view.Handle(input.Mouse{Action: input.WheelDown}) || first.Offset() == 0 || second.Offset() != 0 {
		t.Fatalf("pending replacement changed wheel owner: first=%d second=%d", first.Offset(), second.Offset())
	}
	before := first.Offset()
	root.Draw(surface.View())
	if !view.Handle(input.Mouse{Action: input.WheelDown}) || first.Offset() != before || second.Offset() == 0 {
		t.Fatalf("committed replacement retained old wheel owner: first=%d second=%d", first.Offset(), second.Offset())
	}
	view.Scroll = nil
	if !view.Handle(input.Mouse{Action: input.WheelUp}) || second.Offset() != 0 {
		t.Fatal("pending removal revoked the presented scroll owner")
	}
	root.Draw(surface.View())
	if view.Handle(input.Mouse{Action: input.WheelDown}) {
		t.Fatal("removed scroll owner still handled wheel input")
	}
}

func TestTranscriptClickRunsBelongToOnePresentationLifetime(t *testing.T) {
	for _, replacement := range []string{"content", "selection", "collapsed", "clipped", "cancelled"} {
		t.Run(replacement, func(t *testing.T) {
			content := session(t, 12, []string{"one two"})
			var selection headless.Selection
			view := &kit.Transcript{Content: content, Selection: &selection}
			root := headless.NewRoot(view)
			surface := grid.NewSurface(12, 1)
			root.Draw(surface.View())
			press := input.Mouse{Pos: image.Pt(1, 0), Action: input.MouseDown, Button: input.ButtonLeft, At: time.Unix(100, 0)}
			view.Handle(press)
			view.Handle(input.Mouse{Action: input.MouseUp})
			switch replacement {
			case "content":
				view.Content = session(t, 12, []string{"three four"})
				root.Draw(surface.View())
			case "selection":
				view.Selection = new(headless.Selection)
				root.Draw(surface.View())
				view.Selection = &selection
				root.Draw(surface.View())
			case "collapsed":
				root.Draw(grid.NewSurface(0, 0).View())
				root.Draw(surface.View())
			case "clipped":
				root.Draw(surface.View().Sub(image.Rect(13, 0, 25, 1)))
				root.Draw(surface.View())
			case "cancelled":
				view.Handle(input.Mouse{Action: input.MouseCancel})
			}
			press.At = press.At.Add(50 * time.Millisecond)
			view.Handle(press)
			start, end := selection.Range()
			if !selection.Dragging() || start != (headless.Point{Row: 0, Col: 1}) || end != start {
				t.Fatalf("new presentation inherited an old click run: %v..%v dragging=%t", start, end, selection.Dragging())
			}
			view.Handle(input.Mouse{Action: input.MouseUp})
			press.At = press.At.Add(50 * time.Millisecond)
			view.Handle(press)
			start, end = selection.Range()
			if selection.Dragging() || start.Col != 0 || end.Col < 2 {
				t.Fatalf("continuous presentation lost double clicks: %v..%v dragging=%t", start, end, selection.Dragging())
			}
		})
	}
}
