package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type focusEditor struct {
	headless.Editor
	onFocus func(bool)
}

func (e *focusEditor) Focus(has bool) {
	e.Editor.Focus(has)
	if e.onFocus != nil {
		e.onFocus(has)
	}
}

func TestStackFocusAfterCloseCallbacks(t *testing.T) {
	for _, path := range []string{"pop", "remove", "clear", "dialog pop", "dialog dismiss", "dialog clear"} {
		t.Run(path, func(t *testing.T) {
			base := &focusEditor{}
			stack := headless.NewStack(base)
			layer := &reopeningPanel{}
			var dialog *headless.Dialog
			var closeLayer func()
			switch path {
			case "pop", "remove", "clear":
				id := stack.Push(layer)
				closeLayer = func() {
					switch path {
					case "pop":
						stack.Pop()
					case "remove":
						stack.Remove(id)
					case "clear":
						stack.Clear()
					}
				}
			default:
				dialog = headless.NewDialog(headless.DialogConfig{Stack: stack, Content: layer})
				dialog.Show()
				closeLayer = func() {
					switch path {
					case "dialog pop":
						stack.Pop()
					case "dialog dismiss":
						dialog.Dismiss()
					case "dialog clear":
						stack.Clear()
					}
				}
			}
			duringClose, restored := false, false
			restore := func(has bool) {
				if !has && duringClose && !restored {
					restored = true
					stack.Focus(true)
				}
			}
			base.onFocus, layer.changed = restore, restore
			layer.onClose = func() {
				duringClose = true
				stack.Focus(false)
			}
			closeLayer()
			if !restored || stack.Depth() != 0 || layer.closed != 1 || layer.focused {
				t.Fatalf("close: restored=%v depth=%d closed=%d focused=%v", restored, stack.Depth(), layer.closed, layer.focused)
			}
			if dialog != nil && (dialog.Open() || dialog.Semantics().State.Has(headless.StateFocused)) {
				t.Fatal("closed dialog retained open or focused semantics")
			}
			assertEditorFocus(t, stack, true)
			stack.Focus(true)
			assertEditorFocus(t, stack, true)
			if !stack.Handle(input.Key{Code: input.Character, Rune: 'x'}) || base.Text() != "x" {
				t.Fatal("editor did not receive the routed key")
			}
		})
	}
}

func TestCompoundFocusReentry(t *testing.T) {
	for _, kind := range []string{"container", "tabs", "stack"} {
		t.Run(kind, func(t *testing.T) {
			for _, finalFocus := range []bool{false, true} {
				editor := &focusEditor{}
				var owner headless.Focusable
				switch kind {
				case "container":
					owner = headless.NewContainer(layout.Down, headless.Item{Of: editor, Size: layout.Flex(1)})
				case "tabs":
					owner = headless.NewTabs(headless.TabsConfig{Items: []headless.Tab{{Of: editor}}})
				case "stack":
					owner = headless.NewStack(editor)
				}
				owner.Focus(finalFocus)
				editor.onFocus = func(bool) {
					editor.onFocus = nil
					owner.Focus(finalFocus)
				}
				owner.Focus(!finalFocus)
				assertEditorFocus(t, owner, finalFocus)
				owner.Focus(finalFocus)
				assertEditorFocus(t, owner, finalFocus)
			}
		})
	}
}

func TestTabsFocusDuringControlledSelection(t *testing.T) {
	selected := 0
	old := &focusProbe{}
	editor := &headless.Editor{}
	tabs := headless.NewTabs(headless.TabsConfig{
		Items:     []headless.Tab{{Of: old}, {Of: editor}},
		Selection: headless.Bind(&selected),
	})
	old.changed = func(has bool) {
		if !has {
			old.changed = nil
			tabs.Focus(true)
		}
	}
	selected = 1
	tabs.Focus(false)
	assertEditorFocus(t, tabs, true)
	if old.focused || !tabs.Semantics().State.Has(headless.StateFocused) {
		t.Fatal("tab focus disagrees with the selected editor")
	}
}

func assertEditorFocus(t *testing.T, widget headless.Widget, focused bool) {
	t.Helper()
	screen := grid.NewScreen(20, 3)
	headless.NewRoot(widget).Draw(screen.Frame())
	if screen.Cursor().Visible != focused {
		t.Errorf("editor cursor visible=%v, want focused=%v", screen.Cursor().Visible, focused)
	}
}
