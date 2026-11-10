// Package scenetest confines adapter identity and scene lifetimes so tests and
// fuzz samples cannot retain earlier scenes.
package scenetest

import (
	"errors"
	"reflect"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Scope bounds adapter identity and scene lifetime to one test or fuzz sample.
// It must not be copied or used concurrently. Close it before the next sample.
type Scope struct {
	noCopy   noCopy
	elements map[any]scene.Element
	roots    []*Scene
	closed   bool
}

func New() *Scope {
	return &Scope{
		elements: make(map[any]scene.Element),
	}
}

func (s *Scope) Close() {
	if s.closed {
		return
	}
	s.closed = true
	roots := s.roots
	s.elements, s.roots = nil, nil
	for _, root := range roots {
		defer root.Close()
	}
}

func (s *Scope) requireOpen() {
	if s == nil || s.closed {
		panic("scenetest: closed scope")
	}
}

func (s *Scope) Element(value any) scene.Element {
	s.requireOpen()
	if value == nil {
		return scene.Element{}
	}
	if element, ok := value.(scene.Element); ok {
		return element
	}
	if provider, ok := value.(interface{ Element() scene.Element }); ok {
		return provider.Element()
	}
	cacheable := reflect.TypeOf(value).Kind() == reflect.Pointer
	if cacheable {
		if element, ok := s.elements[value]; ok {
			return element
		}
	}
	element := wrap(value)
	if cacheable {
		s.elements[value] = element
	}
	return element
}

func wrap(value any) scene.Element {
	config := scene.ElementConfig{}
	if paint, ok := value.(interface{ Draw(frame scene.Frame) }); ok {
		config.Paint = paint.Draw
	} else if block, ok := value.(grid.Drawable); ok {
		config.Paint = func(frame scene.Frame) { block.Draw(frame.View) }
	} else {
		panic("scenetest: value supplies neither Element nor Draw")
	}
	if focused, ok := value.(interface{ Focus(has bool) }); ok {
		config.FocusChanged = focused.Focus
	}
	if handler, ok := value.(interface{ Handle(event input.Event) bool }); ok {
		config.Handle = func(event scene.Event) scene.EventResult {
			accepted := handler.Handle(event.Input())
			result := scene.EventResult{Consumed: accepted}
			if mouse, ok := event.Input().(input.Mouse); ok && accepted && mouse.Action == input.MouseDown {
				result.Focus = true
				result.Capture = true
			}
			return result
		}
		config.Focusable = func() bool { return true }
	}
	if measure, ok := value.(interface{ HeightForWidth(width int) int }); ok {
		config.Measure = scene.MeasureRows(measure.HeightForWidth)
	}
	return scene.NewElement(config)
}

func (s *Scope) Mount(value any) *Scene {
	element := s.Element(value)
	target := element.Target()
	for _, known := range s.roots {
		if known.Valid(target) {
			return known
		}
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		panic(err)
	}
	fixture := &Scene{Root: root, scope: s}
	s.roots = append(s.roots, fixture)
	root.FlushNotifications()
	return fixture
}

func (s *Scope) Focus(value any, has bool) {
	element := s.Element(value)
	if has {
		root := s.Mount(value)
		if err := element.RequestFocusFirst(); err != nil && !errors.Is(err, scene.ErrUnavailable) {
			panic(err)
		}
		root.FlushNotifications()
		return
	}
	if element.HasBeenAttached() && !element.Alive() {
		return
	}
	root := s.Mount(value)
	if element.Alive() {
		root.SetContent(scene.NewElement(scene.ElementConfig{}))
	}
}

// Scene exercises actual prepare, admission and write boundaries. Use a
// scene.Root without this fixture to separate admission from write completion.
// Scene must not be copied; its admission sequence belongs to one root.
type Scene struct {
	*scene.Root
	noCopy   noCopy
	scope    *Scope
	sequence uint64
}

func (r *Scene) Draw(view grid.View) {
	r.scope.requireOpen()
	r.FlushNotifications()
	prepared, err := r.Prepare(view)
	if err != nil {
		panic(err)
	}
	r.sequence++
	prepared.Commit(r.sequence)
	r.Wrote(r.sequence)
}

func (r *Scene) SetContent(value any) {
	if err := r.Root.SetContent(r.scope.Element(value)); err != nil {
		panic(err)
	}
	r.FlushNotifications()
}

func (r *Scene) Close() {
	root := r.Root
	r.Root, r.scope = nil, nil
	root.Close()
}
