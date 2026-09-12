package headless_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestPointerCaptureCannotResumeAfterAPresentationGap(t *testing.T) {
	for _, interveningDrag := range []bool{false, true} {
		child := &pointerTarget{takes: true}
		fixture := &pointerRegionFixture{area: image.Rect(1, 1, 5, 4), child: child}
		fixture.draw()
		fixture.Handle(regionPress(2, 2))
		fixture.child = nil
		fixture.draw()
		if interveningDrag {
			fixture.Handle(regionDrag(2, 2))
		}
		fixture.child = child
		fixture.draw()
		fixture.Handle(regionDrag(2, 2))
		fixture.Handle(regionRelease(2, 2))
		if len(child.events) != 1 {
			t.Fatalf("returning child inherited an ended gesture: %v", child.events)
		}
		if !fixture.Handle(regionPress(2, 2)) {
			t.Fatal("new gesture could not begin")
		}
	}
}

func TestOutsidePressSettlesPointerCapture(t *testing.T) {
	child := &pointerTarget{takes: true}
	fixture := &pointerRegionFixture{area: image.Rect(1, 1, 5, 4), child: child}
	fixture.draw()
	fixture.Handle(regionPress(2, 2))
	fixture.Handle(regionPress(9, 6))
	fixture.Handle(regionDrag(9, 6))
	fixture.Handle(regionRelease(9, 6))
	if len(child.events) != 1 {
		t.Fatal("outside press left the previous capture alive")
	}
}

func TestListDeclinesCoordinatesFromAReplacedCollection(t *testing.T) {
	var list headless.List[string]
	list.SetItems([]string{"old zero", "old one"})
	root := headless.NewRoot(&list)
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

func TestTabsKeepPointerGesturesWithThePresentedPane(t *testing.T) {
	first, second := &pointerTarget{takes: true}, &pointerTarget{takes: true}
	tabs := headless.NewTabs(headless.TabsConfig{Items: []headless.Tab{{Of: first}, {Of: second}}})
	root := headless.NewRoot(tabs)
	surface := grid.NewSurface(12, 3)
	root.Draw(surface.View())
	tabs.Handle(regionPress(1, 1))
	tabs.Select(1)
	tabs.Handle(regionDrag(1, 1))
	if len(first.events) != 2 || len(second.events) != 0 {
		t.Fatal("undrawn pane inherited the gesture")
	}
	root.Draw(surface.View())
	tabs.Handle(regionDrag(1, 1))
	tabs.Handle(regionRelease(1, 1))
	if len(second.events) != 0 {
		t.Fatal("newly drawn pane inherited another pane's gesture")
	}
	tabs.Set(headless.Tab{Of: second}, headless.Tab{Of: first})
	if tabs.SelectPresented(0) {
		t.Fatal("replaced tab collection accepted an old strip index")
	}
	root.Draw(surface.View())
	if !tabs.SelectPresented(0) || tabs.Selected() != 0 {
		t.Fatal("committed strip index was rejected")
	}
}

type tallPointerTarget struct{ pointerTarget }

func (*tallPointerTarget) HeightForWidth(int) int { return 30 }

func TestViewportCaptureTracksContentAndItsCurrentOffset(t *testing.T) {
	first := &tallPointerTarget{takes: true}
	second := &tallPointerTarget{takes: true}
	viewport := headless.NewViewport(first)
	root := headless.NewRoot(viewport)
	surface := grid.NewSurface(12, 3)
	root.Draw(surface.View())
	viewport.Handle(regionPress(1, 1))
	viewport.Scroll().By(4)
	root.Draw(surface.View())
	viewport.Handle(regionDrag(1, 5))
	if first.last() != image.Pt(1, 9) {
		t.Fatalf("capture lost the content's current origin: %v", first.last())
	}
	viewport.SetContent(second)
	root.Draw(surface.View())
	viewport.Handle(regionDrag(1, 1))
	viewport.Handle(regionRelease(1, 1))
	if len(second.events) != 0 {
		t.Fatal("viewport handed a gesture to replacement content")
	}
	if viewport.Handle(regionPress(1, 4)) {
		t.Fatal("content outside the viewport accepted a press")
	}
}

func TestEditorNavigationIsConsumedAndManualScrollingWins(t *testing.T) {
	var editor headless.Editor
	editor.SetText(strings.Repeat("line\n", 30))
	editor.SetCursor(0, 0)
	root := headless.NewRoot(&editor)
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
	var editor headless.Editor
	editor.SetText(strings.Repeat("a", 100))
	editor.SetCursor(0, 0)
	root := headless.NewRoot(&editor)
	root.Draw(grid.NewSurface(20, 3).View())
	editor.SetCursor(0, 90)
	root.Draw(grid.NewSurface(10, 3).View())
	if editor.Scroll().Offset() != 7 {
		t.Fatalf("navigation used the old wrap: offset %d, want 7", editor.Scroll().Offset())
	}
}

func TestEditorReleaseEndsDragWithoutClearingSelection(t *testing.T) {
	var editor headless.Editor
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
	pointer *headless.Pointer
	area    image.Rectangle
}

func (w pointerStageWidget) Draw(frame headless.Frame) { w.pointer.Stage(frame, w.area) }

func stagePointer(p *headless.Pointer, area image.Rectangle) {
	headless.NewRoot(pointerStageWidget{p, area}).Draw(grid.NewSurface(20, 20).View())
}

func TestPointerFollowsItsControlAndEndsAtAPresentationGap(t *testing.T) {
	var pointer headless.Pointer
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

func TestDialogTriggerClicksWithoutAnInterveningDraw(t *testing.T) {
	dialog := headless.NewDialog(headless.DialogConfig{Stack: &headless.Stack{}, Content: &panel{place: middle(8, 3)}})
	trigger := dialog.Trigger("Open", nil)
	paintWidget(12, 1, trigger)
	trigger.Handle(regionPress(1, 0))
	trigger.Handle(regionRelease(1, 0))
	if !dialog.Open() {
		t.Fatal("click required a draw between press and release")
	}
}

func TestFilterResetReturnsToTheFirstResult(t *testing.T) {
	var filter headless.Filter[string]
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
	var editor headless.Editor
	editor.SetText(strings.Repeat("a", 600))
	editor.SetCursor(0, 0)
	root := headless.NewRoot(&editor)
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
			slider := headless.NewSlider(headless.SliderConfig{Maximum: 100})
			track := &sliderTrack{control: slider, rect: image.Rect(0, 0, 11, 1)}
			root := headless.NewRoot(track)
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
				slider.Focus(false)
			}
			if slider.Handle(regionDrag(9, 0)) || slider.Handle(regionRelease(9, 0)) || slider.Value() != 50 {
				t.Fatal("invalidated gesture changed the slider")
			}
		})
	}
}

func TestPendingCloseDoesNotMoveToANewModal(t *testing.T) {
	g := input.Chord{Code: input.Character, Rune: 'g'}
	keys := &keymap.Map{}
	keys.Bind(headless.Close, g, g)
	stack := &headless.Stack{Keys: keys}
	stack.Push(&panel{place: middle(8, 3)})
	paintWidget(12, 5, stack)
	stack.Handle(input.Key{Code: input.Character, Rune: 'g'})
	stack.Push(&panel{place: middle(8, 3)})
	paintWidget(12, 5, stack)
	stack.Handle(input.Key{Code: input.Character, Rune: 'g'})
	if stack.Depth() != 2 {
		t.Fatal("a new modal inherited a close prefix")
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
		keys.Bind(headless.Increase, g)
		keys.Bind(headless.Decrease, g, g)
		changes := 0
		settings := &headless.Settings[string]{
			EditKeys: keys,
			Change:   func(int, string, keymap.Action) bool { changes++; return true },
		}
		settings.SetItems([]string{"one", "two"})
		settings.Handle(input.Key{Code: input.Character, Rune: 'g'})
		if replace {
			settings.SetItems([]string{"new", "items"})
		} else {
			settings.Select(1)
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

func TestRemovedModalDoesNotHandItsGestureToTheBase(t *testing.T) {
	base := &pointerTarget{takes: true}
	stack := headless.NewStack(base)
	modal := &panel{place: middle(8, 3), takesMouse: true}
	id := stack.Push(modal)
	root := headless.NewRoot(stack)
	surface := grid.NewSurface(12, 5)
	root.Draw(surface.View())
	area, _ := stack.Area()
	stack.Handle(regionPress(area.Min.X, area.Min.Y))
	stack.Remove(id)
	root.Draw(surface.View())
	stack.Handle(regionDrag(1, 1))
	stack.Handle(regionRelease(1, 1))
	if len(base.events) != 0 {
		t.Fatal("base inherited a removed modal's gesture")
	}
	if !stack.Handle(regionPress(1, 1)) {
		t.Fatal("base did not receive a new gesture")
	}
}

func TestListBoundaryNavigationRevealsAnUnmovedCursor(t *testing.T) {
	var list headless.List[int]
	list.SetItems(make([]int, 30))
	paintWidget(12, 3, &list)
	list.Scroll().By(5)
	paintWidget(12, 3, &list)
	list.Do(headless.SelectFirst)
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
	keys.Bind(headless.Accept, g)
	keys.Bind(headless.SelectNext, g, g)
	accepted := false
	completion := &headless.Completion{Keys: keys, Accept: func(headless.Candidate, headless.Token) { accepted = true }}
	completion.Offer(headless.Token{Query: "old"}, []headless.Candidate{{Text: "old candidate"}})
	completion.Handle(input.Key{Code: input.Character, Rune: 'g'})
	completion.Offer(headless.Token{Query: "new"}, []headless.Candidate{{Text: "new candidate"}})
	resolve()
	if accepted || !completion.Open() {
		t.Fatal("a new query inherited pending acceptance")
	}
}

func TestStackBaseCaptureSurvivesCoveringButNotReplacement(t *testing.T) {
	base, replacement := &pointerTarget{takes: true}, &pointerTarget{takes: true}
	stack := headless.NewStack(base)
	root := headless.NewRoot(stack)
	surface := grid.NewSurface(12, 5)
	root.Draw(surface.View())
	stack.Handle(regionPress(1, 1))
	modal := &panel{place: middle(8, 3), takesMouse: true}
	stack.Push(modal)
	root.Draw(surface.View())
	stack.Handle(regionDrag(3, 2))
	if len(base.events) != 2 || len(modal.seen) != 0 {
		t.Fatal("covering modal inherited a gesture that began in the base")
	}
	stack.SetBase(replacement)
	root.Draw(surface.View())
	stack.Handle(regionRelease(3, 2))
	if len(replacement.events) != 0 || len(modal.seen) != 0 {
		t.Fatal("replacement base or covering modal inherited the old release")
	}
}
