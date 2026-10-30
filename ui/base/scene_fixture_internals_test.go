package base

import (
	"reflect"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type (
	testPaint interface{ Draw(frame scene.Frame) }
	testScene struct {
		root     *scene.Root
		sequence uint64
	}
)

var (
	testElements = map[any]scene.Element{}
	testRoots    = map[scene.Element]*testScene{}
)

// Paint assertions run through actual scene prepare/admission/write
// boundaries. This fixture is test code, not a second application API.
func testElement(value any) scene.Element {
	if value == nil {
		return scene.Element{}
	}
	if element, ok := value.(scene.Element); ok {
		return element
	}
	if provider, ok := value.(interface{ Element() scene.Element }); ok {
		return provider.Element()
	}
	cacheable := reflect.TypeOf(value).Kind() == reflect.Pointer && reflect.TypeOf(value).Comparable()
	if cacheable {
		if element, ok := testElements[value]; ok {
			return element
		}
	}
	config := scene.ElementConfig{}
	if paint, ok := value.(testPaint); ok {
		config.Paint = paint.Draw
	} else if block, ok := value.(grid.Drawable); ok {
		config.Paint = func(frame scene.Frame) { block.Draw(frame.View) }
	} else {
		panic("test: value supplies neither Element nor Paint")
	}
	if focused, ok := value.(interface{ Focus(has bool) }); ok {
		config.FocusChanged = focused.Focus
	}
	if handler, ok := value.(interface{ Handle(event input.Event) bool }); ok {
		config.Handle = func(event scene.Event) scene.EventResult {
			accepted := handler.Handle(event.Input)
			result := scene.EventResult{Consumed: accepted}
			if mouse, ok := event.Input.(input.Mouse); ok && accepted && mouse.Action == input.MouseDown {
				result.Focus = true
				result.Capture = true
			}
			return result
		}
		config.Focusable = func() bool { return true }
	}
	if measure, ok := value.(interface{ HeightForWidth(width int) int }); ok {
		config.Measure = func(axis layout.Axis, width int) int {
			if axis == layout.Down {
				return measure.HeightForWidth(width)
			}
			return 0
		}
	}
	element := scene.NewElement(config)
	if cacheable {
		testElements[value] = element
	}
	return element
}

func newTestRoot(value any) *testScene {
	element := testElement(value)
	if known := testRoots[element]; known != nil && element.Alive() {
		return known
	}
	if element.Alive() {
		for _, known := range testRoots {
			if _, err := known.root.Observe(scene.Query{Root: element.Target()}); err == nil {
				testRoots[element] = known
				return known
			}
		}
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		panic(err)
	}
	fixture := &testScene{root: root}
	testRoots[element] = fixture
	root.FlushNotifications()
	return fixture
}

func (r *testScene) Draw(view grid.View) {
	r.root.FlushNotifications()
	prepared, err := r.root.Prepare(view)
	if err != nil {
		panic(err)
	}
	r.sequence++
	prepared.Commit(r.sequence)
	r.root.Wrote(r.sequence)
}
func chosenValue[T any](field *Select[T]) T { chosen, _ := field.Chosen(); return chosen.Value }
