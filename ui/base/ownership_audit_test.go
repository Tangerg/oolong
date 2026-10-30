package base_test

import (
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestChoiceFieldsEndTheListsAcceptedGesture(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		for _, end := range []string{"outside release", "label press", "blur"} {
			t.Run(end+"/"+map[bool]string{false: "select", true: "multi"}[multiple], func(t *testing.T) {
				one := &base.Select[string]{Label: "Choice", Same: base.Equal[string]}
				many := &base.MultiSelect[string]{Label: "Choice", Same: base.Equal[string]}
				one.SetOptions(base.Options("a", "b", "c"))
				many.SetOptions(base.Options("a", "b", "c"))
				var field base.Field = one
				if multiple {
					field = many
				}
				root := newTestRoot(field)
				root.Draw(grid.NewSurface(8, 4).View())
				testFocus(field, true)
				if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 1)}) {
					t.Fatal("primary press refused")
				}
				switch end {
				case "outside release":
					root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(1, 8)})
				case "label press":
					root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 0)})
				case "blur":
					testFocus(field, false)
				}
				if root.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(1, 3)}) && end != "blur" {
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
	for name, field := range map[string]base.Field{
		"text":    &base.Text{Label: "Label"},
		"confirm": &base.Confirm{Label: "Label"},
		"select":  selectWith(base.Options("a", "b")),
		"multi":   multiWith(base.Options("a", "b")),
	} {
		t.Run(name, func(t *testing.T) {
			view := grid.NewSurface(3, 1).View().Sub(image.Rect(-2, -1, 8, 4))
			root := newTestRoot(field)
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
			var pointer base.Pointer
			stagePointer(&pointer, image.Rect(0, 0, 8, 1))
			slider := base.NewSlider(base.SliderConfig{Maximum: 7})
			newTestRoot(sliderTrack{control: slider, rect: image.Rect(0, 0, 8, 1)}).Draw(grid.NewSurface(8, 1).View())
			editor := &base.Editor{}
			editor.SetText("abcdefg")
			newTestRoot(editor).Draw(grid.NewSurface(8, 1).View())
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

func TestChoiceFieldsDoNotScrollThroughTheirLabels(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		one := &base.Select[string]{Label: "Choice", Same: base.Equal[string]}
		many := &base.MultiSelect[string]{Label: "Choice", Same: base.Equal[string]}
		one.SetOptions(base.Options("a", "b", "c"))
		many.SetOptions(base.Options("a", "b", "c"))
		var field base.Field = one
		if multiple {
			field = many
		}
		root := newTestRoot(field)
		root.Draw(grid.NewSurface(8, 2).View())
		if root.Handle(input.Mouse{Action: input.WheelDown, Pos: image.Pt(0, 0)}) {
			t.Fatal("label wheel was treated as a list wheel")
		}
		if !root.Handle(input.Mouse{Action: input.WheelDown, Pos: image.Pt(0, 1)}) {
			t.Fatal("visible list wheel was refused")
		}
	}
}
