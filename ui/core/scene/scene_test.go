package scene_test

import (
	"context"
	"errors"
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func root(t *testing.T, element scene.Element) *scene.Root {
	t.Helper()
	r, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func draw(t *testing.T, r *scene.Root, width, height int, sequence uint64) {
	t.Helper()
	p, err := r.Prepare(grid.NewSurface(width, height).View())
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(sequence)
	if sequence == 0 {
		r.Wrote(r.Progress().WrittenSequence)
	}
}

func item(key string, element scene.Element) scene.Item {
	return scene.Item{Key: key, Size: layout.Fixed(1), Element: element}
}

func TestAttachmentIdentitySurvivesMoveButNotUnmount(t *testing.T) {
	a, b := scene.NewElement(scene.ElementConfig{Key: "a"}), scene.NewElement(scene.ElementConfig{Key: "b"})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a), item("b", b)}})
	r := root(t, g.Element())
	old := a.Target()
	lease := a.Attachment()
	if err := g.Move(0, 1); err != nil {
		t.Fatal(err)
	}
	if a.Target() != old {
		t.Fatal("move replaced a retained attachment")
	}
	if err := g.Set(item("b", b)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Done():
	default:
		t.Fatal("detachment did not revoke the lease synchronously")
	}
	if r.Valid(old) {
		t.Fatal("removed target remained valid")
	}
	if err := g.Set(item("a", a), item("b", b)); err != nil {
		t.Fatal(err)
	}
	if a.Target().Node != old.Node || a.Target().Generation <= old.Generation {
		t.Fatal("remount did not preserve identity and advance generation")
	}
	if r.Valid(old) {
		t.Fatal("old attachment became valid again")
	}
	replacement := scene.NewElement(scene.ElementConfig{Key: "a"})
	if err := g.Set(item("a", replacement)); err != nil {
		t.Fatal(err)
	}
	if replacement.Target().Node == old.Node {
		t.Fatal("same key revived another instance")
	}
}

func TestOneElementCannotHaveTwoParentsOrRoots(t *testing.T) {
	e := scene.NewElement(scene.ElementConfig{})
	first := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("same", e)}})
	second := scene.NewGroup(scene.GroupConfig{})
	if err := second.Set(item("same", e)); !errors.Is(err, scene.ErrMounted) {
		t.Fatalf("second parent: %v", err)
	}
	if err := first.Set(item("a", e), item("b", e)); !errors.Is(err, scene.ErrDuplicate) {
		t.Fatalf("duplicate instance: %v", err)
	}
	if len(first.Items()) != 1 {
		t.Fatal("rejected mutation changed child ownership")
	}
	r := root(t, first.Element())
	if _, err := scene.NewRoot(scene.RootConfig{Element: first.Element()}); !errors.Is(err, scene.ErrMounted) {
		t.Fatalf("second root: %v", err)
	}
	if err := first.Set(item("cycle", first.Element())); !errors.Is(err, scene.ErrCycle) {
		t.Fatalf("cycle: %v", err)
	}
	if r.Content() != first.Element() {
		t.Fatal("rejected mutation changed the root")
	}
}

func TestFrameCommitIsAtomicAndAbortDoesNotUndoModel(t *testing.T) {
	var geometry scene.Snapshot[int]
	value := 1
	e := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { geometry.Stage(frame, value) }})
	r := root(t, e)
	draw(t, r, 10, 2, 1)
	r.Wrote(1)
	value = 2
	e.Invalidate()
	before := r.Progress()
	p, err := r.Prepare(grid.NewSurface(5, 3).View())
	if err != nil {
		t.Fatal(err)
	}
	if geometry.Value() != 1 || r.Progress().LayoutRevision != before.LayoutRevision {
		t.Fatal("Prepare leaked candidate geometry")
	}
	p.Abort()
	if value != 2 || geometry.Value() != 1 || r.Progress().ModelRevision != before.ModelRevision {
		t.Fatal("Abort rolled back model state or committed geometry")
	}
	draw(t, r, 5, 3, 2)
	if geometry.Value() != 2 || r.Progress().LayoutRevision <= before.LayoutRevision {
		t.Fatal("Commit did not publish complete geometry")
	}
}

func TestPanicAbortsEveryNestedSnapshot(t *testing.T) {
	var first, second scene.Snapshot[int]
	value := 1
	fail := false
	a := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { first.Stage(frame, value) }})
	b := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		second.Stage(frame, value)
		if fail {
			panic("paint failure")
		}
	}})
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a), item("b", b)}}).Element())
	draw(t, r, 10, 2, 0)
	value, fail = 2, true
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing painter failure")
			}
		}()
		_, _ = r.Prepare(grid.NewSurface(10, 2).View())
	}()
	if first.Value() != 1 || second.Value() != 1 {
		t.Fatal("failed nested prepare published partial geometry")
	}
	fail = false
	draw(t, r, 10, 2, 0)
	if first.Value() != 2 || second.Value() != 2 {
		t.Fatal("failed prepare retained a candidate producer")
	}
}

func TestPointerUsesArrivalWrittenGeometryAndCurrentAttachment(t *testing.T) {
	var received []string
	leaf := func(name string) scene.Element {
		return scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
			if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown {
				received = append(received, name)
			}
			return scene.EventResult{Consumed: true}
		}})
	}
	a, b := leaf("a"), leaf("b")
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a), item("b", b)}})
	r := root(t, g.Element())
	draw(t, r, 10, 2, 1)
	r.Wrote(1)
	click := r.CaptureInput(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 0)})
	if err := g.Move(0, 1); err != nil {
		t.Fatal(err)
	}
	draw(t, r, 10, 2, 2)
	r.Handle(input.Mouse{Action: input.MouseDown, Pos: image.Pt(0, 0)})
	r.Wrote(2)
	r.HandleInput(click)
	r.Handle(input.Mouse{Action: input.MouseDown, Pos: image.Pt(0, 0)})
	if !slices.Equal(received, []string{"a", "a", "b"}) {
		t.Fatalf("input retargeted across admission/writing: %v", received)
	}
	old := r.CaptureInput(input.Mouse{Action: input.MouseDown, Pos: image.Pt(0, 0)})
	if err := g.Set(item("a", a)); err != nil {
		t.Fatal(err)
	}
	r.HandleInput(old)
	if len(received) != 3 {
		t.Fatalf("removed target received old geometry input: %v", received)
	}
}

func TestCaptureKeepsForeignReleaseAndEndsOnDetach(t *testing.T) {
	var received []input.MouseAction
	e := scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
		mouse, ok := event.Input.(input.Mouse)
		if !ok {
			return scene.EventResult{}
		}
		received = append(received, mouse.Action)
		return scene.EventResult{Consumed: true, Capture: mouse.Action == input.MouseDown}
	}})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("target", e)}})
	r := root(t, g.Element())
	draw(t, r, 10, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight, Pos: image.Pt(30, 30)})
	r.Handle(input.Mouse{Action: input.MouseDrag, Button: input.ButtonLeft, Pos: image.Pt(30, 30)})
	if err := g.Set(); err != nil {
		t.Fatal(err)
	}
	r.FlushNotifications()
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
	if !slices.Equal(received, []input.MouseAction{input.MouseDown, input.MouseDrag, input.MouseCancel, input.MouseLeave}) {
		t.Fatalf("capture lifetime: %v", received)
	}
}

func TestModalFocusAndActionEligibilityHaveOneOwner(t *testing.T) {
	invoked := 0
	action := scene.NewAction(scene.ActionConfig[struct{}, int]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[int], error) {
		invoked++
		return scene.ActionResult[int]{State: scene.Applied, Value: invoked}, nil
	}})
	leaf := func(name string) scene.Element {
		return scene.NewElement(scene.ElementConfig{Key: name, Focusable: func() bool { return true }, Actions: []scene.ActionBinding{action.Binding()}, Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleButton, Name: name} }})
	}
	a, b, modalButton := leaf("a"), leaf("b"), leaf("modal")
	open := false
	modal := scene.NewGroup(scene.GroupConfig{Modal: true, Overlay: true, Element: scene.ElementConfig{Enabled: func() bool { return open }}})
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a), item("b", b), item("modal", modal.Element())}})
	r := root(t, g.Element())
	if err := b.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	open = true
	if err := modal.Set(item("button", modalButton)); err != nil {
		t.Fatal(err)
	}
	if !modalButton.Focused() || b.Focused() {
		t.Fatal("modal did not own actual focus")
	}
	if _, err := r.Invoke(context.Background(), b.Target(), scene.ActionActivate, nil); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("background action: %v", err)
	}
	if _, err := r.Invoke(context.Background(), modalButton.Target(), scene.ActionActivate, nil); err != nil {
		t.Fatal(err)
	}
	open = false
	if err := modal.Set(); err != nil {
		t.Fatal(err)
	}
	if !b.Focused() || invoked != 1 {
		t.Fatal("close did not restore valid prior focus")
	}
}

func TestObservationIsPureBoundedAndRedactsSensitiveValues(t *testing.T) {
	value := "secret"
	e := scene.NewElement(scene.ElementConfig{Key: "password", Semantics: func() scene.Semantic {
		return scene.Semantic{Role: scene.RoleTextBox, Name: "Password", Value: value, Sensitive: true, Relations: []scene.Relation{{Kind: scene.LabelledBy}}}
	}})
	r := root(t, e)
	before := r.Progress()
	first, err := r.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Nodes) != 1 || first.Nodes[0].Value != "" || r.Progress() != before {
		t.Fatal("observation changed state or exposed a secret")
	}
	first.Nodes[0].Relations[0].Kind = scene.Controls
	second, err := r.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Nodes[0].Relations[0].Kind != scene.LabelledBy {
		t.Fatal("observation exposed mutable backing storage")
	}
}

func TestTypedActionRejectsParametersAndReportsExplicitResult(t *testing.T) {
	value := 0
	set := scene.NewAction(scene.ActionConfig[int, int]{ID: scene.ActionSetValue, Availability: func(next int) scene.Availability { return scene.Availability{Disabled: next < 0, Reason: "negative"} }, Run: func(_ context.Context, next int) (scene.ActionResult[int], error) {
		if value == next {
			return scene.ActionResult[int]{State: scene.Unchanged, Value: value}, nil
		}
		value = next
		return scene.ActionResult[int]{State: scene.Applied, Value: value}, nil
	}})
	e := scene.NewElement(scene.ElementConfig{Actions: []scene.ActionBinding{set.Binding()}})
	r := root(t, e)
	if _, err := r.Invoke(context.Background(), e.Target(), scene.ActionSetValue, "1"); !errors.Is(err, scene.ErrArgument) {
		t.Fatalf("argument: %v", err)
	}
	if _, err := r.Invoke(context.Background(), e.Target(), scene.ActionSetValue, -1); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("availability: %v", err)
	}
	result, err := r.Invoke(context.Background(), e.Target(), scene.ActionSetValue, 1)
	if err != nil || result.State != scene.Applied || result.Value != 1 {
		t.Fatalf("applied: %+v %v", result, err)
	}
	result, err = r.Invoke(context.Background(), e.Target(), scene.ActionSetValue, 1)
	if err != nil || result.State != scene.Unchanged {
		t.Fatalf("unchanged: %+v %v", result, err)
	}
}

func TestZeroByteLayoutWaitsForInheritedWrite(t *testing.T) {
	e := scene.NewElement(scene.ElementConfig{})
	r := root(t, e)
	draw(t, r, 10, 2, 1)
	draw(t, r, 20, 2, 0)
	if r.Progress().WrittenFrame != 0 {
		t.Fatal("zero bytes manufactured transport completion")
	}
	r.Wrote(1)
	if r.Progress().WrittenFrame != r.Progress().LayoutRevision || r.Progress().WrittenSequence != 1 {
		t.Fatalf("inherited write did not settle layout: %+v", r.Progress())
	}
}
