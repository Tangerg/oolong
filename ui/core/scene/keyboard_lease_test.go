package scene_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestKeyboardLeaseFollowsContinuousFocusWithin(t *testing.T) {
	first := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	second := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	outside := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	inside := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first), item("second", second)}})
	container := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("inside", inside.Element()), item("outside", outside)}})
	standalone := inside.Element().KeyboardLease()
	if !standalone.Valid() || (scene.KeyboardLease{}).Valid() {
		t.Fatal("standalone and missing keyboard lifetimes were confused")
	}
	r := root(t, container.Element())
	if err := first.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	lease := inside.Element().KeyboardLease()
	delegated := second.KeyboardLease()
	if !lease.Valid() || standalone.Valid() {
		t.Fatal("attachment did not replace the standalone lifetime")
	}
	if err := second.RequestFocus(); err != nil || !lease.Valid() {
		t.Fatal("moving within the group revoked its keyboard lifetime", err)
	}
	if delegated.Valid() {
		t.Fatal("a delegated lease survived its keyboard origin losing focus")
	}
	if err := outside.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if err := first.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if lease.Valid() {
		t.Fatal("returning focus revived an earlier keyboard lifetime")
	}
	lease = inside.Element().KeyboardLease()
	if err := r.SetContent(scene.Element{}); err != nil {
		t.Fatal(err)
	}
	r.FlushNotifications()
	if err := r.SetContent(container.Element()); err != nil {
		t.Fatal(err)
	}
	if err := first.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if lease.Valid() {
		t.Fatal("remount revived an earlier keyboard lifetime")
	}
}

func TestKeyboardLeaseTracksModalFallbackWithoutFocus(t *testing.T) {
	body := scene.NewElement(scene.ElementConfig{})
	modal := scene.NewGroup(scene.GroupConfig{Modal: true})
	container := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("modal", modal.Element())}})
	r := root(t, container.Element())
	beforeModal := container.Element().KeyboardLease()
	if !r.Focus().IsZero() || !beforeModal.Valid() {
		t.Fatal("root content did not own focusless keyboard input")
	}
	if err := modal.Set(item("body", body)); err != nil {
		t.Fatal(err)
	}
	opened := modal.Element().KeyboardLease()
	if !r.Focus().IsZero() || beforeModal.Valid() || !opened.Valid() {
		t.Fatal("modal fallback did not revoke the previous route while focus stayed empty")
	}
	if err := modal.Set(); err != nil {
		t.Fatal(err)
	}
	if opened.Valid() || beforeModal.Valid() || !container.Element().KeyboardLease().Valid() {
		t.Fatal("closing a focusless modal revived an older root route")
	}
	if err := modal.Set(item("body", body)); err != nil {
		t.Fatal(err)
	}
	if opened.Valid() || !modal.Element().KeyboardLease().Valid() {
		t.Fatal("reopening a focusless modal revived its older lifetime")
	}
}

func TestKeyboardLeasesReadTheCurrentCompositionBeforeFocusSettles(t *testing.T) {
	var previous, captured scene.KeyboardLease
	var modal *scene.Group
	query, observed, previousWasValid := false, false, false
	modal = scene.NewGroup(scene.GroupConfig{Modal: true, Element: scene.ElementConfig{Focusable: func() bool {
		if query {
			observed = true
			previousWasValid = previous.Valid()
			captured = modal.Element().KeyboardLease()
		}
		return false
	}}})
	container := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("modal", modal.Element())}})
	root(t, container.Element())
	previous = container.Element().KeyboardLease()
	query = true
	if err := modal.Set(item("body", scene.NewElement(scene.ElementConfig{}))); err != nil {
		t.Fatal(err)
	}
	if !observed || previousWasValid {
		t.Fatal("the previous route authorized input after the composition changed")
	}
	if !captured.Valid() {
		t.Fatal("a lease captured from the current composition borrowed the previous route")
	}
}

func TestInterruptedFocusSettlementCannotReviveAKeyboardLease(t *testing.T) {
	interrupt := false
	modal := scene.NewGroup(scene.GroupConfig{Modal: true, Element: scene.ElementConfig{Focusable: func() bool {
		if interrupt {
			panic("focus policy interrupted")
		}
		return false
	}}})
	container := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("modal", modal.Element())}})
	root(t, container.Element())
	previous := container.Element().KeyboardLease()
	interrupt = true
	func() {
		defer func() {
			if recover() == nil {
				t.Error("focus policy did not interrupt settlement")
			}
		}()
		if err := modal.Set(item("body", scene.NewElement(scene.ElementConfig{}))); err != nil {
			t.Fatal(err)
		}
	}()
	interrupt = false
	if previous.Valid() {
		t.Fatal("interrupted settlement retained the earlier keyboard scope")
	}
	if err := modal.Set(); err != nil {
		t.Fatal(err)
	}
	if previous.Valid() {
		t.Fatal("returning to the previous route revived its interrupted lease")
	}
}

func TestKeyboardLeaseQueriesBorrowCurrentEligibility(t *testing.T) {
	var container *scene.Group
	check, queries := false, 0
	element := scene.NewElement(scene.ElementConfig{Enabled: func() bool {
		if check {
			queries++
			if err := container.Set(); !errors.Is(err, scene.ErrBusy) {
				t.Errorf("eligibility detached the receiver of its keyboard lease: %v", err)
			}
		}
		return true
	}, Focusable: func() bool { return true }})
	container = scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("element", element)}})
	root(t, container.Element())
	lease := element.KeyboardLease()
	check = true
	if !element.KeyboardLease().Valid() || !lease.Valid() || queries == 0 {
		t.Fatal("keyboard leases did not borrow current eligibility")
	}
}
