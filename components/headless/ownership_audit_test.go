package headless_test

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/keymap"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

func TestChoiceFieldsEndTheListsAcceptedGesture(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		for _, end := range []string{"outside release", "label press", "blur"} {
			t.Run(end+"/"+map[bool]string{false: "select", true: "multi"}[multiple], func(t *testing.T) {
				one := &headless.Select[string]{Label: "Choice", Same: headless.Equal[string]}
				many := &headless.MultiSelect[string]{Label: "Choice", Same: headless.Equal[string]}
				one.SetOptions(headless.Options("a", "b", "c"))
				many.SetOptions(headless.Options("a", "b", "c"))
				var field headless.Field = one
				if multiple {
					field = many
				}
				root := headless.NewRoot(field)
				root.Draw(grid.NewSurface(8, 4).View())
				field.Focus(true)
				if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}) {
					t.Fatal("primary press refused")
				}
				switch end {
				case "outside release":
					root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(1, 8)})
				case "label press":
					root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 0)})
				case "blur":
					field.Focus(false)
				}
				if root.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(1, 3)}) {
					t.Fatal("ended gesture continued selecting")
				}
				if multiple {
					many.Toggle()
					if !slices.Equal(many.Taken(), []string{"a"}) {
						t.Fatalf("orphan drag moved the cursor: %v", many.Taken())
					}
				} else if chosen, _ := one.Chosen(); chosen.Value != "a" {
					t.Fatalf("orphan drag selected %q", chosen.Value)
				}
			})
		}
	}
}

func TestClippedFieldsRejectInvisiblePresses(t *testing.T) {
	for name, field := range map[string]headless.Field{
		"text":    &headless.Text{Label: "Label"},
		"confirm": &headless.Confirm{Label: "Label"},
		"select":  selectWith(headless.Options("a", "b")),
		"multi":   multiWith(headless.Options("a", "b")),
	} {
		t.Run(name, func(t *testing.T) {
			view := grid.NewSurface(3, 1).View().Sub(image.Rect(-2, -1, 8, 4))
			root := headless.NewRoot(field)
			root.Draw(view)
			if root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 1)}) {
				t.Fatal("clipped part of the field accepted a press")
			}
		})
	}
}

func TestPrimaryControlsRejectAnotherButtonsDrag(t *testing.T) {
	for _, name := range []string{"pointer", "slider", "editor"} {
		t.Run(name, func(t *testing.T) {
			var pointer headless.Pointer
			stagePointer(&pointer, image.Rect(0, 0, 8, 1))
			slider := headless.NewSlider(headless.SliderConfig{Maximum: 7})
			headless.NewRoot(sliderTrack{control: slider, rect: image.Rect(0, 0, 8, 1)}).Draw(grid.NewSurface(8, 1).View())
			editor := &headless.Editor{}
			editor.SetText("abcdefg")
			headless.NewRoot(editor).Draw(grid.NewSurface(8, 1).View())
			var handle func(input.Event) bool
			switch name {
			case "pointer":
				handle = pointer.Handle
			case "slider":
				handle = slider.Handle
			case "editor":
				handle = editor.Handle
			}
			if !handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 0)}) {
				t.Fatal("primary press refused")
			}
			if handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonRight, Pos: image.Pt(6, 0)}) {
				t.Fatal("another button's drag was accepted")
			}
			if !handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonNone, Pos: image.Pt(4, 0)}) {
				t.Fatal("foreign drag canceled the accepted capture")
			}
		})
	}
}

func TestTabsSharingAPaneKeepOneFocusHolder(t *testing.T) {
	pane := &reentrant{name: "shared"}
	tabs := headless.NewTabs(headless.TabsConfig{Items: []headless.Tab{{Title: "a", Of: pane}, {Title: "b", Of: pane}}})
	tabs.Select(1)
	if !pane.focused || !slices.Equal(pane.changes, []bool{true}) {
		t.Fatalf("one pane received competing focus transitions: %v", pane.changes)
	}
}

type sharedModal struct{ *reentrant }

func (*sharedModal) Place(image.Point) layout.Placement { return middle(8, 1) }

func TestSharedAttachmentsKeepOneFocusHolder(t *testing.T) {
	for _, owner := range []string{"container", "stack", "covered base"} {
		t.Run(owner, func(t *testing.T) {
			pane := &sharedModal{&reentrant{name: "shared"}}
			switch owner {
			case "container":
				c := headless.NewContainer(layout.Down, headless.Item{Of: pane}, headless.Item{Of: pane})
				c.FocusIndex(1)
			case "stack":
				var s headless.Stack
				s.Push(pane)
				s.Push(pane)
				s.Pop()
			case "covered base":
				var s headless.Stack
				s.Push(pane)
				s.SetBase(pane)
			}
			if !pane.focused || !slices.Equal(pane.changes, []bool{true}) {
				t.Fatalf("one pane received competing focus transitions: %v", pane.changes)
			}
		})
	}
}

func TestChoiceFieldsDoNotScrollThroughTheirLabels(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		one := &headless.Select[string]{Label: "Choice", Same: headless.Equal[string]}
		many := &headless.MultiSelect[string]{Label: "Choice", Same: headless.Equal[string]}
		one.SetOptions(headless.Options("a", "b", "c"))
		many.SetOptions(headless.Options("a", "b", "c"))
		var field headless.Field = one
		if multiple {
			field = many
		}
		root := headless.NewRoot(field)
		root.Draw(grid.NewSurface(8, 2).View())
		if root.Handle(input.Mouse{Action: input.WheelDown, Pos: image.Pt(0, 0)}) {
			t.Fatal("label wheel was treated as a list wheel")
		}
		if !root.Handle(input.Mouse{Action: input.WheelDown, Pos: image.Pt(0, 1)}) {
			t.Fatal("visible list wheel was refused")
		}
	}
}

func TestStackMatcherCancellationCannotReviveCoveredPane(t *testing.T) {
	a := &sharedModal{&reentrant{name: "a"}}
	b := &sharedModal{&reentrant{name: "b"}}
	c := &sharedModal{&reentrant{name: "c"}}
	var s headless.Stack
	var keys keymap.Map
	keys.Bind(headless.Close, input.Chord{Rune: 'g'})
	keys.Bind(headless.Close, input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	keys.Resolve = func(time.Duration, func()) func() { return func() { s.Push(c) } }
	s.Keys = &keys
	s.Push(a)
	headless.NewRoot(&s).Draw(grid.NewSurface(10, 2).View())
	s.Handle(input.Key{Rune: 'g'})
	s.Push(b)
	if s.Top() != c || !c.focused || b.focused {
		t.Fatalf("cancellation revived old focus: a=%v b=%v c=%v", a.focused, b.focused, c.focused)
	}
}

func TestStackPresentationCannotDispatchKeysForAnotherFocusOwner(t *testing.T) {
	for _, replacement := range []string{"base", "value base", "layer"} {
		t.Run(replacement, func(t *testing.T) {
			a, b := &panel{name: "a"}, &panel{name: "b"}
			s := headless.NewStack(a)
			switch replacement {
			case "value base":
				s.SetBase(valuePanel{target: a, payload: []byte("a")})
			case "layer":
				s.Push(a)
			}
			draw(s, 10, 2)
			switch replacement {
			case "value base":
				s.SetBase(valuePanel{target: b, payload: []byte("b")})
			case "base":
				s.SetBase(b)
			case "layer":
				s.Push(b)
			}
			s.Handle(input.Key{Rune: 'x'})
			if len(a.seen) != 0 || len(b.seen) != 0 {
				t.Fatalf("pending focus dispatched keys: old=%v new=%v", a.seen, b.seen)
			}
			draw(s, 10, 2)
			s.Handle(input.Key{Rune: 'x'})
			if len(a.seen) != 0 || len(b.seen) != 1 {
				t.Fatal("committed focus holder did not receive its key")
			}
		})
	}
}
