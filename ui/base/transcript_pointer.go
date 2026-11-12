package base

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func (t *Transcript) handle(event scene.Event) scene.EventResult {
	if t.closed {
		return scene.EventResult{}
	}
	presented, valid := event.Read(&t.presentation)
	consumed := t.handleInput(event.Input(), presented, valid)
	mouse, ok := event.Input().(input.Mouse)
	return scene.EventResult{Consumed: consumed, Capture: consumed && ok && mouse.Action == input.MouseDown && mouse.Button == input.ButtonLeft, Focus: consumed && ok && mouse.Action == input.MouseDown}
}

// Handle changes reading intent through the same owner used by scene actions.
// It panics if the transcript is closed or its scene forbids model mutation.
func (t *Transcript) Handle(event input.Event) bool {
	return t.handleInput(event, t.presentation.Value(), true)
}

func (t *Transcript) handleInput(event input.Event, presented transcriptPresentation, valid bool) bool {
	t.assertMutable()
	if t.scroll.Handle(event, t.keys.current) {
		return true
	}
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	switch mouse.Action {
	case input.MouseCancel:
		t.selection.cancel()
		return false
	case input.MouseUp:
		if mouse.Releases(input.ButtonLeft) {
			was := t.selection.Dragging()
			t.selection.Done()
			return was
		}
		return false
	case input.MouseDown:
		if !valid {
			return false
		}
		return t.press(mouse, presented)
	case input.MouseDrag:
		if !valid {
			return false
		}
		return t.drag(mouse, presented)
	default:
		return false
	}
}

func (t *Transcript) press(mouse input.Mouse, p transcriptPresentation) bool {
	t.selection.Done()
	if mouse.Button != input.ButtonLeft {
		return false
	}
	if p.revision != t.revision {
		return false
	}
	at, ok := p.pointAt(mouse.Pos, false)
	if !ok {
		return false
	}
	switch t.selection.press(mouse, p.layout.key, at) {
	case 2:
		if t.selection.SelectWord(p.layout, at) {
			return true
		}
	case 3:
		if t.selection.SelectLine(p.layout, at) {
			return true
		}
	}
	selected, ok := p.layout.hit(at, true)
	if !ok {
		return false
	}
	if t.selection.setRange(selected, true) {
		t.element.Invalidate()
	}
	return true
}

func (t *Transcript) drag(mouse input.Mouse, p transcriptPresentation) bool {
	if !t.selection.Dragging() || !mouse.Drags(input.ButtonLeft) {
		return false
	}
	if p.revision != t.revision {
		return false
	}
	at, ok := p.pointAt(mouse.Pos, true)
	if !ok {
		return false
	}
	selected, ok := p.layout.hit(at, true)
	if !ok {
		return false
	}
	if selected.Start.Before(t.selection.Anchor()) {
		t.selection.Extend(selected.Start)
	} else {
		t.selection.Extend(selected.End)
	}
	return true
}

type transcriptPresentation struct {
	layout                TranscriptLayout
	revision              uint64
	body, header, visible image.Rectangle
	bodyFrom, headerFrom  int
}

func (p transcriptPresentation) pointAt(point image.Point, nearest bool) (Point, bool) {
	if p.visible.Empty() {
		return Point{}, false
	}
	if nearest {
		point.X = min(max(point.X, p.visible.Min.X), p.visible.Max.X-1)
		point.Y = min(max(point.Y, p.visible.Min.Y), p.visible.Max.Y-1)
	} else if !point.In(p.visible) {
		return Point{}, false
	}
	area, row, ok := p.rowAt(point)
	if !ok && nearest {
		area, row, ok = p.nearestRow(point.Y)
	}
	if !ok || area.Empty() {
		return Point{}, false
	}
	col := point.X - area.Min.X
	if nearest {
		col = min(max(col, 0), area.Dx()-1)
	} else if col < 0 || col >= area.Dx() {
		return Point{}, false
	}
	return Point{Row: row, Col: col}, true
}

func (p transcriptPresentation) rowAt(point image.Point) (image.Rectangle, int, bool) {
	switch {
	case point.In(p.header):
		return p.header, p.headerFrom + point.Y - p.header.Min.Y, true
	case point.In(p.body):
		return p.body, p.bodyFrom + point.Y - p.body.Min.Y, true
	default:
		return image.Rectangle{}, 0, false
	}
}

func (p transcriptPresentation) nearestRow(y int) (image.Rectangle, int, bool) {
	header, body := p.header.Intersect(p.visible), p.body.Intersect(p.visible)
	if !header.Empty() && y < p.body.Min.Y {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, p.headerFrom + at - p.header.Min.Y, true
	}
	if !body.Empty() {
		at := min(max(y, body.Min.Y), body.Max.Y-1)
		return p.body, p.bodyFrom + at - p.body.Min.Y, true
	}
	if !header.Empty() {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, p.headerFrom + at - p.header.Min.Y, true
	}
	return image.Rectangle{}, 0, false
}
