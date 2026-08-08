package headless

import (
	"image"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// Dialog is the behavior and semantic owner of one modal interaction.
//
// Its [DialogContent] is a compound part that adapts an appearance-supplied Modal to
// the owning [Stack]. Show, Dismiss and Sync are the only open-state transitions; they
// keep caller-owned state, stack membership and focus restoration in one state machine.
// Dialog chooses no border, colour, placement or product wording beyond the semantic
// title supplied by its caller.
type Dialog struct {
	stack   *Stack
	content *DialogContent
	open    ownedValue[bool]
	title   string
	detail  string
	layer   LayerID
}

// NewDialog constructs an uncontrolled dialog whose open state is locally owned.
func NewDialog(stack *Stack, title string, content Modal) *Dialog {
	return newDialog(stack, title, content, localValue(false))
}

// NewControlledDialog constructs a dialog whose open state lives in binding.
//
// Controller operations write binding directly; there is no private copy to drift.
// When the owner writes the binding itself, it calls [Dialog.Sync] to perform the
// corresponding stack and focus transition.
func NewControlledDialog(stack *Stack, binding Accessor[bool], title string, content Modal) *Dialog {
	return newDialog(stack, title, content, controlledValue(binding))
}

func newDialog(stack *Stack, title string, content Modal, open ownedValue[bool]) *Dialog {
	if stack == nil {
		panic("headless: dialog requires a stack")
	}
	if content == nil {
		panic("headless: dialog requires content")
	}
	dialog := &Dialog{stack: stack, open: open, title: strings.Clone(title)}
	dialog.content = &DialogContent{dialog: dialog, modal: content}
	dialog.Sync()
	return dialog
}

// Show opens the dialog and gives its content the stack's keyboard ownership.
func (d *Dialog) Show() {
	if d == nil {
		return
	}
	d.open.set(true)
	d.Sync()
}

// Dismiss closes the dialog and restores focus to the layer or base beneath it.
func (d *Dialog) Dismiss() {
	if d == nil {
		return
	}
	d.open.set(false)
	d.Sync()
}

// Sync applies caller-written controlled state to stack membership and focus.
//
// Accessors are deliberately not observable. Making this transition explicit keeps
// focus changes out of Draw and preserves the rule that drawing cannot advance
// semantic state.
func (d *Dialog) Sync() {
	if d == nil || d.stack == nil || d.content == nil {
		return
	}
	shown := d.stack.Contains(d.layer)
	switch {
	case d.open.get() && !shown:
		d.layer = d.stack.Push(d.content)
	case !d.open.get() && shown:
		d.stack.Remove(d.layer)
	}
}

// Open reports whether the dialog is semantically open.
func (d *Dialog) Open() bool { return d != nil && d.open.get() }

// Content returns the modal compound part owned by the dialog.
func (d *Dialog) Content() *DialogContent {
	if d == nil {
		return nil
	}
	return d.content
}

// Title returns the dialog's semantic title.
func (d *Dialog) Title() string {
	if d == nil {
		return ""
	}
	return d.title
}

// SetTitle changes the dialog's semantic title.
func (d *Dialog) SetTitle(title string) {
	if d != nil {
		d.title = strings.Clone(title)
	}
}

// Description returns the dialog's semantic description.
func (d *Dialog) Description() string {
	if d == nil {
		return ""
	}
	return d.detail
}

// SetDescription changes the dialog's semantic description.
func (d *Dialog) SetDescription(description string) {
	if d != nil {
		d.detail = strings.Clone(description)
	}
}

// Trigger constructs an activation part for this dialog around appearance of.
func (d *Dialog) Trigger(label string, of Widget) *DialogTrigger {
	return &DialogTrigger{dialog: d, label: label, Of: of}
}

// Semantics returns the dialog independently of its visual boxes.
func (d *Dialog) Semantics() SemanticNode {
	if d == nil {
		return SemanticNode{Role: RoleDialog}
	}
	state := SemanticState(0)
	if d.Open() {
		state |= StateOpen
	}
	if d.content != nil && d.content.focused {
		state |= StateFocused
	}
	return SemanticNode{
		Role: RoleDialog, Label: d.title, Description: d.detail, State: state,
	}
}

func (d *Dialog) closed(content *DialogContent) {
	if d == nil || d.content != content {
		return
	}
	d.open.set(false)
	d.layer = 0
}

// DialogContent is the modal compound part of a [Dialog].
//
// It delegates drawing, placement and input to an appearance-supplied Modal while
// keeping closure and focus in the controller. Callers receive it from
// [Dialog.Content]; constructing one directly would leave it without an owner.
type DialogContent struct {
	dialog  *Dialog
	modal   Modal
	focused bool
}

// Draw delegates to the appearance-supplied content.
func (c *DialogContent) Draw(frame Frame) {
	if c != nil && c.modal != nil {
		c.modal.Draw(frame)
	}
}

// Handle delegates input to the appearance-supplied content.
func (c *DialogContent) Handle(event input.Event) bool {
	return c != nil && c.modal != nil && c.modal.Handle(event)
}

// Place delegates placement to the appearance-supplied content.
func (c *DialogContent) Place(space image.Point) layout.Placement {
	if c == nil || c.modal == nil {
		return layout.Placement{}
	}
	return c.modal.Place(space)
}

// Backdrop delegates the optional backdrop part without making it behavior.
func (c *DialogContent) Backdrop(view grid.View) {
	if c == nil || c.modal == nil {
		return
	}
	if backdrop, ok := c.modal.(Backdrop); ok {
		backdrop.Backdrop(view)
	}
}

// Insists delegates whether the content currently requires an answer.
func (c *DialogContent) Insists() bool {
	if c == nil || c.modal == nil {
		return false
	}
	insistent, ok := c.modal.(Insistent)
	return ok && insistent.Insists()
}

// Focus records semantic focus and delegates it to the appearance content.
func (c *DialogContent) Focus(has bool) {
	if c == nil {
		return
	}
	c.focused = has
	tell(c.modal, has)
}

// Closed settles controller state when any stack path removes the content.
func (c *DialogContent) Closed() {
	if c == nil {
		return
	}
	c.focused = false
	if c.dialog != nil {
		c.dialog.closed(c)
	}
	if closer, ok := c.modal.(Closer); ok {
		closer.Closed()
	}
}

// Semantics delegates to the owning controller.
func (c *DialogContent) Semantics() SemanticNode {
	if c == nil || c.dialog == nil {
		return SemanticNode{Role: RoleDialog}
	}
	return c.dialog.Semantics()
}

// DialogTrigger is the activation compound part of a [Dialog].
//
// Of supplies appearance only. The trigger owns activation, pointer capture, focus and
// semantic state, so a different appearance cannot accidentally change behavior.
type DialogTrigger struct {
	// Of is the trigger's appearance. It may measure and receive focus, but activation
	// belongs to the trigger itself.
	Of Widget
	// Keys maps activation. Nil reads through [DefaultActivationKeys].
	Keys *keymap.Map

	dialog       *Dialog
	label        string
	focused      bool
	pointer      Pointer
	presentation Snapshot[image.Rectangle]
	pending      keymap.Pending
}

// Draw paints the appearance and claims a pending press over its committed box.
func (t *DialogTrigger) Draw(frame Frame) {
	if t == nil {
		return
	}
	area := frame.Bounds()
	t.presentation.Stage(frame, area)
	t.pointer.Claim(area)
	if t.Of != nil {
		t.Of.Draw(frame)
	}
}

// Measure delegates to a measured appearance.
func (t *DialogTrigger) Measure(across int) int {
	if t == nil {
		return 0
	}
	measurer, ok := t.Of.(layout.Measurer)
	if !ok {
		return 0
	}
	return measurer.Measure(across)
}

// Handle opens the dialog on an activation key or completed primary click.
func (t *DialogTrigger) Handle(event input.Event) bool {
	if t == nil {
		return false
	}
	if _, ok := event.(input.Mouse); ok {
		if t.presentation.Value().Empty() {
			return false
		}
		t.pointer.Handle(event)
		if t.pointer.Clicked(t.presentation.Value(), input.ButtonLeft) {
			t.dialog.Show()
		}
		return true
	}
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	action, mine := t.keys().Lookup(key, &t.pending)
	if !mine || action == "" {
		return mine
	}
	return t.Do(action)
}

// Do runs the activation action by name.
func (t *DialogTrigger) Do(action keymap.Action) bool {
	if t == nil || action != Activate {
		return false
	}
	t.dialog.Show()
	return true
}

// Focus records semantic focus and passes it to the appearance.
func (t *DialogTrigger) Focus(has bool) {
	if t == nil {
		return
	}
	t.focused = has
	tell(t.Of, has)
}

// Semantics returns the trigger as a button associated with the dialog's open state.
func (t *DialogTrigger) Semantics() SemanticNode {
	state := SemanticState(0)
	if t != nil && t.focused {
		state |= StateFocused
	}
	if t != nil && t.dialog != nil && t.dialog.Open() {
		state |= StateOpen
	}
	if t == nil {
		return SemanticNode{Role: RoleButton}
	}
	return SemanticNode{Role: RoleButton, Label: t.label, State: state}
}

func (t *DialogTrigger) keys() *keymap.Map {
	if t != nil && t.Keys != nil {
		return t.Keys
	}
	return activationKeys()
}
