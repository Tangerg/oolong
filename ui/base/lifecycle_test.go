package base_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func TestListDeclinesCoordinatesFromAReplacedCollection(t *testing.T) {
	var list base.List[string]
	list.SetItems([]string{"old zero", "old one"})
	root := newTestRoot(&list)
	surface := grid.NewSurface(12, 2)
	root.Draw(surface.View())
	list.SetItems([]string{"new zero", "new one"})
	if list.Handle(regionPress(0, 1)) || list.Selected() != 0 {
		t.Fatal("old row geometry selected a new item")
	}
	root.Draw(surface.View())
	if !list.Handle(regionPress(0, 1)) || list.Selected() != 1 {
		t.Fatal("committed replacement did not receive its own press")
	}
}

func TestEditorNavigationIsConsumedAndManualScrollingWins(t *testing.T) {
	var editor base.Editor
	editor.SetText(strings.Repeat("line\n", 30))
	editor.SetCursor(0, 0)
	root := newTestRoot(&editor)
	surface := grid.NewSurface(12, 5)
	root.Draw(surface.View())
	editor.Scroll().By(3)
	root.Draw(surface.View())
	root.Draw(grid.NewSurface(10, 4).View())
	if editor.Scroll().Offset() != 3 {
		t.Fatal("draw or resize replayed cursor navigation")
	}
	editor.SetCursor(20, 0)
	root.Draw(surface.View())
	if editor.Scroll().Offset() != 16 {
		t.Fatalf("cursor navigation was not revealed: %d", editor.Scroll().Offset())
	}
	editor.SetCursor(25, 0)
	editor.Scroll().ToTop()
	root.Draw(surface.View())
	if editor.Scroll().Offset() != 0 {
		t.Fatal("pending cursor navigation overrode later manual scrolling")
	}
	editor.Insert("x")
	root.Draw(surface.View())
	if editor.Scroll().Offset() != 21 {
		t.Fatal("editing did not return to the cursor")
	}
}

func TestEditorNavigationResolvesAgainstTheNextWrap(t *testing.T) {
	var editor base.Editor
	editor.SetText(strings.Repeat("a", 100))
	editor.SetCursor(0, 0)
	root := newTestRoot(&editor)
	root.Draw(grid.NewSurface(20, 3).View())
	editor.SetCursor(0, 90)
	root.Draw(grid.NewSurface(10, 3).View())
	if editor.Scroll().Offset() != 7 {
		t.Fatalf("navigation used the old wrap: offset %d, want 7", editor.Scroll().Offset())
	}
}

func TestEditorReleaseEndsDragWithoutClearingSelection(t *testing.T) {
	var editor base.Editor
	editor.SetText("abcdef")
	paintWidget(12, 1, &editor)
	editor.Handle(regionPress(1, 0))
	editor.Handle(regionDrag(4, 0))
	if !editor.Handle(regionRelease(-1, 0)) || editor.Selected() != "bcd" {
		t.Fatal("outside release did not settle the gesture and preserve selection")
	}
	if editor.Handle(regionDrag(5, 0)) || editor.Selected() != "bcd" {
		t.Fatal("selection state kept a released drag alive")
	}
}

type pointerStageWidget struct {
	pointer *base.Pointer
	area    image.Rectangle
}

func (w pointerStageWidget) Draw(frame scene.Frame) { w.pointer.Stage(frame, w.area) }

func stagePointer(p *base.Pointer, area image.Rectangle) {
	newTestRoot(pointerStageWidget{p, area}).Draw(grid.NewSurface(20, 20).View())
}

func TestPointerFollowsItsControlAndEndsAtAPresentationGap(t *testing.T) {
	var pointer base.Pointer
	stagePointer(&pointer, image.Rect(0, 0, 4, 1))
	pointer.Handle(regionPress(1, 0))
	stagePointer(&pointer, image.Rect(2, 0, 6, 1))
	pointer.Handle(regionRelease(5, 0))
	if !pointer.Clicked(input.ButtonLeft) {
		t.Fatal("moving the control lost its click")
	}
	pointer.Handle(regionPress(3, 0))
	stagePointer(&pointer, image.Rectangle{})
	stagePointer(&pointer, image.Rect(2, 0, 6, 1))
	pointer.Handle(regionRelease(3, 0))
	if pointer.Clicked(input.ButtonLeft) {
		t.Fatal("returning control revived an ended press")
	}
}

func TestFilterResetReturnsToTheFirstResult(t *testing.T) {
	var filter base.Filter[string]
	items := make([]string, 30)
	for i := range items {
		items[i] = "matching"
	}
	filter.SetItems(items, func(s string) string { return s })
	paintWidget(12, 3, &filter)
	filter.Scroll().By(5)
	paintWidget(12, 3, &filter)
	filter.SetPattern("match")
	paintWidget(12, 3, &filter)
	if filter.Scroll().Offset() != 0 {
		t.Fatal("new query kept an old result window")
	}
}

func TestEditorNavigationUsesTheLeastScrollAfterWidening(t *testing.T) {
	var editor base.Editor
	editor.SetText(strings.Repeat("a", 600))
	editor.SetCursor(0, 0)
	root := newTestRoot(&editor)
	root.Draw(grid.NewSurface(10, 3).View())
	editor.SetCursor(0, 200)
	root.Draw(grid.NewSurface(20, 3).View())
	if editor.Scroll().Offset() != 8 {
		t.Fatalf("old wrap over-scrolled new layout: %d, want 8", editor.Scroll().Offset())
	}
}

func TestSliderSettlesInvalidatedGestures(t *testing.T) {
	for _, cause := range []string{"outside press", "hidden track", "focus loss"} {
		t.Run(cause, func(t *testing.T) {
			slider := base.NewSlider(base.SliderConfig{Maximum: 100})
			track := &sliderTrack{control: slider, rect: image.Rect(0, 0, 11, 1)}
			root := newTestRoot(track)
			surface := grid.NewSurface(12, 2)
			root.Draw(surface.View())
			slider.Handle(regionPress(5, 0))
			switch cause {
			case "outside press":
				slider.Handle(regionPress(0, 1))
			case "hidden track":
				track.rect = image.Rectangle{}
				root.Draw(surface.View())
				track.rect = image.Rect(0, 0, 11, 1)
				root.Draw(surface.View())
			case "focus loss":
				testFocus(slider, false)
			}
			if slider.Handle(regionDrag(9, 0)) || slider.Handle(regionRelease(9, 0)) || slider.Value() != 50 {
				t.Fatal("invalidated gesture changed the slider")
			}
		})
	}
}

func TestDeferredSettingActionCannotEditAnotherRow(t *testing.T) {
	for _, replace := range []bool{false, true} {
		var resolve func()
		keys := &keymap.Map{Resolve: func(_ time.Duration, fn func()) func() {
			resolve = fn
			return func() {}
		}}
		g := input.Chord{Code: input.Character, Rune: 'g'}
		keys.Bind(base.Increase, g)
		keys.Bind(base.Decrease, g, g)
		changes := 0
		settings := &base.Settings[string]{
			EditKeys: keys,
			Change:   func(int, string, keymap.Action) bool { changes++; return true },
		}
		settings.List().SetItems([]string{"one", "two"})
		settings.Handle(input.Key{Code: input.Character, Rune: 'g'})
		if replace {
			settings.List().SetItems([]string{"new", "items"})
		} else {
			settings.List().Select(1)
		}
		resolve()
		if changes != 0 {
			t.Fatal("a deferred action edited another setting")
		}
		settings.Handle(input.Key{Code: input.Character, Rune: 'g'})
		if changes != 0 {
			t.Fatal("new row inherited a partial sequence")
		}
	}
}

func TestListBoundaryNavigationRevealsAnUnmovedCursor(t *testing.T) {
	var list base.List[int]
	list.SetItems(make([]int, 30))
	paintWidget(12, 3, &list)
	list.Scroll().By(5)
	paintWidget(12, 3, &list)
	list.Do(base.SelectFirst)
	paintWidget(12, 3, &list)
	if list.Selected() != 0 || list.Scroll().Offset() != 0 {
		t.Fatal("explicit navigation left its unchanged cursor off screen")
	}
}

func TestNewCompletionOfferCancelsPendingAcceptance(t *testing.T) {
	var resolve func()
	keys := &keymap.Map{Resolve: func(_ time.Duration, fn func()) func() {
		resolve = fn
		return func() {}
	}}
	g := input.Chord{Code: input.Character, Rune: 'g'}
	keys.Bind(base.Accept, g)
	keys.Bind(base.SelectNext, g, g)
	accepted := false
	completion := &base.Completion{Keys: keys, Accept: func(base.Candidate, base.Token) { accepted = true }}
	completion.Offer(base.Token{Query: "old"}, []base.Candidate{{Text: "old candidate"}})
	completion.Handle(input.Key{Code: input.Character, Rune: 'g'})
	completion.Offer(base.Token{Query: "new"}, []base.Candidate{{Text: "new candidate"}})
	resolve()
	if accepted || !completion.Open() {
		t.Fatal("a new query inherited pending acceptance")
	}
}
