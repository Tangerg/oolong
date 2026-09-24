package headless_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// reentrant moves the keyboard again from inside being told where it stands.
//
// It is not a contrivance: a pane that closes its dialog when it loses focus, a tab
// that selects another on being blurred, and a field that hands the keyboard on when
// it is done are all this shape. The owner is part-way through one transfer and is
// called back to begin another, synchronously, on the same goroutine.
type reentrant struct {
	name    string
	focused bool
	// changes is every transition this widget was told about, in order. A widget
	// that ends up with the keyboard having been told it lost it in between has
	// already done whatever losing it means — committed its edit, closed its
	// completion list — so the ending state alone does not say the transfer was
	// right.
	changes []bool
	// on runs once, so that the test exercises one interruption rather than a
	// recursion whose depth is the thing under test.
	on func(has bool)
}

func (*reentrant) Draw(headless.Frame) {}

func (*reentrant) HeightForWidth(int) int { return 1 }

func (*reentrant) Handle(input.Event) bool { return false }

func (r *reentrant) Focus(has bool) {
	r.focused = has
	r.changes = append(r.changes, has)
	if on := r.on; on != nil {
		r.on = nil
		on(has)
	}
}

// told is what a widget that was given the keyboard and never lost it heard.
var told = []bool{true}

// forget drops what every widget has been told so far, so that a test reads only
// the transfer it is about and not the settling that set it up.
func forget(widgets ...*reentrant) {
	for _, w := range widgets {
		w.changes = nil
	}
}

func holders(widgets ...*reentrant) []string {
	var has []string
	for _, w := range widgets {
		if w.focused {
			has = append(has, w.name)
		}
	}
	return has
}

func TestATabChosenWhileTheKeyboardWasMovingIsTheOneThatEndsWithIt(t *testing.T) {
	a, b, c := &reentrant{name: "a"}, &reentrant{name: "b"}, &reentrant{name: "c"}
	tabs := headless.NewTabs(headless.TabsConfig{Items: []headless.Tab{
		{Title: "a", Of: a}, {Title: "b", Of: b}, {Title: "c", Of: c},
	}})
	tabs.Focus(true)
	a.on = func(has bool) {
		if !has {
			tabs.Select(2)
		}
	}

	forget(a, b, c)
	tabs.Select(1)
	if got := tabs.Selected(); got != 2 {
		t.Fatalf("selected %d, want the tab chosen from inside the transfer", got)
	}
	if got := holders(a, b, c); len(got) != 1 || got[0] != "c" {
		t.Fatalf("the keyboard is on %v, want it on the selected tab alone", got)
	}
	if !slices.Equal(c.changes, told) {
		t.Fatalf("c was told %v, want it given the keyboard once and never taken off it", c.changes)
	}
}

func TestAChildChosenWhileTheKeyboardWasMovingIsTheOneThatEndsWithIt(t *testing.T) {
	a, b, c := &reentrant{name: "a"}, &reentrant{name: "b"}, &reentrant{name: "c"}
	box := headless.NewContainer(layout.Down,
		headless.Item{Of: a}, headless.Item{Of: b}, headless.Item{Of: c})
	box.Focus(true)
	a.on = func(has bool) {
		if !has {
			box.FocusIndex(2)
		}
	}

	forget(a, b, c)
	box.FocusIndex(1)
	if got := box.Focused(); got != headless.Widget(c) {
		t.Fatalf("the container holds %v, want the child chosen from inside the transfer", got)
	}
	if got := holders(a, b, c); len(got) != 1 || got[0] != "c" {
		t.Fatalf("the keyboard is on %v, want it on the focused child alone", got)
	}
	if !slices.Equal(c.changes, told) {
		t.Fatalf("c was told %v, want it given the keyboard once and never taken off it", c.changes)
	}
}

func TestContentReplacedWhileTheKeyboardWasMovingIsWhatEndsWithIt(t *testing.T) {
	a, b, c := &reentrant{name: "a"}, &reentrant{name: "b"}, &reentrant{name: "c"}
	window := headless.NewViewport(a)
	a.on = func(has bool) {
		if !has {
			window.SetContent(c)
		}
	}

	forget(a, b, c)
	window.SetContent(b)
	if got := window.Content(); got != headless.Sized(c) {
		t.Fatalf("the window shows %v, want the content set from inside the transfer", got)
	}
	if got := holders(a, b, c); len(got) != 1 || got[0] != "c" {
		t.Fatalf("the keyboard is on %v, want it on what the window shows alone", got)
	}
	if !slices.Equal(c.changes, told) {
		t.Fatalf("c was told %v, want it given the keyboard once and never taken off it", c.changes)
	}
}

func TestATriggerAppearanceReplacedWhileTheKeyboardWasMovingIsWhatEndsWithIt(t *testing.T) {
	a, b, c := &reentrant{name: "a"}, &reentrant{name: "b"}, &reentrant{name: "c"}
	stack := headless.NewStack(&focusProbe{})
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Content: &focusedPanel{}})
	trigger := dialog.Trigger("open", a)
	trigger.Focus(true)
	a.on = func(has bool) {
		if !has {
			trigger.SetAppearance(c)
		}
	}

	forget(a, b, c)
	trigger.SetAppearance(b)
	if got := trigger.Appearance(); got != headless.Widget(c) {
		t.Fatalf("the trigger shows %v, want the appearance set from inside the transfer", got)
	}
	if got := holders(a, b, c); len(got) != 1 || got[0] != "c" {
		t.Fatalf("the keyboard is on %v, want it on what the trigger shows alone", got)
	}
	if !slices.Equal(c.changes, told) {
		t.Fatalf("c was told %v, want it given the keyboard once and never taken off it", c.changes)
	}
}
