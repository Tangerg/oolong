package headless_test

import (
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestTextFailedBindingReadPreservesThePreviousOwnerAndHistory(t *testing.T) {
	for _, exit := range []string{"panic", "goexit"} {
		t.Run(exit, func(t *testing.T) {
			old := "seed"
			field := headless.NewText(headless.TextConfig{Value: headless.Bind(&old)})
			field.SetText("seedx")
			next := &unreadableBinding[string]{value: "seedx"}
			failBindingRead(t, exit, next, func() { field.SetBinding(next) })
			if !field.Do(headless.Undo) || old != "seed" || next.value != "seedx" {
				t.Fatalf("failed transfer moved old history to a new owner: old=%q next=%q", old, next.value)
			}
			if !field.Do(headless.Redo) || old != "seedx" {
				t.Fatal("failed transfer discarded the previous owner's redo")
			}
			field.SetBinding(next)
			revision := field.Revision()
			field.Do(headless.Undo)
			if next.value != "seedx" || next.writes != 0 || field.Revision() != revision {
				t.Fatal("completed transfer retained the previous owner's history")
			}
		})
	}
}

func TestSelectFailedBindingReadPreservesThePreviousOwner(t *testing.T) {
	for _, exit := range []string{"panic", "goexit"} {
		t.Run(exit, func(t *testing.T) {
			old := "a"
			field := headless.NewSelect(headless.SelectConfig[string]{Value: headless.Bind(&old), Same: headless.Equal[string]})
			field.SetOptions(headless.Options("a", "b"))
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			validated := field.Validation()
			next := &unreadableBinding[string]{value: "b"}
			failBindingRead(t, exit, next, func() { field.SetBinding(next) })
			if chosen, ok := field.Chosen(); !ok || chosen.Value != "a" || !validated() {
				t.Fatalf("failed transfer published choice=%v exists=%t or revoked its validation", chosen, ok)
			}
			field.Do(headless.SelectNext)
			if old != "b" || next.writes != 0 {
				t.Fatal("failed transfer changed the answer owner")
			}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			validated = field.Validation()
			field.SetBinding(next)
			if validated() || field.Validation() != nil {
				t.Fatal("completed transfer retained the previous owner's validation")
			}
		})
	}
}

func TestSelectBindingComparisonPanicCannotTransferOwnership(t *testing.T) {
	old, next := "a", "b"
	fail := false
	field := headless.NewSelect(headless.SelectConfig[string]{Value: headless.Bind(&old), Same: func(a, b string) bool {
		if fail && b == next {
			panic("comparison failed")
		}
		return a == b
	}})
	field.SetOptions(headless.Options("a", "b"))
	fail = true
	func() {
		defer func() {
			if got := recover(); got != "comparison failed" {
				t.Fatalf("comparison panic = %v", got)
			}
		}()
		field.SetBinding(headless.Bind(&next))
	}()
	fail = false
	if chosen, ok := field.Chosen(); !ok || chosen.Value != old {
		t.Fatalf("failed comparison published choice=%v exists=%t", chosen, ok)
	}
	field.Do(headless.SelectNext)
	if old != "b" {
		t.Fatal("failed comparison changed the answer owner")
	}
}

func TestTextUnreadableRejectedWriteRetainsItsSettlement(t *testing.T) {
	for _, test := range []struct {
		name                string
		goexit, synchronize bool
	}{
		{"panic action", false, false},
		{"goexit action", true, false},
		{"panic Sync", false, true},
		{"goexit Sync", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := &unreadableBinding[string]{value: "seed"}
			field := headless.NewText(headless.TextConfig{Value: value})
			field.SetText("seedx")
			field.Do(headless.Undo)
			revision := field.Revision()
			value.write = func(string) {
				if test.goexit {
					value.read = runtime.Goexit
				} else {
					value.read = func() { panic("read failed") }
				}
				value.read()
			}
			if test.goexit {
				done := make(chan struct{})
				go func() {
					defer close(done)
					field.SetText("rejected")
					t.Error("write returned instead of exiting")
				}()
				<-done
			} else {
				func() {
					defer func() {
						if got := recover(); got != "read failed" {
							t.Fatalf("write panic = %v", got)
						}
					}()
					field.SetText("rejected")
				}()
			}
			value.read, value.write = nil, nil
			if test.synchronize {
				field.Sync()
				if field.Revision() != revision {
					t.Fatal("Sync published a rejected proposal as an external content change")
				}
			}
			field.Do(headless.Redo)
			if value.value != "seedx" {
				t.Fatalf("unreadable rejected write lost its redo settlement: %q", value.value)
			}
		})
	}
}

func TestReplacingBindingDoesNotReadTheAbandonedAnswer(t *testing.T) {
	for _, kind := range []string{"Text", "Select", "MultiSelect", "Confirm"} {
		t.Run(kind, func(t *testing.T) {
			var replace func()
			var current func() bool
			var stopReading func()
			switch kind {
			case "Text":
				old := &unreadableBinding[string]{value: "old"}
				field := headless.NewText(headless.TextConfig{Value: old})
				next := "new"
				replace = func() { field.SetBinding(headless.Bind(&next)) }
				current = func() bool { return field.Text() == next }
				stopReading = func() { old.read = func() { panic("abandoned answer read") } }
			case "Select":
				old := &unreadableBinding[string]{value: "old"}
				field := headless.NewSelect(headless.SelectConfig[string]{Value: old, Same: headless.Equal[string]})
				field.SetOptions(headless.Options("old", "new"))
				next := "new"
				replace = func() { field.SetBinding(headless.Bind(&next)) }
				current = func() bool { choice, ok := field.Chosen(); return ok && choice.Value == next }
				stopReading = func() { old.read = func() { panic("abandoned answer read") } }
			case "MultiSelect":
				old := &unreadableBinding[[]string]{value: []string{"old"}}
				field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Value: old, Same: headless.Equal[string]})
				field.SetOptions(headless.Options("old", "new"))
				next := []string{"new"}
				replace = func() { field.SetBinding(headless.Bind(&next)) }
				current = func() bool { taken := field.Taken(); return len(taken) == 1 && taken[0] == next[0] }
				stopReading = func() { old.read = func() { panic("abandoned answer read") } }
			case "Confirm":
				old := &unreadableBinding[bool]{}
				field := headless.NewConfirm(headless.ConfirmConfig{Value: old})
				next := true
				replace = func() { field.SetBinding(headless.Bind(&next)) }
				current = func() bool { return field.Answer() == next }
				stopReading = func() { old.read = func() { panic("abandoned answer read") } }
			}
			stopReading()
			defer func() {
				if got := recover(); got != nil {
					t.Fatalf("replacement consulted the abandoned owner: %v", got)
				}
			}()
			replace()
			if !current() {
				t.Fatal("replacement did not publish the new owner")
			}
		})
	}
}

type unreadableBinding[T any] struct {
	value  T
	writes int
	read   func()
	write  func(T)
}

func (v *unreadableBinding[T]) Value() T {
	if v.read != nil {
		v.read()
	}
	return v.value
}

func (v *unreadableBinding[T]) Set(value T) {
	v.writes++
	if v.write != nil {
		v.write(value)
		return
	}
	v.value = value
}

func failBindingRead[T any](t *testing.T, exit string, value *unreadableBinding[T], change func()) {
	t.Helper()
	if exit == "goexit" {
		value.read = runtime.Goexit
		done := make(chan struct{})
		go func() {
			defer close(done)
			change()
			t.Error("binding read returned instead of exiting")
		}()
		<-done
	} else {
		value.read = func() { panic("read failed") }
		func() {
			defer func() {
				if got := recover(); got != "read failed" {
					t.Fatalf("read panic = %v", got)
				}
			}()
			change()
		}()
	}
	value.read = nil
}
