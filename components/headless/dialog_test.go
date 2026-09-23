package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type focusProbe struct {
	focused bool
	changes []bool
	changed func(bool)
}

func (*focusProbe) Draw(headless.Frame) {}

func (*focusProbe) Handle(input.Event) bool { return false }

func (p *focusProbe) Focus(has bool) {
	p.focused = has
	p.changes = append(p.changes, has)
	if p.changed != nil {
		p.changed(has)
	}
}

func TestDialogReopenedDuringClosureKeepsItsNewMembership(t *testing.T) {
	for _, phase := range []string{"base focus", "content blur", "closed"} {
		t.Run(phase, func(t *testing.T) {
			base := &focusProbe{}
			content := &reopeningPanel{}
			stack := headless.NewStack(base)
			dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Content: content})
			dialog.Show()
			reopen := func() {
				base.changed, content.changed, content.onClose = nil, nil, nil
				dialog.Show()
			}
			switch phase {
			case "base focus":
				base.changed = func(has bool) {
					if has {
						reopen()
					}
				}
			case "content blur":
				content.changed = func(has bool) {
					if !has {
						reopen()
					}
				}
			case "closed":
				content.onClose = func() {
					if dialog.Semantics().State.Has(headless.StateFocused) != content.focused {
						t.Fatal("close notification advanced focus before the focus owner")
					}
					reopen()
				}
			}
			dialog.Dismiss()
			if !dialog.Open() || stack.Depth() != 1 || base.focused || !content.focused ||
				!dialog.Semantics().State.Has(headless.StateFocused) || content.closed != 1 {
				t.Fatalf("reopened: open=%v depth=%d base=%v content=%v semantics=%v closed=%d",
					dialog.Open(), stack.Depth(), base.focused, content.focused, dialog.Semantics(), content.closed)
			}
			if dialog.Sync() || stack.Depth() != 1 {
				t.Fatal("Sync duplicated the reopened insertion")
			}
			dialog.Dismiss()
			if dialog.Open() || stack.Depth() != 0 || !base.focused || content.focused || content.closed != 2 {
				t.Fatal("final dismissal left a live insertion or incorrect focus/close count")
			}
		})
	}
}

type reopeningPanel struct {
	focusedPanel
	onClose func()
}

func (p *reopeningPanel) Closed() {
	p.closed++
	if p.onClose != nil {
		p.onClose()
	}
}

func TestDialogCanDismissFromItsInitialFocusNotification(t *testing.T) {
	base := &focusProbe{}
	content := &focusedPanel{}
	stack := headless.NewStack(base)
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Content: content})
	content.changed = func(has bool) {
		if has {
			dialog.Dismiss()
		}
	}
	dialog.Show()
	if dialog.Open() || stack.Depth() != 0 || !base.focused || content.focused || content.closed != 1 || dialog.Sync() {
		t.Fatal("focus callback could not close the current insertion")
	}
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
	content := &focusedPanel{name: "confirm", place: middle(10, 3)}
	stack := headless.NewStack(base)
	stack.Focus(true)
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Title: "Approve command", Content: content})
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
	if dialog.Open() || stack.Depth() != 0 {
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

func TestAStackDoesNotTransferOwnershipToTheSameBase(t *testing.T) {
	base := &focusProbe{}
	stack := headless.NewStack(base)
	stack.SetBase(base)
	stack.Focus(true)
	if len(base.changes) != 1 || !base.focused {
		t.Fatalf("same base received focus transitions %v, want only construction", base.changes)
	}
	stack.Focus(false)
	stack.Focus(false)
	if len(base.changes) != 2 || base.focused {
		t.Fatalf("one real focus loss produced transitions %v", base.changes)
	}
}

func TestReplacingAStackBaseDoesNotReassignTheOpenLayer(t *testing.T) {
	first, second, modal := &focusProbe{}, &focusProbe{}, &focusedPanel{}
	stack := headless.NewStack(first)
	stack.Push(modal)
	want := len(modal.changes)
	stack.SetBase(second)
	if len(modal.changes) != want || !modal.focused {
		t.Fatalf("unchanged top layer received focus transitions %v", modal.changes)
	}
	if second.focused || len(second.changes) != 1 {
		t.Fatalf("new covered base received transitions %v, want one loss", second.changes)
	}
}

func TestABaseInstalledWhileBlurredBecomesTheSettledOwner(t *testing.T) {
	base := &focusProbe{}
	stack := &headless.Stack{}
	stack.Focus(false)
	stack.SetBase(base)
	stack.Focus(true)
	if len(base.changes) != 2 || !base.focused || base.changes[0] || !base.changes[1] {
		t.Fatalf("base focus transitions = %v, want [false true]", base.changes)
	}
}

func TestDialogSettlesEveryStackDismissalIntoControlledState(t *testing.T) {
	open := false
	stack := &headless.Stack{}
	dialog := headless.NewDialog(headless.DialogConfig{
		Stack: stack, Open: headless.Bind(&open), Title: "Confirm", Content: &panel{place: middle(8, 3)},
	})
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
	if stack.Depth() != 0 {
		t.Fatal("sync did not apply caller-written closed state")
	}
}

func TestDialogCanCloseUnderANewerLayerWithoutDismissingIt(t *testing.T) {
	stack := &headless.Stack{}
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Title: "First", Content: &panel{place: middle(8, 3)}})
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
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Title: "Confirm", Content: &panel{place: middle(8, 3)}})
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

func TestDialogTriggerTransfersFocusWithItsAppearance(t *testing.T) {
	dialog := headless.NewDialog(headless.DialogConfig{Stack: &headless.Stack{}, Title: "Confirm", Content: &panel{place: middle(8, 3)}})
	first := &focusProbe{}
	second := &focusProbe{}
	trigger := dialog.Trigger("Open", first)
	if trigger.Appearance() != first || !first.focused {
		t.Fatal("a new trigger did not give its appearance the keyboard")
	}
	trigger.SetAppearance(first)
	if len(first.changes) != 1 {
		t.Fatalf("same appearance received focus transitions %v, want only construction", first.changes)
	}

	trigger.SetAppearance(second)
	if first.focused || !second.focused {
		t.Fatalf("focus after replacement: first=%v second=%v", first.focused, second.focused)
	}
	trigger.Focus(false)
	if second.focused {
		t.Fatal("the active appearance kept focus after the trigger lost it")
	}
}

func TestDialogTriggerDeclinesAPointerBeforeItHasAFrame(t *testing.T) {
	stack := &headless.Stack{}
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Title: "Confirm", Content: &panel{place: middle(8, 3)}})
	trigger := dialog.Trigger("Open", nil)
	if trigger.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("a trigger answered a pointer about no presented frame")
	}
}

func TestAFocusReportIsMadeOnceAndNotRepeated(t *testing.T) {
	// A widget with nothing above it assumes it has the keyboard, so an owner's
	// first report is never empty — it is the moment the widget stops assuming.
	// Every report after it that says the same thing is empty, and an appearance
	// that does something on gaining focus should not do it twice. Three owners
	// used to answer this question in three ways; a dialog's content answered it by
	// repeating itself.
	stack := &headless.Stack{}
	modal := &placedFocusProbe{}
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Title: "Confirm", Content: modal})
	content := dialog.Content()

	content.Focus(true)
	if len(modal.changes) != 1 || !modal.changes[0] {
		t.Fatalf("the first report reached the modal as %v, want one gain", modal.changes)
	}
	content.Focus(true)
	content.Focus(true)
	if len(modal.changes) != 1 {
		t.Fatalf("repeated reports reached the modal as %v, want only the first", modal.changes)
	}
	content.Focus(false)
	if len(modal.changes) != 2 || modal.changes[1] {
		t.Fatalf("losing the keyboard reached the modal as %v", modal.changes)
	}
}

func TestAViewportTellsItsContentWhereItStandsOnce(t *testing.T) {
	content := &sizedFocusProbe{}
	window := headless.NewViewport(content)
	if len(content.changes) != 1 || !content.changes[0] {
		t.Fatalf("a new window told its content %v, want one gain", content.changes)
	}
	window.Focus(true)
	if len(content.changes) != 1 {
		t.Fatalf("a repeated report told the content %v, want only the first", content.changes)
	}
	window.Focus(false)
	window.Focus(false)
	if len(content.changes) != 2 || content.changes[1] {
		t.Fatalf("losing the keyboard told the content %v", content.changes)
	}
}

type sizedFocusProbe struct{ focusProbe }

func (*sizedFocusProbe) HeightForWidth(int) int { return 1 }

type placedFocusProbe struct{ focusProbe }

func (*placedFocusProbe) Place(image.Point) layout.Placement { return middle(8, 3) }
