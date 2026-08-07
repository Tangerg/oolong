package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

type focusProbe struct {
	focused bool
	changes []bool
}

func (*focusProbe) Draw(headless.Frame) {}

func (*focusProbe) Handle(input.Event) bool { return false }

func (p *focusProbe) Focus(has bool) {
	p.focused = has
	p.changes = append(p.changes, has)
}

type focusedPanel struct {
	panel
	focusProbe
}

func (p *focusedPanel) Draw(frame headless.Frame) { p.panel.Draw(frame) }

func (p *focusedPanel) Handle(event input.Event) bool { return p.panel.Handle(event) }

func (p *focusedPanel) Focus(has bool) { p.focusProbe.Focus(has) }

func TestDialogOwnsOpenStateAndRestoresFocus(t *testing.T) {
	base := &focusProbe{}
	content := &focusedPanel{panel: panel{name: "confirm", place: middle(10, 3)}}
	stack := &headless.Stack{Base: base}
	stack.Focus(true)
	dialog := headless.NewDialog(stack, "Approve command", content)
	dialog.SetDescription("The command may change files")

	dialog.Show()
	if !dialog.Open() || stack.Top() != dialog.Content() {
		t.Fatal("show did not make the dialog content the open top layer")
	}
	if base.focused || !content.focused {
		t.Fatalf("focus while open: base=%v content=%v", base.focused, content.focused)
	}
	semantic := dialog.Semantics()
	if semantic.Role != headless.RoleDialog || semantic.Label != "Approve command" ||
		semantic.Description == "" || !semantic.State.Has(headless.StateOpen|headless.StateFocused) {
		t.Fatalf("dialog semantics = %+v", semantic)
	}

	dialog.Dismiss()
	if dialog.Open() || !stack.Empty() {
		t.Fatal("dismiss left the dialog open or in the stack")
	}
	if !base.focused || content.focused {
		t.Fatalf("focus after dismiss: base=%v %v content=%v %v",
			base.focused, base.changes, content.focused, content.changes)
	}
	if content.closed != 1 {
		t.Fatalf("appearance content closed %d times, want once", content.closed)
	}
}

func TestDialogSettlesEveryStackDismissalIntoControlledState(t *testing.T) {
	open := false
	stack := &headless.Stack{}
	dialog := headless.NewControlledDialog(
		stack, headless.Bind(&open), "Confirm", &panel{place: middle(8, 3)},
	)
	dialog.Show()
	if !open {
		t.Fatal("show did not write caller-owned state")
	}
	stack.Pop()
	if open || dialog.Open() {
		t.Fatal("a stack dismissal did not settle caller-owned state")
	}

	open = true
	dialog.Sync()
	if stack.Top() != dialog.Content() {
		t.Fatal("sync did not apply caller-written open state")
	}
	open = false
	dialog.Sync()
	if !stack.Empty() {
		t.Fatal("sync did not apply caller-written closed state")
	}
}

func TestDialogCanCloseUnderANewerLayerWithoutDismissingIt(t *testing.T) {
	stack := &headless.Stack{}
	dialog := headless.NewDialog(stack, "First", &panel{place: middle(8, 3)})
	dialog.Show()
	newer := &panel{name: "newer", place: middle(6, 2)}
	stack.Push(newer)

	dialog.Dismiss()
	if stack.Depth() != 1 || stack.Top() != newer {
		t.Fatal("closing the covered dialog dismissed or displaced the newer layer")
	}
}

func TestDialogTriggerOwnsActivationAndSemantics(t *testing.T) {
	stack := &headless.Stack{}
	dialog := headless.NewDialog(stack, "Confirm", &panel{place: middle(8, 3)})
	trigger := dialog.Trigger("Open confirmation", &focusProbe{})
	root := headless.NewRoot(trigger)
	root.Draw(grid.NewSurface(12, 1).View())
	trigger.Focus(true)

	if !trigger.Handle(input.Key{Code: input.Enter}) || !dialog.Open() {
		t.Fatal("the default activation key did not open the dialog")
	}
	semantic := trigger.Semantics()
	if semantic.Role != headless.RoleButton || semantic.Label != "Open confirmation" ||
		!semantic.State.Has(headless.StateFocused|headless.StateOpen) {
		t.Fatalf("trigger semantics = %+v", semantic)
	}
}

func TestDialogTriggerDeclinesAPointerBeforeItHasAFrame(t *testing.T) {
	stack := &headless.Stack{}
	dialog := headless.NewDialog(stack, "Confirm", &panel{place: middle(8, 3)})
	trigger := dialog.Trigger("Open", nil)
	if trigger.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("a trigger answered a pointer about no presented frame")
	}
}
