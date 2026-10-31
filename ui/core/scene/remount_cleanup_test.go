package scene_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTransferRequiresOnlyThisElementsOutstandingCleanup(t *testing.T) {
	first, spare := scene.NewElement(scene.ElementConfig{
		Focusable: func() bool { return true }, FocusChanged: func(bool) {},
	}), scene.NewElement(scene.ElementConfig{})
	other := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("first", first), item("other", other), item("spare", spare)}})
	original := root(t, group.Element())
	original.FlushNotifications()
	if err := other.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	// The old blur was scheduled before removal, not by detach.
	if err := group.Set(item("other", other)); err != nil {
		t.Fatal(err)
	}
	destination := root(t, scene.Element{})
	if err := destination.SetContent(spare); err != nil {
		t.Fatalf("another element's pending blur blocked transfer: %v", err)
	}
	if err := destination.SetContent(first); !errors.Is(err, scene.ErrCleanupPending) {
		t.Fatal("new generation claimed an element still owed its old blur")
	}
	if destination.Content() != spare || first.Alive() {
		t.Fatal("refused transfer changed the destination")
	}
	original.FlushNotifications()
	if err := destination.SetContent(first); err != nil || !first.Focused() {
		t.Fatalf("settled transfer did not establish the new focus: %v", err)
	}
}

func TestInterruptedCleanupBlocksRemountUntilItsLastCallbackReturns(t *testing.T) {
	for _, abnormal := range []string{"panic", "goexit"} {
		t.Run(abnormal, func(t *testing.T) {
			destination := root(t, scene.Element{})
			var element scene.Element
			detaches := 0
			check := func() {
				if err := destination.SetContent(element); !errors.Is(err, scene.ErrCleanupPending) {
					t.Error("reentrant mount claimed an element during old cleanup")
				}
			}
			element = scene.NewElement(scene.ElementConfig{
				Focusable: func() bool { return true },
				FocusChanged: func(focused bool) {
					if focused || detaches != 0 {
						return
					}
					check()
					if abnormal == "goexit" {
						runtime.Goexit()
					}
					panic("interrupted")
				},
				Detached: func() {
					detaches++
					if detaches == 1 {
						check()
					}
				},
			})
			original := root(t, element)
			original.FlushNotifications()
			if err := original.SetContent(scene.Element{}); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					got := recover()
					if abnormal == "panic" && got != "interrupted" || abnormal == "goexit" && got != nil {
						t.Errorf("unexpected callback exit: %v", got)
					}
				}()
				original.FlushNotifications()
			}()
			<-done
			if err := original.SetContent(element); !errors.Is(err, scene.ErrCleanupPending) {
				t.Fatal("interrupted flush abandoned the pending Detached callback")
			}
			original.FlushNotifications()
			if err := original.SetContent(element); err != nil || detaches != 1 {
				t.Fatalf("same-root remount after cleanup: detached=%d, err=%v", detaches, err)
			}
		})
	}
}

func TestFailedConstructorDoesNotStrandTheUnpublishedCleanup(t *testing.T) {
	first := true
	detaches := 0
	element := scene.NewElement(scene.ElementConfig{Focusable: func() bool {
		if first {
			first = false
			panic("incomplete mount")
		}
		return true
	}, Detached: func() { detaches++ }})
	func() {
		defer func() {
			if recover() != "incomplete mount" {
				t.Error("constructor did not propagate the focus predicate panic")
			}
		}()
		_, _ = scene.NewRoot(scene.RootConfig{Element: element})
	}()
	next := root(t, element)
	if detaches != 0 {
		t.Fatal("an unpublished mount delivered cleanup")
	}
	next.Close()
	if detaches != 1 {
		t.Fatal("published mount lost its cleanup")
	}
}
